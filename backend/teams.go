package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type AccessRequest struct {
	ID        ID         `bson:"_id" json:"id"`
	UserID    ID         `bson:"userId" json:"userId"`
	AdminID   ID         `bson:"adminId" json:"adminId"`
	Message   string     `bson:"message" json:"message"`
	Status    string     `bson:"status" json:"status"` // pending | approved | rejected
	Note      string     `bson:"note" json:"note"`
	CreatedAt time.Time  `bson:"createdAt" json:"createdAt"`
	DecidedAt *time.Time `bson:"decidedAt,omitempty" json:"decidedAt,omitempty"`
	User      *Brief     `bson:"-" json:"user,omitempty"`
	Admin     *Brief     `bson:"-" json:"admin,omitempty"`
}

func search(q string) M { return M{"$regex": regexp.QuoteMeta(strings.TrimSpace(q)), "$options": "i"} }

// GET /api/admins (public) — administrators users can choose to work under.
func listAdmins(w http.ResponseWriter, r *http.Request) {
	f := M{"role": "admin", "active": true}
	if q := r.URL.Query().Get("q"); q != "" {
		f["name"] = search(q)
	}
	list := findAll[Brief](r.Context(), "users", f, bson.D{{Key: "name", Value: 1}}, 100)
	for i := range list {
		list[i].Email = "" // keep admin emails private on the public list
	}
	ok(w, list)
}

func createRequest(r *http.Request, u *User, adminID ID, message string) string {
	c := r.Context()
	admin, found := findOne[User](c, "users", M{"_id": adminID, "role": "admin", "active": true})
	if !found {
		return "administrator not found"
	}
	if u.AdminID != nil && *u.AdminID == adminID {
		return "you already work under this administrator"
	}
	if n, _ := col("requests").CountDocuments(c, M{"userId": u.ID, "status": "pending"}); n > 0 {
		return "you already have a pending request"
	}
	req := AccessRequest{ID: bson.NewObjectID(), UserID: u.ID, AdminID: adminID, Message: strings.TrimSpace(message), Status: "pending", CreatedAt: time.Now()}
	if _, err := col("requests").InsertOne(c, req); err != nil {
		return "could not submit request"
	}
	notify(adminID, "request", u.Name+" ("+u.Email+") requested to join your team", "requests.html", true)
	activity(u, "requested to join "+admin.Name+"'s team", "request", req.ID, nil, nil, &adminID, "")
	return ""
}

func submitRequest(w http.ResponseWriter, r *http.Request, u *User) {
	var p struct{ AdminID, Message string }
	if !body(w, r, &p) {
		return
	}
	id, good := oid(p.AdminID)
	if !good {
		fail(w, 400, "choose an administrator")
		return
	}
	if u.Role == "admin" || u.Role == "sysadmin" {
		fail(w, 400, "administrators do not need to join a team")
		return
	}
	if e := createRequest(r, u, id, p.Message); e != "" {
		fail(w, 400, e)
		return
	}
	send(w, 201, M{"success": true, "message": "Request sent to the administrator"})
}

func fillRequests(r *http.Request, list []AccessRequest) []AccessRequest {
	ids := []ID{}
	for _, q := range list {
		ids = append(ids, q.UserID, q.AdminID)
	}
	m := map[ID]Brief{}
	for _, b := range briefs(r.Context(), uniq(ids)) {
		m[b.ID] = b
	}
	for i := range list {
		a, b := m[list[i].UserID], m[list[i].AdminID]
		list[i].User, list[i].Admin = &a, &b
	}
	return list
}

func myRequests(w http.ResponseWriter, r *http.Request, u *User) {
	ok(w, fillRequests(r, findAll[AccessRequest](r.Context(), "requests", M{"userId": u.ID}, bson.D{{Key: "createdAt", Value: -1}}, 50)))
}

func incomingRequests(w http.ResponseWriter, r *http.Request, u *User) {
	f := M{}
	if u.Role == "admin" {
		f["adminId"] = u.ID
	}
	if s := r.URL.Query().Get("status"); in(s, "pending", "approved", "rejected") {
		f["status"] = s
	}
	page, limit := paging(r)
	ok(w, paginate(fillRequests(r, findAll[AccessRequest](r.Context(), "requests", f, bson.D{{Key: "createdAt", Value: -1}}, 1000)), page, limit))
}

