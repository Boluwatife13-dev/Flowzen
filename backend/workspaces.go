package main

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Workspace struct {
	ID          ID                `bson:"_id" json:"id"`
	Name        string            `bson:"name" json:"name"`
	Description string            `bson:"description" json:"description"`
	OwnerID     ID                `bson:"ownerId" json:"ownerId"`
	AdminID     ID                `bson:"adminId" json:"adminId"` // administrator whose scope this workspace belongs to
	MemberIDs   []ID              `bson:"memberIds" json:"memberIds"`
	Roles       map[string]string `bson:"roles" json:"roles"` // userId -> owner | manager | member
	CreatedAt   time.Time         `bson:"createdAt" json:"createdAt"`
}

// wsRole returns the caller's effective role in a workspace ("" = no access).
func wsRole(u *User, ws *Workspace) string {
	switch {
	case u.Role == "sysadmin":
		return "sysadmin"
	case ws.AdminID == u.ID:
		return "admin"
	case ws.OwnerID == u.ID:
		return "owner"
	case containsID(ws.MemberIDs, u.ID):
		return ws.Roles[u.ID.Hex()]
	}
	return ""
}
func canManageWS(u *User, ws *Workspace) bool {
	return in(wsRole(u, ws), "sysadmin", "admin", "owner", "manager")
}

func loadWS(w http.ResponseWriter, r *http.Request, u *User, manage bool) (*Workspace, bool) {
	id, good := pathID(w, r, "id")
	if !good {
		return nil, false
	}
	ws, found := findOne[Workspace](r.Context(), "workspaces", M{"_id": id})
	if !found || wsRole(u, ws) == "" {
		fail(w, 404, "workspace not found")
		return nil, false
	}
	if manage && !canManageWS(u, ws) {
		fail(w, 403, "you cannot manage this workspace")
		return nil, false
	}
	return ws, true
}

func visibleWorkspaces(c context.Context, u *User) []Workspace {
	f := M{}
	if u.Role != "sysadmin" {
		f = M{"$or": []M{{"adminId": u.ID}, {"ownerId": u.ID}, {"memberIds": u.ID}}}
	}
	return findAll[Workspace](c, "workspaces", f, bson.D{{Key: "createdAt", Value: -1}}, 0)
}

func listWorkspaces(w http.ResponseWriter, r *http.Request, u *User) {
	list := visibleWorkspaces(r.Context(), u)
	q := strings.ToLower(r.URL.Query().Get("q"))
	out := []M{}
	for _, ws := range list {
		if q != "" && !strings.Contains(strings.ToLower(ws.Name), q) {
			continue
		}
		n, _ := col("projects").CountDocuments(r.Context(), M{"workspaceId": ws.ID})
		out = append(out, M{"workspace": ws, "projects": n, "members": len(ws.MemberIDs), "myRole": wsRole(u, &ws)})
	}
	page, limit := paging(r)
	ok(w, paginate(out, page, limit))
}

