package handlers

import (
	"net/http"
	"sort"
	"time"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type counter struct{ total, done, late int }

func (c *counter) add(done, late bool) {
	c.total++
	if done {
		c.done++
	}
	if late {
		c.late++
	}
}

func counterFor(m map[models.ID]*counter, id models.ID) *counter {
	if m[id] == nil {
		m[id] = &counter{}
	}
	return m[id]
}

// buildReport counts the tasks a user can see. from/to filter by the day a task was created.
func buildReport(u *models.User, from, to, projectID string) bson.M {
	projects, workspaces := services.VisibleProjects(u)
	ids := []models.ID{}
	for _, p := range projects {
		if projectID == "" || p.ID.Hex() == projectID {
			ids = append(ids, p.ID)
		}
	}
	all, byProject, byPerson := &counter{}, map[models.ID]*counter{}, map[models.ID]*counter{}
	byStatus, byPriority := map[string]int{}, map[string]int{}
	upcoming := []bson.M{}
	nextWeek := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	for _, t := range utils.FindAll[models.Task]("tasks", bson.M{"projectId": bson.M{"$in": ids}}) {
		created := t.CreatedAt.Format("2006-01-02")
		if (from != "" && created < from) || (to != "" && created > to) {
			continue
		}
		done, late := t.Status == "completed", t.IsOverdue()
		byStatus[t.Status]++
		byPriority[t.Priority]++
		all.add(done, late)
		counterFor(byProject, t.ProjectID).add(done, late)
		for _, person := range t.Assignees {
			counterFor(byPerson, person).add(done, late)
		}
		if !done && t.DueDate >= utils.Today() && t.DueDate <= nextWeek {
			upcoming = append(upcoming, bson.M{"id": t.ID, "title": t.Title, "dueDate": t.DueDate, "priority": t.Priority})
		}
	}
	sort.Slice(upcoming, func(i, j int) bool { return upcoming[i]["dueDate"].(string) < upcoming[j]["dueDate"].(string) })

	projectRows := []bson.M{}
	for _, p := range projects {
		if utils.HasID(ids, p.ID) {
			c, percent := counterFor(byProject, p.ID), 0
			if c.total > 0 {
				percent = c.done * 100 / c.total
			}
			projectRows = append(projectRows, bson.M{"id": p.ID, "name": p.Name, "workspace": workspaces[p.WorkspaceID].Name, "status": p.Status,
				"deadline": p.Deadline, "total": c.total, "completed": c.done, "overdue": c.late, "percent": percent})
		}
	}
	people := []models.ID{}
	for id := range byPerson {
		people = append(people, id)
	}
	workload := []bson.M{}
	for _, person := range utils.UsersByID(people) {
		c := byPerson[person["id"].(models.ID)]
		workload = append(workload, bson.M{"user": person, "open": c.total - c.done, "completed": c.done, "overdue": c.late})
	}
	return bson.M{"total": all.total, "completed": all.done, "incomplete": all.total - all.done, "overdue": all.late,
		"byStatus": byStatus, "byPriority": byPriority, "projects": projectRows, "workload": workload, "upcoming": upcoming,
		"summary": summary(u, len(workspaces), len(projects)), "activity": services.RecentActivity(services.ActivityFilter(u, projects), 15)}
}

// summary: platform numbers for the system admin, team numbers for an admin.
func summary(u *models.User, workspaces, projects int) bson.M {
	count := func(collection string, f bson.M) int64 {
		n, _ := database.Col(collection).CountDocuments(database.Ctx, f)
		return n
	}
	result := bson.M{"workspaces": workspaces, "projects": projects}
	if u.Role == "sysadmin" {
		result["users"], result["activeUsers"] = count("users", bson.M{}), count("users", bson.M{"active": true})
		result["admins"], result["pendingRequests"] = count("users", bson.M{"role": "admin"}), count("requests", bson.M{"status": "pending"})
	} else if u.Role == "admin" {
		result["teamSize"], result["pendingRequests"] = count("users", bson.M{"adminId": u.ID}), count("requests", bson.M{"adminId": u.ID, "status": "pending"})
	}
	return result
}

// Reports: ?from=&to= (task creation dates) and ?projectId=.
func Reports(r *http.Request, u *models.User) (any, error) {
	q := r.URL.Query()
	if !utils.ValidDate(q.Get("from")) || !utils.ValidDate(q.Get("to")) {
		return nil, utils.Fail(400, "dates must look like 2026-12-31")
	}
	return buildReport(u, q.Get("from"), q.Get("to"), q.Get("projectId")), nil
}
