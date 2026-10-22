package handlers

import (
	"net/http"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SaveProjectMember adds a workspace member to the project, or changes their role.
func SaveProjectMember(r *http.Request, u *models.User) (any, error) {
	p, ws, role, err := services.LoadProject(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	var in struct{ UserID, Role string }
	utils.ReadJSON(r, &in)
	id := utils.ToID(in.UserID)
	switch {
	case role != "manager":
		return nil, utils.Fail(403, "only project managers can manage members")
	case !utils.Contains([]string{"manager", "contributor", "viewer"}, in.Role):
		return nil, utils.Fail(400, "role must be manager, contributor or viewer")
	case services.MemberRole(ws.Members, id) == "" && id != ws.AdminID:
		return nil, utils.Fail(400, "add this person to the workspace first")
	}
	isNew := services.MemberRole(p.Members, id) == ""
	utils.SetFields("projects", p.ID, bson.M{"members": services.SetMember(p.Members, id, in.Role)})
	services.Notify(id, u.Name+" set your role in project "+p.Name+" to "+in.Role, "project.html?id="+p.ID.Hex(), isNew)
	services.AddActivity(u, "set a member's role in project "+p.Name+" to "+in.Role, projectScope(p))
	return utils.Msg("Member saved"), nil
}

// RemoveProjectMember also unassigns them from the project's tasks.
func RemoveProjectMember(r *http.Request, u *models.User) (any, error) {
	p, _, role, err := services.LoadProject(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	id := utils.PathID(r, "userId")
	if role != "manager" {
		return nil, utils.Fail(403, "only project managers can manage members")
	}
	if services.MemberRole(p.Members, id) == "" || id == p.OwnerID {
		return nil, utils.Fail(400, "member not found, or is the owner")
	}
	utils.SetFields("projects", p.ID, bson.M{"members": services.WithoutMember(p.Members, id)})
	database.Col("tasks").UpdateMany(database.Ctx, bson.M{"projectId": p.ID, "assignees": id}, bson.M{"$pull": bson.M{"assignees": id}})
	services.Notify(id, "You were removed from project "+p.Name, "projects.html", true)
	services.AddActivity(u, "removed a member from project "+p.Name, projectScope(p))
	return utils.Msg("Member removed"), nil
}
