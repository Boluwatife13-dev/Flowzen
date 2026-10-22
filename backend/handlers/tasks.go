package handlers

import (
	"net/http"
	"strings"
	"time"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func assignedText(u *models.User, a taskAccess) string {
	text := u.Name + " assigned you to \"" + a.t.Title + "\" in project " + a.p.Name
	if a.t.DueDate != "" {
		text += " (due " + a.t.DueDate + ")"
	}
	return text
}

// ListTasks supports: q, projectId, status (or "open"), priority, assignee (me or an id),
// due (overdue, today, week, none), dueFrom, dueTo, sort, order, page, limit.
func ListTasks(r *http.Request, u *models.User) (any, error) {
	q := r.URL.Query()
	projects, _ := services.VisibleProjects(u)
	names, ids := bson.M{}, []models.ID{}
	for _, p := range projects {
		names[p.ID.Hex()] = p.Name
		ids = append(ids, p.ID)
	}
	filter := bson.M{"projectId": bson.M{"$in": ids}}
	if id := utils.ToID(q.Get("projectId")); utils.HasID(ids, id) {
		filter["projectId"] = id
	}
	if s := q.Get("status"); s == "open" {
		filter["status"] = bson.M{"$ne": "completed"}
	} else if s != "" {
		filter["status"] = s
	}
	if q.Get("priority") != "" {
		filter["priority"] = q.Get("priority")
	}
	if a := q.Get("assignee"); a == "me" {
		filter["assignees"] = u.ID
	} else if a != "" {
		filter["assignees"] = utils.ToID(a)
	}
	if s := q.Get("q"); s != "" {
		filter["$or"] = []bson.M{{"title": utils.Search(s)}, {"description": utils.Search(s)}, {"labels": strings.ToLower(strings.TrimSpace(s))}}
	}

	today, due := utils.Today(), bson.M{}
	switch q.Get("due") {
	case "overdue":
		due["$gt"], due["$lt"], filter["status"] = "", today, bson.M{"$ne": "completed"}
	case "today":
		due["$eq"] = today
	case "week":
		due["$gte"], due["$lte"] = today, time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	case "none":
		filter["dueDate"] = ""
	}
	if from := q.Get("dueFrom"); from != "" && utils.ValidDate(from) {
		due["$gte"] = from
	}
	if to := q.Get("dueTo"); to != "" && utils.ValidDate(to) {
		due["$lte"], due["$gt"] = to, ""
	}
	if len(due) > 0 {
		filter["dueDate"] = due
	}

	field := map[string]string{"dueDate": "dueDate", "priority": "priorityLevel", "title": "title", "status": "status"}[q.Get("sort")]
	direction := 1
	if field == "" {
		field, direction = "createdAt", -1 // newest first
	}
	if q.Get("order") == "desc" {
		direction = -1
	} else if q.Get("order") == "asc" {
		direction = 1
	}
	result := utils.FindPage[models.Task](r, "tasks", filter, bson.D{{Key: field, Value: direction}, {Key: "_id", Value: -1}})
	people := []models.ID{}
	for _, t := range result["items"].([]models.Task) {
		people = append(people, t.Assignees...)
	}
	result["projects"], result["users"] = names, utils.UsersByID(people)
	return result, nil
}

// CreateTask: managers and contributors create tasks; only managers can assign them.
func CreateTask(r *http.Request, u *models.User) (any, error) {
	var in taskInput
	utils.ReadJSON(r, &in)
	p, ws, role, err := services.LoadProject(u, utils.ToID(in.ProjectID))
	if err != nil {
		return nil, err
	}
	if role != "manager" && role != "contributor" {
		return nil, utils.Fail(403, "you cannot create tasks in this project")
	}
	if in.Assignees != nil && len(*in.Assignees) > 0 && role != "manager" {
		return nil, utils.Fail(403, "only project managers can assign tasks")
	}
	now := time.Now()
	t := &models.Task{ID: models.NewID(), ProjectID: p.ID, WorkspaceID: ws.ID, AdminID: ws.AdminID, Priority: "medium", PriorityLevel: 2,
		Status: "pending", Assignees: []models.ID{}, Labels: []string{}, Dependencies: []models.ID{}, Subtasks: []models.Subtask{},
		Files: []models.File{}, CreatedBy: u.ID, CreatedAt: now, UpdatedAt: now}
	a := taskAccess{t, p, ws, role}
	if err := in.apply(a); err != nil {
		return nil, err
	}
	if t.Status == "completed" {
		t.CompletedAt = &now
	}
	database.Col("tasks").InsertOne(database.Ctx, t)
	for _, id := range t.Assignees {
		if id != u.ID {
			services.Notify(id, assignedText(u, a), a.link(), true)
		}
	}
	services.AddActivity(u, "created task \""+t.Title+"\"", a.scope())
	return t, nil
}

func GetTask(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	others := []bson.M{} // other tasks in the project, for the dependency picker
	for _, o := range utils.FindAll[models.Task]("tasks", bson.M{"projectId": a.p.ID, "_id": bson.M{"$ne": a.t.ID}}, utils.SortBy("title", 1)) {
		others = append(others, bson.M{"id": o.ID, "title": o.Title, "status": o.Status})
	}
	people := append(append(services.MemberIDs(a.p.Members), a.t.Assignees...), a.t.CreatedBy, a.ws.AdminID, a.p.OwnerID)
	return bson.M{"task": a.t, "overdue": a.t.IsOverdue(), "projectTasks": others, "users": utils.UsersByID(people),
		"project":  bson.M{"id": a.p.ID, "name": a.p.Name, "members": services.MemberIDs(a.p.Members), "adminId": a.ws.AdminID},
		"activity": services.RecentActivity(bson.M{"taskId": a.t.ID}, 100), "myRole": a.role, "canEdit": a.canEdit(u), "canManage": a.canManage(u)}, nil
}

// UpdateTask saves the changes, records history and notifies the people involved.
func UpdateTask(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	var in taskInput
	utils.ReadJSON(r, &in)
	if !a.canEdit(u) {
		return nil, utils.Fail(403, "you cannot edit this task")
	}
	if (in.Assignees != nil || in.Dependencies != nil) && !a.canManage(u) {
		return nil, utils.Fail(403, "only project managers or the creator can reassign tasks or change dependencies")
	}
	before := *a.t
	if err := in.apply(a); err != nil {
		return nil, err
	}
	t, now := a.t, time.Now()
	changes := bson.M{"title": t.Title, "description": t.Description, "priority": t.Priority, "priorityLevel": t.PriorityLevel, "status": t.Status,
		"startDate": t.StartDate, "dueDate": t.DueDate, "labels": t.Labels, "assignees": t.Assignees, "dependencies": t.Dependencies, "updatedAt": now}
	if t.Status != "completed" {
		t.CompletedAt, changes["completedAt"] = nil, nil
	} else if before.Status != "completed" {
		t.CompletedAt, changes["completedAt"] = &now, now
	}
	if t.DueDate != before.DueDate { // new deadline, so reminders can be sent again
		changes["reminded"], changes["overdueSent"] = false, false
	}
	utils.SetFields("tasks", t.ID, changes)

	changed := []string{}
	for label, pair := range map[string][2]string{"status": {before.Status, t.Status}, "priority": {before.Priority, t.Priority}, "due date": {before.DueDate, t.DueDate}} {
		if pair[0] != pair[1] {
			changed = append(changed, label+": "+pair[1])
		}
	}
	if before.Title != t.Title || before.Description != t.Description {
		changed = append(changed, "details edited")
	}
	if len(changed) > 0 {
		text := strings.Join(changed, ", ")
		services.AddActivity(u, "updated the task ("+text+")", a.scope())
		for _, id := range append([]models.ID{t.CreatedBy}, t.Assignees...) {
			if id != u.ID {
				services.Notify(id, u.Name+" updated \""+t.Title+"\" ("+text+")", a.link(), before.Status != t.Status)
			}
		}
	}
	for _, id := range t.Assignees {
		if !utils.HasID(before.Assignees, id) && id != u.ID {
			services.Notify(id, assignedText(u, a), a.link(), true)
		}
	}
	for _, id := range before.Assignees {
		if !utils.HasID(t.Assignees, id) && id != u.ID {
			services.Notify(id, u.Name+" removed you from task \""+t.Title+"\"", a.link(), true)
		}
	}
	if !sameIDs(before.Assignees, t.Assignees) {
		services.AddActivity(u, "changed the assignees", a.scope())
	}
	return t, nil
}

func sameIDs(x, y []models.ID) bool {
	for _, id := range x {
		if !utils.HasID(y, id) {
			return false
		}
	}
	return len(x) == len(y)
}

func DeleteTask(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	if !a.canManage(u) {
		return nil, utils.Fail(403, "only project managers or the creator can delete this task")
	}
	for _, f := range a.t.Files {
		utils.RemoveFile(f.Path)
	}
	database.Col("comments").DeleteMany(database.Ctx, bson.M{"taskId": a.t.ID})
	database.Col("tasks").UpdateMany(database.Ctx, bson.M{"dependencies": a.t.ID}, bson.M{"$pull": bson.M{"dependencies": a.t.ID}})
	database.Col("tasks").DeleteOne(database.Ctx, bson.M{"_id": a.t.ID})
	for _, id := range a.t.Assignees {
		if id != u.ID {
			services.Notify(id, u.Name+" deleted task \""+a.t.Title+"\"", "tasks.html", false)
		}
	}
	services.AddActivity(u, "deleted task \""+a.t.Title+"\"", projectScope(a.p))
	return utils.Msg("Task deleted"), nil
}
