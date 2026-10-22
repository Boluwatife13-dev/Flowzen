package services

import (
	"time"

	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SubmitRequest saves a request to join an admin's team and tells the admin.
func SubmitRequest(u *models.User, adminID models.ID, message string) error {
	admin, err := utils.FindUser(adminID)
	if err != nil || admin.Role != "admin" || !admin.Active {
		return utils.Fail(400, "administrator not found")
	}
	if u.AdminID != nil && *u.AdminID == adminID {
		return utils.Fail(400, "you already work under this administrator")
	}
	if n, _ := database.Col("requests").CountDocuments(database.Ctx, bson.M{"userId": u.ID, "status": "pending"}); n > 0 {
		return utils.Fail(400, "you already have a pending request")
	}
	database.Col("requests").InsertOne(database.Ctx, models.Request{ID: models.NewID(), UserID: u.ID, AdminID: adminID,
		Message: message, Status: "pending", CreatedAt: time.Now()})
	Notify(adminID, u.Name+" ("+u.Email+") wants to join your team", "requests.html", true)
	AddActivity(u, "requested to join "+admin.Name+"'s team", Scope{AdminID: &adminID})
	return nil
}

// RemoveFromTeam takes a user out of every workspace, project and task of an admin's team.
func RemoveFromTeam(userID, adminID models.ID) {
	for _, ws := range utils.FindAll[models.Workspace]("workspaces", bson.M{"adminId": adminID, "members.userId": userID}) {
		utils.SetFields("workspaces", ws.ID, bson.M{"members": WithoutMember(ws.Members, userID)})
	}
	for _, p := range utils.FindAll[models.Project]("projects", bson.M{"adminId": adminID, "members.userId": userID}) {
		utils.SetFields("projects", p.ID, bson.M{"members": WithoutMember(p.Members, userID)})
	}
	database.Col("tasks").UpdateMany(database.Ctx, bson.M{"adminId": adminID, "assignees": userID}, bson.M{"$pull": bson.M{"assignees": userID}})
}
