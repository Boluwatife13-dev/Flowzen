package main

import (
	"context"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Project struct {
	ID          ID                `bson:"_id" json:"id"`
	WorkspaceID ID                `bson:"workspaceId" json:"workspaceId"`
	AdminID     ID                `bson:"adminId" json:"adminId"`
	OwnerID     ID                `bson:"ownerId" json:"ownerId"`
	Name        string            `bson:"name" json:"name"`
	Description string            `bson:"description" json:"description"`
	Status      string            `bson:"status" json:"status"` // planning | active | on_hold | completed | cancelled
	StartDate   string            `bson:"startDate" json:"startDate"`
	Deadline    string            `bson:"deadline" json:"deadline"`
	MemberIDs   []ID              `bson:"memberIds" json:"memberIds"`
	Roles       map[string]string `bson:"roles" json:"roles"` // userId -> manager | contributor | viewer
	CreatedAt   time.Time         `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time         `bson:"updatedAt" json:"updatedAt"`
}

var projectStatuses = []string{"planning", "active", "on_hold", "completed", "cancelled"}

// projRole: "manager" | "contributor" | "viewer" | "" (no access).
func projRole(u *User, p *Project, ws *Workspace) string {
	if canManageWS(u, ws) || p.OwnerID == u.ID {
		return "manager"
	}
	if containsID(p.MemberIDs, u.ID) {
		return p.Roles[u.ID.Hex()]
	}
	return ""
}

func loadProject(w http.ResponseWriter, r *http.Request, u *User, id ID, manage bool) (*Project, *Workspace, string, bool) {
	c := r.Context()
	p, found := findOne[Project](c, "projects", M{"_id": id})
	var ws *Workspace
	if found {
		ws, found = findOne[Workspace](c, "workspaces", M{"_id": p.WorkspaceID})
	}
	role := ""
	if found {
		role = projRole(u, p, ws)
	}
	if role == "" {
		fail(w, 404, "project not found")
		return nil, nil, "", false
	}
	if manage && role != "manager" {
		fail(w, 403, "you cannot manage this project")
		return nil, nil, "", false
	}
	return p, ws, role, true
}

func visibleProjects(c context.Context, u *User) ([]Project, map[ID]*Workspace) {
	wsMap := map[ID]*Workspace{}
	ids := []ID{}
	for _, ws := range visibleWorkspaces(c, u) {
		ws := ws
		wsMap[ws.ID] = &ws
		ids = append(ids, ws.ID)
	}
	out := []Project{}
	if len(ids) == 0 {
		return out, wsMap
	}
	for _, p := range findAll[Project](c, "projects", M{"workspaceId": M{"$in": ids}}, bson.D{{Key: "createdAt", Value: -1}}, 0) {
		if projRole(u, &p, wsMap[p.WorkspaceID]) != "" {
			out = append(out, p)
		}
	}
	return out, wsMap
}

func progress(c context.Context, pid ID) M {
	total, _ := col("tasks").CountDocuments(c, M{"projectId": pid})
	done, _ := col("tasks").CountDocuments(c, M{"projectId": pid, "status": "completed"})
	overdue, _ := col("tasks").CountDocuments(c, M{"projectId": pid, "status": M{"$ne": "completed"}, "dueDate": M{"$lt": today(), "$gt": ""}})
	pct := 0
	if total > 0 {
		pct = int(done * 100 / total)
	}
	return M{"total": total, "completed": done, "overdue": overdue, "percent": pct}
}

func listProjects(w http.ResponseWriter, r *http.Request, u *User) {
	c, q := r.Context(), r.URL.Query()
	list, wsMap := visibleProjects(c, u)
	search, status, wsID := strings.ToLower(q.Get("q")), q.Get("status"), q.Get("workspaceId")
	out := []M{}
	for _, p := range list {
		if (search != "" && !strings.Contains(strings.ToLower(p.Name+" "+p.Description), search)) || (status != "" && p.Status != status) || (wsID != "" && p.WorkspaceID.Hex() != wsID) {
			continue
		}
		out = append(out, M{"project": p, "workspace": wsMap[p.WorkspaceID].Name, "progress": progress(c, p.ID), "myRole": projRole(u, &p, wsMap[p.WorkspaceID])})
	}
	key, desc := q.Get("sort"), q.Get("order") == "desc"
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i]["project"].(Project), out[j]["project"].(Project)
		var less bool
		switch key {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "deadline":
			less = a.Deadline < b.Deadline
		case "progress":
			less = out[i]["progress"].(M)["percent"].(int) < out[j]["progress"].(M)["percent"].(int)
		default:
			less = a.CreatedAt.Before(b.CreatedAt)
		}
		if desc {
			return !less
		}
		return less
	})
	page, limit := paging(r)
	ok(w, paginate(out, page, limit))
}

type projectInput struct {
	WorkspaceID, Name, Description, Status, StartDate, Deadline, OwnerID string
}

func (p *projectInput) check() string {
	p.Name = strings.TrimSpace(p.Name)
	if p.Status == "" {
		p.Status = "planning"
	}
	switch {
	case len(p.Name) < 2 || len(p.Name) > 120:
		return "project name must be 2-120 characters"
	case !in(p.Status, projectStatuses...):
		return "invalid project status"
	case !validDate(p.StartDate) || !validDate(p.Deadline):
		return "dates must be in YYYY-MM-DD format"
	case p.StartDate != "" && p.Deadline != "" && p.Deadline < p.StartDate:
		return "deadline cannot be before the start date"
	}
	return ""
}

func createProject(w http.ResponseWriter, r *http.Request, u *User) {
	var p projectInput
	if !body(w, r, &p) {
		return
	}
	if e := p.check(); e != "" {
		fail(w, 400, e)
		return
	}
	c := r.Context()
	wsID, _ := oid(p.WorkspaceID)
	ws, found := findOne[Workspace](c, "workspaces", M{"_id": wsID})
	if !found || !canManageWS(u, ws) {
		fail(w, 403, "you cannot create projects in this workspace")
		return
	}
	owner := u.ID
	if p.OwnerID != "" {
		o, good := oid(p.OwnerID)
		if !good || !(containsID(ws.MemberIDs, o) || o == ws.AdminID) {
			fail(w, 400, "project owner must be a workspace member")
			return
		}
		owner = o
	}
	now := time.Now()
	pr := Project{ID: bson.NewObjectID(), WorkspaceID: ws.ID, AdminID: ws.AdminID, OwnerID: owner, Name: p.Name, Description: p.Description, Status: p.Status,
		StartDate: p.StartDate, Deadline: p.Deadline, MemberIDs: uniq([]ID{owner, u.ID}), Roles: map[string]string{owner.Hex(): "manager", u.ID.Hex(): "manager"}, CreatedAt: now, UpdatedAt: now}
	if _, err := col("projects").InsertOne(c, pr); err != nil {
		fail(w, 500, "could not create project")
		return
	}
	if owner != u.ID {
		notify(owner, "project", u.Name+" made you owner of project "+pr.Name, "project.html?id="+pr.ID.Hex(), true)
	}
	activity(u, "created project "+pr.Name, "project", pr.ID, &pr.ID, &ws.ID, &ws.AdminID, "")
	send(w, 201, M{"success": true, "data": pr})
}

func getProject(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	p, ws, role, good := loadProject(w, r, u, id, false)
	if !good {
		return
	}
	c := r.Context()
	ok(w, M{"project": p, "workspace": M{"id": ws.ID, "name": ws.Name}, "members": briefs(c, p.MemberIDs), "wsMembers": briefs(c, uniq(append(ws.MemberIDs, ws.AdminID))),
		"progress": progress(c, p.ID), "myRole": role, "canManage": role == "manager",
		"activity": findAll[Activity](c, "activities", M{"projectId": p.ID}, bson.D{{Key: "createdAt", Value: -1}}, 25)})
}

func updateProject(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	pr, ws, _, good := loadProject(w, r, u, id, true)
	if !good {
		return
	}
	var p projectInput
	if !body(w, r, &p) {
		return
	}
	if e := p.check(); e != "" {
		fail(w, 400, e)
		return
	}
	set := M{"name": p.Name, "description": p.Description, "status": p.Status, "startDate": p.StartDate, "deadline": p.Deadline, "updatedAt": time.Now()}
	changes := []string{}
	if p.Status != pr.Status {
		changes = append(changes, "status: "+pr.Status+" → "+p.Status)
	}
	if p.Deadline != pr.Deadline {
		changes = append(changes, "deadline: "+p.Deadline)
	}
	update := M{"$set": set}
	if p.OwnerID != "" && p.OwnerID != pr.OwnerID.Hex() {
		o, good := oid(p.OwnerID)
		if !good || !(containsID(ws.MemberIDs, o) || o == ws.AdminID) {
			fail(w, 400, "project owner must be a workspace member")
			return
		}
		set["ownerId"], set["roles."+o.Hex()] = o, "manager"
		update["$addToSet"] = M{"memberIds": o}
		changes = append(changes, "new owner")
		notify(o, "project", "You are now the owner of project "+pr.Name, "project.html?id="+pr.ID.Hex(), true)
	}
	col("projects").UpdateOne(r.Context(), M{"_id": pr.ID}, update)
	if len(changes) > 0 {
		for _, m := range pr.MemberIDs {
			if m != u.ID {
				notify(m, "project", "Project "+pr.Name+" updated ("+strings.Join(changes, ", ")+")", "project.html?id="+pr.ID.Hex(), false)
			}
		}
	}
	activity(u, "updated project "+p.Name, "project", pr.ID, &pr.ID, &ws.ID, &ws.AdminID, strings.Join(changes, ", "))
	msg(w, "Project updated")
}

func purgeProject(c context.Context, pid ID) {
	for _, t := range findAll[Task](c, "tasks", M{"projectId": pid}, nil, 0) {
		for _, a := range t.Attachments {
			os.Remove(a.Path)
		}
		col("comments").DeleteMany(c, M{"taskId": t.ID})
	}
	col("tasks").DeleteMany(c, M{"projectId": pid})
	col("projects").DeleteOne(c, M{"_id": pid})
}

func deleteProject(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	p, ws, _, good := loadProject(w, r, u, id, true)
	if !good {
		return
	}
	purgeProject(r.Context(), p.ID)
	activity(u, "deleted project "+p.Name, "project", p.ID, nil, &ws.ID, &ws.AdminID, "")
	msg(w, "Project deleted")
}

func projectMember(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	p, ws, _, good := loadProject(w, r, u, id, true)
	if !good {
		return
	}
	c := r.Context()
	var b struct{ UserID, Role string }
	if r.Method != http.MethodDelete && !body(w, r, &b) {
		return
	}
	if r.Method != http.MethodPost {
		b.UserID = r.PathValue("uid")
	}
	uid, good := oid(b.UserID)
	if !good {
		fail(w, 400, "invalid user id")
		return
	}
	key, link := "roles."+uid.Hex(), "project.html?id="+p.ID.Hex()
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		if !in(b.Role, "manager", "contributor", "viewer") {
			fail(w, 400, "role must be manager, contributor or viewer")
			return
		}
		if !(containsID(ws.MemberIDs, uid) || uid == ws.AdminID) {
			fail(w, 400, "user must be a member of the workspace first")
			return
		}
		if r.Method == http.MethodPost && containsID(p.MemberIDs, uid) {
			fail(w, 409, "user is already a project member")
			return
		}
		if r.Method == http.MethodPut && !containsID(p.MemberIDs, uid) {
			fail(w, 404, "member not found")
			return
		}
		col("projects").UpdateOne(c, M{"_id": p.ID}, M{"$addToSet": M{"memberIds": uid}, "$set": M{key: b.Role}})
		notify(uid, "project", u.Name+" set your role in project "+p.Name+" to "+b.Role, link, r.Method == http.MethodPost)
		activity(u, "set a member role in project "+p.Name, "project", p.ID, &p.ID, &ws.ID, &ws.AdminID, "role: "+b.Role)
		msg(w, "Project member saved")
	case http.MethodDelete:
		if uid == p.OwnerID || !containsID(p.MemberIDs, uid) {
			fail(w, 400, "member not found or is the project owner")
			return
		}
		col("projects").UpdateOne(c, M{"_id": p.ID}, M{"$pull": M{"memberIds": uid}, "$unset": M{key: ""}})
		col("tasks").UpdateMany(c, M{"projectId": p.ID, "assignees": uid}, M{"$pull": M{"assignees": uid}})
		notify(uid, "project", "You were removed from project "+p.Name, "projects.html", true)
		activity(u, "removed a member from project "+p.Name, "project", p.ID, &p.ID, &ws.ID, &ws.AdminID, "")
		msg(w, "Member removed")
	}
}
