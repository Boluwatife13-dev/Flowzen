package services

import (
	"time"

	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Scope says what an activity belongs to, so it appears in the right history.
type Scope struct{ TaskID, ProjectID, WorkspaceID, AdminID *models.ID }

func AddActivity(u *models.User, action string, s Scope) {
	database.Col("activities").InsertOne(database.Ctx, models.Activity{ID: models.NewID(), UserID: u.ID, UserName: u.Name, Action: action,
		TaskID: s.TaskID, ProjectID: s.ProjectID, WorkspaceID: s.WorkspaceID, AdminID: s.AdminID, CreatedAt: time.Now()})
}

func RecentActivity(filter bson.M, limit int64) []models.Activity {
	return utils.FindAll[models.Activity]("activities", filter, utils.SortBy("createdAt", -1).SetLimit(limit))
}

// ActivityFilter: what a user is allowed to see in activity feeds.
func ActivityFilter(u *models.User, projects []models.Project) bson.M {
	if u.Role == "sysadmin" {
		return bson.M{}
	}
	ids := []models.ID{}
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	return bson.M{"$or": []bson.M{{"userId": u.ID}, {"adminId": u.ID}, {"projectId": bson.M{"$in": ids}}}}
}