func createWorkspace(w http.ResponseWriter, r *http.Request, u *User) {
	var p struct{ Name, Description string }
	if !body(w, r, &p) {
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if len(p.Name) < 2 || len(p.Name) > 100 {
		fail(w, 400, "workspace name must be 2-100 characters")
		return
	}
	scope := u.ID
	if u.Role == "manager" {
		scope = *u.AdminID
	}
	ws := Workspace{ID: bson.NewObjectID(), Name: p.Name, Description: p.Description, OwnerID: u.ID, AdminID: scope,
		MemberIDs: []ID{u.ID}, Roles: map[string]string{u.ID.Hex(): "owner"}, CreatedAt: time.Now()}
	if _, err := col("workspaces").InsertOne(r.Context(), ws); err != nil {
		fail(w, 500, "could not create workspace")
		return
	}
	activity(u, "created workspace "+ws.Name, "workspace", ws.ID, nil, &ws.ID, &ws.AdminID, "")
	send(w, 201, M{"success": true, "data": ws})
}

func getWorkspace(w http.ResponseWriter, r *http.Request, u *User) {
	ws, good := loadWS(w, r, u, false)
	if !good {
		return
	}
	c := r.Context()
	projects := []M{}
	for _, p := range findAll[Project](c, "projects", M{"workspaceId": ws.ID}, bson.D{{Key: "createdAt", Value: -1}}, 0) {
		if projRole(u, &p, ws) != "" {
			projects = append(projects, M{"project": p, "progress": progress(c, p.ID)})
		}
	}
	ok(w, M{"workspace": ws, "members": briefs(c, ws.MemberIDs), "projects": projects, "myRole": wsRole(u, ws), "canManage": canManageWS(u, ws),
		"activity": findAll[Activity](c, "activities", M{"workspaceId": ws.ID}, bson.D{{Key: "createdAt", Value: -1}}, 20)})
}

func updateWorkspace(w http.ResponseWriter, r *http.Request, u *User) {
	ws, good := loadWS(w, r, u, true)
	if !good {
		return
	}
	var p struct{ Name, Description, OwnerID string }
	if !body(w, r, &p) {
		return
	}
	set := M{}
	if name := strings.TrimSpace(p.Name); name != "" {
		if len(name) > 100 {
			fail(w, 400, "workspace name is too long")
			return
		}
		set["name"] = name
	}
	set["description"] = p.Description
	if p.OwnerID != "" && p.OwnerID != ws.OwnerID.Hex() {
		if !in(wsRole(u, ws), "sysadmin", "admin", "owner") {
			fail(w, 403, "only the owner or administrator can transfer ownership")
			return
		}
		owner, good := oid(p.OwnerID)
		if !good || !containsID(ws.MemberIDs, owner) {
			fail(w, 400, "the new owner must be a workspace member")
			return
		}
		set["ownerId"] = owner
		set["roles."+owner.Hex()] = "owner"
		if containsID(ws.MemberIDs, ws.OwnerID) {
			set["roles."+ws.OwnerID.Hex()] = "manager"
		}
		notify(owner, "workspace", "You are now the owner of workspace "+ws.Name, "workspace.html?id="+ws.ID.Hex(), true)
	}
	col("workspaces").UpdateOne(r.Context(), M{"_id": ws.ID}, M{"$set": set})
	activity(u, "updated workspace "+ws.Name, "workspace", ws.ID, nil, &ws.ID, &ws.AdminID, "")
	msg(w, "Workspace updated")
}

func deleteWorkspace(w http.ResponseWriter, r *http.Request, u *User) {
	ws, good := loadWS(w, r, u, true)
	if !good {
		return
	}
	if !in(wsRole(u, ws), "sysadmin", "admin", "owner") {
		fail(w, 403, "only the owner or administrator can delete a workspace")
		return
	}
	c := r.Context()
	for _, p := range findAll[Project](c, "projects", M{"workspaceId": ws.ID}, nil, 0) {
		purgeProject(c, p.ID)
	}
	col("workspaces").DeleteOne(c, M{"_id": ws.ID})
	activity(u, "deleted workspace "+ws.Name, "workspace", ws.ID, nil, nil, &ws.AdminID, "")
	msg(w, "Workspace deleted")
}

// eligible: users in the workspace admin's team (or the admin) may be added.
func eligible(c context.Context, userID, adminID ID, anyone bool) (*User, bool) {
	m, found := findOne[User](c, "users", M{"_id": userID, "active": true})
	if !found || (!anyone && m.ID != adminID && (m.AdminID == nil || *m.AdminID != adminID)) {
		return nil, false
	}
	return m, true
}

func addWSMember(w http.ResponseWriter, r *http.Request, u *User) {
	ws, good := loadWS(w, r, u, true)
	if !good {
		return
	}
	var p struct{ UserID, Role string }
	if !body(w, r, &p) {
		return
	}
	if p.Role == "" {
		p.Role = "member"
	}
	id, good := oid(p.UserID)
	if !good || !in(p.Role, "manager", "member") {
		fail(w, 400, "valid user and role (manager or member) are required")
		return
	}
	m, good := eligible(r.Context(), id, ws.AdminID, u.Role == "sysadmin")
	if !good {
		fail(w, 400, "user must be an active member of this administrator's team")
		return
	}
	if containsID(ws.MemberIDs, id) {
		fail(w, 409, "user is already a member")
		return
	}
	col("workspaces").UpdateOne(r.Context(), M{"_id": ws.ID}, M{"$push": M{"memberIds": id}, "$set": M{"roles." + id.Hex(): p.Role}})
	notify(id, "workspace", u.Name+" added you to workspace "+ws.Name+" as "+p.Role, "workspace.html?id="+ws.ID.Hex(), true)
	activity(u, "added "+m.Name+" to workspace "+ws.Name, "workspace", ws.ID, nil, &ws.ID, &ws.AdminID, "role: "+p.Role)
	msg(w, "Member added")
}

func updateWSMember(w http.ResponseWriter, r *http.Request, u *User) {
	ws, good := loadWS(w, r, u, true)
	if !good {
		return
	}
	id, good := oid(r.PathValue("uid"))
	var p struct{ Role string }
	if !good || !body(w, r, &p) {
		if !good {
			fail(w, 400, "invalid user id")
		}
		return
	}
	if !in(p.Role, "manager", "member") || !containsID(ws.MemberIDs, id) || id == ws.OwnerID {
		fail(w, 400, "role must be manager or member, and the owner's role cannot be changed here")
		return
	}
	col("workspaces").UpdateOne(r.Context(), M{"_id": ws.ID}, M{"$set": M{"roles." + id.Hex(): p.Role}})
	notify(id, "workspace", "Your role in workspace "+ws.Name+" is now "+p.Role, "workspace.html?id="+ws.ID.Hex(), false)
	activity(u, "changed a member role in workspace "+ws.Name, "workspace", ws.ID, nil, &ws.ID, &ws.AdminID, "role: "+p.Role)
	msg(w, "Role updated")
}

func removeWSMember(w http.ResponseWriter, r *http.Request, u *User) {
	ws, good := loadWS(w, r, u, true)
	if !good {
		return
	}
	id, good := oid(r.PathValue("uid"))
	if !good || !containsID(ws.MemberIDs, id) {
		fail(w, 404, "member not found")
		return
	}
	if id == ws.OwnerID {
		fail(w, 400, "transfer ownership before removing the owner")
		return
	}
	c := r.Context()
	key := "roles." + id.Hex()
	col("workspaces").UpdateOne(c, M{"_id": ws.ID}, M{"$pull": M{"memberIds": id}, "$unset": M{key: ""}})
	col("projects").UpdateMany(c, M{"workspaceId": ws.ID, "memberIds": id}, M{"$pull": M{"memberIds": id}, "$unset": M{key: ""}})
	notify(id, "workspace", "You were removed from workspace "+ws.Name, "workspaces.html", true)
	activity(u, "removed a member from workspace "+ws.Name, "workspace", ws.ID, nil, &ws.ID, &ws.AdminID, "")
	msg(w, "Member removed")
}

func sortByName[T any](list []T, name func(T) string) {
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(name(list[i])) < strings.ToLower(name(list[j])) })
}