func decideRequest(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	var p struct{ Decision, Note string }
	if !body(w, r, &p) {
		return
	}
	if !in(p.Decision, "approve", "reject") {
		fail(w, 400, "decision must be approve or reject")
		return
	}
	c := r.Context()
	req, found := findOne[AccessRequest](c, "requests", M{"_id": id})
	if !found || (u.Role == "admin" && req.AdminID != u.ID) {
		fail(w, 404, "request not found")
		return
	}
	if req.Status != "pending" {
		fail(w, 400, "this request has already been decided")
		return
	}
	status, now := map[string]string{"approve": "approved", "reject": "rejected"}[p.Decision], time.Now()
	col("requests").UpdateOne(c, M{"_id": id}, M{"$set": M{"status": status, "note": p.Note, "decidedAt": now}})
	admin, _ := findOne[User](c, "users", M{"_id": req.AdminID})
	if status == "approved" {
		applicant, _ := findOne[User](c, "users", M{"_id": req.UserID})
		if applicant != nil && applicant.AdminID != nil && *applicant.AdminID != req.AdminID {
			removeFromScope(r, req.UserID, *applicant.AdminID) // moving teams: leave old workspaces
		}
		col("users").UpdateOne(c, M{"_id": req.UserID}, M{"$set": M{"adminId": req.AdminID}})
	}
	text := "Your request to join " + admin.Name + "'s team was " + status
	if p.Note != "" {
		text += ": " + p.Note
	}
	notify(req.UserID, "request", text, "requests.html", true)
	activity(u, status+" an access request", "request", id, nil, nil, &req.AdminID, "")
	msg(w, "Request "+status)
}

// removeFromScope takes a user out of every workspace and project under an admin.
func removeFromScope(r *http.Request, userID, adminID ID) {
	c := r.Context()
	key := "roles." + userID.Hex()
	col("workspaces").UpdateMany(c, M{"adminId": adminID, "memberIds": userID}, M{"$pull": M{"memberIds": userID}, "$unset": M{key: ""}})
	col("projects").UpdateMany(c, M{"adminId": adminID, "memberIds": userID}, M{"$pull": M{"memberIds": userID}, "$unset": M{key: ""}})
	col("tasks").UpdateMany(c, M{"adminId": adminID, "assignees": userID}, M{"$pull": M{"assignees": userID}})
}

// GET /api/team — admins see their team; members see teammates (used for pickers).
func listTeam(w http.ResponseWriter, r *http.Request, u *User) {
	f := M{}
	switch u.Role {
	case "sysadmin":
		if a, good := oid(r.URL.Query().Get("adminId")); good {
			f["$or"] = []M{{"adminId": a}, {"_id": a}}
		}
	case "admin":
		f["$or"] = []M{{"adminId": u.ID}, {"_id": u.ID}}
	default:
		f["$or"] = []M{{"adminId": *u.AdminID}, {"_id": *u.AdminID}}
	}
	if q := r.URL.Query().Get("q"); q != "" {
		f["name"] = search(q)
	}
	ok(w, findAll[User](r.Context(), "users", f, bson.D{{Key: "name", Value: 1}}, 500))
}

// PUT /api/team/{id} — admin manages a team member: role (member/manager), active, remove.
func updateTeamMember(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	var p struct {
		Role   string
		Active *bool
		Remove bool
	}
	if !body(w, r, &p) {
		return
	}
	c := r.Context()
	m, found := findOne[User](c, "users", M{"_id": id})
	if !found || m.AdminID == nil || (u.Role == "admin" && *m.AdminID != u.ID) || in(m.Role, "admin", "sysadmin") {
		fail(w, 404, "team member not found")
		return
	}
	set := M{}
	if p.Role != "" {
		if !in(p.Role, "member", "manager") {
			fail(w, 403, "administrators can only assign member or manager roles")
			return
		}
		set["role"] = p.Role
	}
	if p.Active != nil {
		set["active"] = *p.Active
	}
	if p.Remove {
		removeFromScope(r, id, *m.AdminID)
		col("users").UpdateOne(c, M{"_id": id}, M{"$unset": M{"adminId": ""}})
		notify(id, "team", "You were removed from "+u.Name+"'s team", "requests.html", true)
		activity(u, "removed "+m.Name+" from the team", "user", id, nil, nil, m.AdminID, "")
		msg(w, "Member removed from team")
		return
	}
	if len(set) == 0 {
		fail(w, 400, "nothing to update")
		return
	}
	col("users").UpdateOne(c, M{"_id": id}, M{"$set": set})
	activity(u, "updated team member "+m.Name, "user", id, nil, nil, m.AdminID, describe(set))
	msg(w, "Team member updated")
}

