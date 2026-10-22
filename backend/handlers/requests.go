package handlers

import (
	"net/http"

	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ListAdmins is public so people can choose an admin while signing up (emails are hidden).
func ListAdmins(r *http.Request, _ *models.User) (any, error) {
	filter := bson.M{"role": "admin", "active": true}
	if q := r.URL.Query().Get("q"); q != "" {
		filter["name"] = utils.Search(q)
	}
	admins := []bson.M{}
	for _, a := range utils.FindAll[models.User]("users", filter, utils.SortBy("name", 1)) {
		admins = append(admins, bson.M{"id": a.ID, "name": a.Name, "avatar": a.Avatar})
	}
	return admins, nil
}

func CreateRequest(r *http.Request, u *models.User) (any, error) {
	var in struct{ AdminID, Message string }
	utils.ReadJSON(r, &in)
	if u.IsAdmin() {
		return nil, utils.Fail(400, "administrators do not need to join a team")
	}
	return utils.Msg("Request sent"), services.SubmitRequest(u, utils.ToID(in.AdminID), in.Message)
}

// withUsers adds the names of everyone in the requests.
func withUsers(requests []models.Request) bson.M {
	ids := []models.ID{}
	for _, q := range requests {
		ids = append(ids, q.UserID, q.AdminID)
	}
	return bson.M{"items": requests, "users": utils.UsersByID(ids)}
}

func MyRequests(r *http.Request, u *models.User) (any, error) {
	return withUsers(utils.FindAll[models.Request]("requests", bson.M{"userId": u.ID}, utils.SortBy("createdAt", -1))), nil
}

func IncomingRequests(r *http.Request, u *models.User) (any, error) {
	filter := bson.M{}
	if u.Role == "admin" {
		filter["adminId"] = u.ID
	}
	if s := r.URL.Query().Get("status"); s != "" {
		filter["status"] = s
	}
	return withUsers(utils.FindAll[models.Request]("requests", filter, utils.SortBy("createdAt", -1).SetLimit(200))), nil
}

// DecideRequest approves or rejects a request; the user is told by notification and email.
func DecideRequest(r *http.Request, u *models.User) (any, error) {
	var in struct{ Decision, Note string }
	utils.ReadJSON(r, &in)
	status := map[string]string{"approve": "approved", "reject": "rejected"}[in.Decision]
	if status == "" {
		return nil, utils.Fail(400, "decision must be approve or reject")
	}
	req, err := utils.FindOne[models.Request]("requests", bson.M{"_id": utils.PathID(r, "id")})
	if err != nil || (u.Role == "admin" && req.AdminID != u.ID) {
		return nil, utils.Fail(404, "request not found")
	}
	if req.Status != "pending" {
		return nil, utils.Fail(400, "this request was already decided")
	}
	utils.SetFields("requests", req.ID, bson.M{"status": status, "note": in.Note})
	if status == "approved" {
		applicant, _ := utils.FindUser(req.UserID)
		if applicant.AdminID != nil && *applicant.AdminID != req.AdminID {
			services.RemoveFromTeam(applicant.ID, *applicant.AdminID) // moving to a new team
		}
		utils.SetFields("users", req.UserID, bson.M{"adminId": req.AdminID})
	}
	admin, _ := utils.FindUser(req.AdminID)
	text := "Your request to join " + admin.Name + "'s team was " + status
	if in.Note != "" {
		text += ": " + in.Note
	}
	services.Notify(req.UserID, text, "requests.html", true)
	services.AddActivity(u, status+" an access request", services.Scope{AdminID: &req.AdminID})
	return utils.Msg("Request " + status), nil
}
