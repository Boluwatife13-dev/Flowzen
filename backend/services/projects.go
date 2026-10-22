package services

import (
	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// VisibleProjects returns every project the user can open, and their workspaces by id.
func VisibleProjects(u *models.User) ([]models.Project, map[models.ID]models.Workspace) {
	workspaces := map[models.ID]models.Workspace{}
	ids := []models.ID{}
	for _, ws := range utils.FindAll[models.Workspace]("workspaces", MyWorkspacesFilter(u)) {
		workspaces[ws.ID] = ws
		ids = append(ids, ws.ID)
	}
	visible := []models.Project{}
	for _, p := range utils.FindAll[models.Project]("projects", bson.M{"workspaceId": bson.M{"$in": ids}}, utils.SortBy("createdAt", -1)) {
		ws := workspaces[p.WorkspaceID]
		if ProjectRole(u, &p, &ws) != "" {
			visible = append(visible, p)
		}
	}
	return visible, workspaces
}

// Progress counts a project's tasks: total, completed, overdue and percent done.
func Progress(projectID models.ID) bson.M {
	count := func(filter bson.M) int64 {
		filter["projectId"] = projectID
		n, _ := database.Col("tasks").CountDocuments(database.Ctx, filter)
		return n
	}
	total := count(bson.M{})
	done := count(bson.M{"status": "completed"})
	overdue := count(bson.M{"status": bson.M{"$ne": "completed"}, "dueDate": bson.M{"$gt": "", "$lt": utils.Today()}})
	percent := int64(0)
	if total > 0 {
		percent = done * 100 / total
	}
	return bson.M{"total": total, "completed": done, "overdue": overdue, "percent": percent}
}

// DeleteProjectData removes a project with its tasks, comments and files.
func DeleteProjectData(projectID models.ID) {
	for _, t := range utils.FindAll[models.Task]("tasks", bson.M{"projectId": projectID}) {
		for _, f := range t.Files {
			utils.RemoveFile(f.Path)
		}
		database.Col("comments").DeleteMany(database.Ctx, bson.M{"taskId": t.ID})
	}
	database.Col("tasks").DeleteMany(database.Ctx, bson.M{"projectId": projectID})
	database.Col("projects").DeleteOne(database.Ctx, bson.M{"_id": projectID})
}
