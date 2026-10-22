package handlers

import (
	"net/http"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// AdminListUsers lists all users for the system admin (search, role and status filters).
func AdminListUsers(r *http.Request, u *models.User) (any, error) {
	q := r.URL.Query()
	filter := bson.M{}
	if s := q.Get("q"); s != "" {
		filter["$or"] = []bson.M{{"name": utils.Search(s)}, {"email": utils.Search(s)}}
	}
	if q.Get("role") != "" {
		filter["role"] = q.Get("role")
	}
	if q.Get("status") != "" {
		filter["active"] = q.Get("status") == "active"
	}
	return utils.FindPage[models.User](r, "users", filter, bson.D{{Key: "createdAt", Value: -1}}), nil
}

// AdminUpdateUser: only the system admin can make someone an admin, or (de)activate anyone.
func AdminUpdateUser(r *http.Request, u *models.User) (any, error) {
	var in struct {
		Role   string
		Active *bool
	}
	utils.ReadJSON(r, &in)
	target, err := utils.FindUser(utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	if target.ID == u.ID {
		return nil, utils.Fail(400, "you cannot change your own account here")
	}
	changes, update := bson.M{}, bson.M{}
	if in.Role != "" {
		if !utils.Contains([]string{"member", "manager", "admin"}, in.Role) {
			return nil, utils.Fail(400, "role must be member, manager or admin")
		}
		changes["role"] = in.Role
		if in.Role == "admin" && target.AdminID != nil { // a new admin leads their own team
			services.RemoveFromTeam(target.ID, *target.AdminID)
			update["$unset"] = bson.M{"adminId": ""}
		}
		if target.Role == "admin" && in.Role != "admin" { // their old team must choose a new admin
			database.Col("users").UpdateMany(database.Ctx, bson.M{"adminId": target.ID}, bson.M{"$unset": bson.M{"adminId": ""}})
		}
	}
	if in.Active != nil {
		changes["active"] = *in.Active
	}
	if len(changes) == 0 {
		return nil, utils.Fail(400, "nothing to update")
	}
	update["$set"] = changes
	database.Col("users").UpdateOne(database.Ctx, bson.M{"_id": target.ID}, update)
	database.Log("info", u.Email+" updated user "+target.Email)
	services.AddActivity(u, "updated user "+target.Name, services.Scope{})
	services.Notify(target.ID, "Your account was updated by the system administrator", "profile.html", true)
	return utils.Msg("User updated"), nil
}

// AdminLogs shows saved system events and errors.
func AdminLogs(r *http.Request, u *models.User) (any, error) {
	filter := bson.M{}
	if level := r.URL.Query().Get("level"); level != "" {
		filter["level"] = level
	}
	return utils.FindPage[models.LogEntry](r, "logs", filter, bson.D{{Key: "createdAt", Value: -1}}), nil
}
