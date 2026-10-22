package handlers

import (
	"net/http"

	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Dashboard: my task numbers, my open tasks, latest notifications, and the team report.
func Dashboard(r *http.Request, u *models.User) (any, error) {
	stats := map[string]int{}
	open := []models.Task{}
	for _, t := range utils.FindAll[models.Task]("tasks", bson.M{"assignees": u.ID}, utils.SortBy("dueDate", 1)) {
		stats["total"]++
		stats[t.Status]++
		if t.IsOverdue() {
			stats["overdue"]++
		}
		if t.Status != "completed" && len(open) < 8 {
			open = append(open, t)
		}
	}
	result := bson.M{"approved": u.IsApproved(), "myStats": stats, "myTasks": open,
		"notifications": utils.FindAll[models.Notification]("notifications", bson.M{"userId": u.ID}, utils.SortBy("createdAt", -1).SetLimit(5))}
	if u.IsApproved() {
		result["report"] = buildReport(u, "", "", "")
	}
	return result, nil
}

// ListActivity: recent activity the user is allowed to see.
func ListActivity(r *http.Request, u *models.User) (any, error) {
	projects, _ := services.VisibleProjects(u)
	return utils.FindPage[models.Activity](r, "activities", services.ActivityFilter(u, projects), bson.D{{Key: "createdAt", Value: -1}}), nil
}
