package handlers

import (
	"net/http"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SaveWorkspaceMember adds (invites) someone from the admin's team, or changes their role.
func SaveWorkspaceMember(r *http.Request, u *models.User) (any, error) {
	ws, role, err := loadWorkspace(r, u)
	if err != nil {
		return nil, err
	}
	if !services.CanManage(role) {
		return nil, utils.Fail(403, "you cannot manage members of this workspace")
	}
	var in struct{ UserID, Role string }
	utils.ReadJSON(r, &in)
	id := utils.ToID(in.UserID)
	if in.Role != "manager" && in.Role != "member" {
		return nil, utils.Fail(400, "role must be manager or member")
	}
	if id == ws.OwnerID {
		return nil, utils.Fail(400, "the owner's role cannot be changed here")
	}
	person, err := utils.FindUser(id)
	if err != nil {
		return nil, utils.Fail(400, "user not found")
	}
	inTeam := person.Active && (person.ID == ws.AdminID || (person.AdminID != nil && *person.AdminID == ws.AdminID))
	if !inTeam && u.Role != "sysadmin" {
		return nil, utils.Fail(400, "this user is not in the administrator's team")
	}
	isNew := services.MemberRole(ws.Members, id) == ""
	utils.SetFields("workspaces", ws.ID, bson.M{"members": services.SetMember(ws.Members, id, in.Role)})
	if isNew {
		services.Notify(id, u.Name+" added you to workspace "+ws.Name, "workspace.html?id="+ws.ID.Hex(), true)
	}
	services.AddActivity(u, "set "+person.Name+" as "+in.Role+" in workspace "+ws.Name, wsScope(ws))
	return utils.Msg("Member saved"), nil
}

// RemoveWorkspaceMember also removes them from the workspace's projects and tasks.
func RemoveWorkspaceMember(r *http.Request, u *models.User) (any, error) {
	ws, role, err := loadWorkspace(r, u)
	if err != nil {
		return nil, err
	}
	id := utils.PathID(r, "userId")
	if !services.CanManage(role) {
		return nil, utils.Fail(403, "you cannot manage members of this workspace")
	}
	if services.MemberRole(ws.Members, id) == "" || id == ws.OwnerID {
		return nil, utils.Fail(400, "member not found, or is the owner")
	}
	utils.SetFields("workspaces", ws.ID, bson.M{"members": services.WithoutMember(ws.Members, id)})
	for _, p := range utils.FindAll[models.Project]("projects", bson.M{"workspaceId": ws.ID, "members.userId": id}) {
		utils.SetFields("projects", p.ID, bson.M{"members": services.WithoutMember(p.Members, id)})
	}
	database.Col("tasks").UpdateMany(database.Ctx, bson.M{"workspaceId": ws.ID, "assignees": id}, bson.M{"$pull": bson.M{"assignees": id}})
	services.Notify(id, "You were removed from workspace "+ws.Name, "workspaces.html", true)
	services.AddActivity(u, "removed a member from workspace "+ws.Name, wsScope(ws))
	return utils.Msg("Member removed"), nil
}
