package handlers

import (
	"strings"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var priorityLevels = map[string]int{"low": 1, "medium": 2, "high": 3, "urgent": 4}

// taskAccess holds a task with its project, workspace and the user's project role.
type taskAccess struct {
	t    *models.Task
	p    *models.Project
	ws   *models.Workspace
	role string
}

func (a taskAccess) scope() services.Scope {
	return services.Scope{TaskID: &a.t.ID, ProjectID: &a.p.ID, WorkspaceID: &a.ws.ID, AdminID: &a.ws.AdminID}
}
func (a taskAccess) link() string { return "task.html?id=" + a.t.ID.Hex() }

// canManage: project managers and the creator. canEdit: those plus the assignees.
func (a taskAccess) canManage(u *models.User) bool {
	return a.role == "manager" || a.t.CreatedBy == u.ID
}
func (a taskAccess) canEdit(u *models.User) bool {
	return a.canManage(u) || utils.HasID(a.t.Assignees, u.ID)
}

// loadTask finds a task and checks the user can see its project.
func loadTask(u *models.User, id models.ID) (taskAccess, error) {
	t, err := utils.FindOne[models.Task]("tasks", bson.M{"_id": id})
	if err != nil {
		return taskAccess{}, err
	}
	p, ws, role, err := services.LoadProject(u, t.ProjectID)
	return taskAccess{t, p, ws, role}, err
}

// taskInput uses pointers so an update only changes the fields that were sent.
type taskInput struct {
	ProjectID                                                string
	Title, Description, Priority, Status, StartDate, DueDate *string
	Assignees, Labels, Dependencies                          *[]string
}

// apply validates the input and copies it onto the task.
func (in *taskInput) apply(a taskAccess) error {
	t := a.t
	for field, value := range map[*string]*string{&t.Title: in.Title, &t.Description: in.Description, &t.StartDate: in.StartDate, &t.DueDate: in.DueDate} {
		if value != nil {
			*field = strings.TrimSpace(*value)
		}
	}
	if in.Priority != nil {
		t.Priority, t.PriorityLevel = *in.Priority, priorityLevels[*in.Priority]
	}
	if in.Status != nil {
		t.Status = *in.Status
	}
	switch {
	case len(t.Title) < 2:
		return utils.Fail(400, "task title is too short")
	case t.PriorityLevel == 0:
		return utils.Fail(400, "priority must be low, medium, high or urgent")
	case !utils.Contains([]string{"pending", "in_progress", "review", "blocked", "completed"}, t.Status):
		return utils.Fail(400, "invalid status")
	case !utils.ValidDate(t.StartDate) || !utils.ValidDate(t.DueDate):
		return utils.Fail(400, "dates must look like 2026-12-31")
	case t.StartDate != "" && t.DueDate != "" && t.DueDate < t.StartDate:
		return utils.Fail(400, "the due date cannot be before the start date")
	}
	if in.Labels != nil {
		t.Labels = []string{}
		for _, l := range *in.Labels {
			if l = strings.ToLower(strings.TrimSpace(l)); l != "" && !utils.Contains(t.Labels, l) {
				t.Labels = append(t.Labels, l)
			}
		}
	}
	if in.Assignees != nil {
		for _, text := range *in.Assignees {
			if utils.ToID(text).IsZero() {
				return utils.Fail(400, "invalid assignee")
			}
		}
		t.Assignees = utils.ToIDs(*in.Assignees)
		for _, id := range t.Assignees {
			if person, err := utils.FindUser(id); err != nil || !person.Active || services.ProjectRole(person, a.p, a.ws) == "" {
				return utils.Fail(400, "every assignee must be a member of this project")
			}
		}
	}
	if in.Dependencies != nil {
		t.Dependencies = utils.ToIDs(*in.Dependencies)
		n, _ := database.Col("tasks").CountDocuments(database.Ctx, bson.M{"_id": bson.M{"$in": t.Dependencies}, "projectId": a.p.ID})
		if utils.HasID(t.Dependencies, t.ID) || int(n) != len(t.Dependencies) {
			return utils.Fail(400, "dependencies must be other tasks in the same project")
		}
	}
	if t.Status == "completed" && len(t.Dependencies) > 0 {
		open, _ := database.Col("tasks").CountDocuments(database.Ctx, bson.M{"_id": bson.M{"$in": t.Dependencies}, "status": bson.M{"$ne": "completed"}})
		if open > 0 {
			return utils.Fail(400, "finish the tasks this one depends on first")
		}
	}
	return nil
}
