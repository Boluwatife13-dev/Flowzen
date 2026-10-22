package handlers

import (
	"net/http"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ListTeam: admins see their team, members see their teammates, the system admin sees everyone.
func ListTeam(r *http.Request, u *models.User) (any, error) {
	filter := bson.M{}
	if u.Role != "sysadmin" {
		admin := u.ID
		if u.Role != "admin" {
			admin = *u.AdminID
		}
		filter["$or"] = []bson.M{{"adminId": admin}, {"_id": admin}}
	}
	return utils.FindAll[models.User]("users", filter, utils.SortBy("name", 1)), nil
}

// UpdateTeamMember lets an admin change a team member's role (member, manager or admin),
// activate or deactivate them, or remove them from the team.
// A member who is made an admin leaves the team and leads their own team.
func UpdateTeamMember(r *http.Request, u *models.User) (any, error) {
	var in struct {
		Role   string
		Active *bool
		Remove bool
	}
	utils.ReadJSON(r, &in)
	member, err := utils.FindUser(utils.PathID(r, "id"))
	if err != nil || member.AdminID == nil || (u.Role == "admin" && *member.AdminID != u.ID) {
		return nil, utils.Fail(404, "team member not found")
	}
	if in.Remove {
		services.RemoveFromTeam(member.ID, *member.AdminID)
		database.Col("users").UpdateOne(database.Ctx, bson.M{"_id": member.ID}, bson.M{"$unset": bson.M{"adminId": ""}})
		services.Notify(member.ID, "You were removed from "+u.Name+"'s team", "requests.html", true)
		services.AddActivity(u, "removed "+member.Name+" from the team", services.Scope{AdminID: member.AdminID})
		return utils.Msg("Member removed"), nil
	}
	if in.Role == "admin" {
		services.RemoveFromTeam(member.ID, *member.AdminID)
		database.Col("users").UpdateOne(database.Ctx, bson.M{"_id": member.ID},
			bson.M{"$set": bson.M{"role": "admin"}, "$unset": bson.M{"adminId": ""}})
		database.Log("info", u.Email+" made "+member.Email+" an admin")
		services.Notify(member.ID, u.Name+" made you an administrator. You can now lead your own team.", "dashboard.html", true)
		services.AddActivity(u, "made "+member.Name+" an admin", services.Scope{AdminID: member.AdminID})
		return utils.Msg(member.Name + " is now an admin"), nil
	}
	changes := bson.M{}
	if in.Role != "" {
		if in.Role != "member" && in.Role != "manager" {
			return nil, utils.Fail(400, "role must be member, manager or admin")
		}
		changes["role"] = in.Role
	}
	if in.Active != nil {
		changes["active"] = *in.Active
	}
	if len(changes) == 0 {
		return nil, utils.Fail(400, "nothing to update")
	}
	utils.SetFields("users", member.ID, changes)
	services.AddActivity(u, "updated team member "+member.Name, services.Scope{AdminID: member.AdminID})
	return utils.Msg("Team member updated"), nil
}