func describe(set M) string {
	parts := []string{}
	for k, v := range set {
		parts = append(parts, k+": "+strings.Trim(strings.ReplaceAll(fmt.Sprint(v), "\n", " "), " "))
	}
	return strings.Join(parts, ", ")
}

// ---------- system administration (sysadmin only) ----------

func adminUsers(w http.ResponseWriter, r *http.Request, u *User) {
	q := r.URL.Query()
	f := M{}
	if s := q.Get("q"); s != "" {
		f["$or"] = []M{{"name": search(s)}, {"email": search(s)}}
	}
	if role := q.Get("role"); role != "" {
		f["role"] = role
	}
	if st := q.Get("status"); st == "active" || st == "inactive" {
		f["active"] = st == "active"
	}
	page, limit := paging(r)
	ok(w, paginate(findAll[User](r.Context(), "users", f, bson.D{{Key: "createdAt", Value: -1}}, 5000), page, limit))
}

func adminUserDetail(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	c := r.Context()
	target, found := findOne[User](c, "users", M{"_id": id})
	if !found {
		fail(w, 404, "user not found")
		return
	}
	var admin *Brief
	if target.AdminID != nil {
		admin, _ = findOne[Brief](c, "users", M{"_id": *target.AdminID})
	}
	tasks, _ := col("tasks").CountDocuments(c, M{"assignees": id})
	team, _ := col("users").CountDocuments(c, M{"adminId": id})
	ok(w, M{"user": target, "admin": admin, "assignedTasks": tasks, "teamSize": team,
		"activity": findAll[Activity](c, "activities", M{"actorId": id}, bson.D{{Key: "createdAt", Value: -1}}, 20)})
}

func adminUpdateUser(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	var p struct {
		Role   string
		Active *bool
	}
	if !body(w, r, &p) {
		return
	}
	if id == u.ID {
		fail(w, 400, "you cannot change your own role or status")
		return
	}
	c := r.Context()
	target, found := findOne[User](c, "users", M{"_id": id})
	if !found {
		fail(w, 404, "user not found")
		return
	}
	set := M{}
	if p.Role != "" {
		if !in(p.Role, "member", "manager", "admin") {
			fail(w, 400, "role must be member, manager or admin")
			return
		}
		set["role"] = p.Role
	}
	if p.Active != nil {
		set["active"] = *p.Active
	}
	if len(set) == 0 {
		fail(w, 400, "nothing to update")
		return
	}
	update := M{"$set": set}
	if p.Role == "admin" && target.AdminID != nil { // a new admin leads their own team
		removeFromScope(r, id, *target.AdminID)
		update["$unset"] = M{"adminId": ""}
	}
	if target.Role == "admin" && p.Role != "" && p.Role != "admin" { // demoted admin: team members become unassigned
		col("users").UpdateMany(c, M{"adminId": id}, M{"$unset": M{"adminId": ""}})
	}
	col("users").UpdateOne(c, M{"_id": id}, update)
	sysLog("info", u.Email+" updated user "+target.Email+" ("+describe(set)+")")
	activity(u, "updated user "+target.Name, "user", id, nil, nil, nil, describe(set))
	notify(id, "account", "Your account was updated by a system administrator ("+describe(set)+")", "profile.html", true)
	msg(w, "User updated")
}

func adminLogs(w http.ResponseWriter, r *http.Request, u *User) {
	f := M{}
	if l := r.URL.Query().Get("level"); l != "" {
		f["level"] = l
	}
	page, limit := paging(r)
	ok(w, paginate(findAll[M](r.Context(), "logs", f, bson.D{{Key: "createdAt", Value: -1}}, 1000), page, limit))
}
