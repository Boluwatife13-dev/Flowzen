package handlers

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func projectScope(p *models.Project) services.Scope {
	return services.Scope{ProjectID: &p.ID, WorkspaceID: &p.WorkspaceID, AdminID: &p.AdminID}
}

// ListProjects: search (q), workspaceId, status, sort (name or deadline), page.
func ListProjects(r *http.Request, u *models.User) (any, error) {
	q := r.URL.Query()
	projects, workspaces := services.VisibleProjects(u)
	filtered := []models.Project{}
	for _, p := range projects {
		if (q.Get("q") == "" || strings.Contains(strings.ToLower(p.Name), strings.ToLower(q.Get("q")))) &&
			(q.Get("status") == "" || p.Status == q.Get("status")) &&
			(q.Get("workspaceId") == "" || p.WorkspaceID.Hex() == q.Get("workspaceId")) {
			filtered = append(filtered, p)
		}
	}
	if q.Get("sort") == "name" {
		sort.Slice(filtered, func(i, j int) bool { return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name) })
	} else if q.Get("sort") == "deadline" {
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].Deadline < filtered[j].Deadline })
	}
	page, limit := utils.PageParams(r)
	items := []bson.M{}
	for i := (page - 1) * limit; i < page*limit && i < int64(len(filtered)); i++ {
		p, ws := filtered[i], workspaces[filtered[i].WorkspaceID]
		items = append(items, bson.M{"project": p, "workspace": ws.Name, "myRole": services.ProjectRole(u, &p, &ws), "progress": services.Progress(p.ID)})
	}
	total := int64(len(filtered))
	return bson.M{"items": items, "total": total, "page": page, "pages": (total + limit - 1) / limit}, nil
}

type projectInput struct{ WorkspaceID, Name, Description, Status, StartDate, Deadline, OwnerID string }

func (in *projectInput) check(ws *models.Workspace) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Status == "" {
		in.Status = "planning"
	}
	owner := utils.ToID(in.OwnerID)
	switch {
	case len(in.Name) < 2:
		return utils.Fail(400, "project name is too short")
	case !utils.Contains([]string{"planning", "active", "on_hold", "completed", "cancelled"}, in.Status):
		return utils.Fail(400, "invalid project status")
	case !utils.ValidDate(in.StartDate) || !utils.ValidDate(in.Deadline):
		return utils.Fail(400, "dates must look like 2026-12-31")
	case in.StartDate != "" && in.Deadline != "" && in.Deadline < in.StartDate:
		return utils.Fail(400, "the deadline cannot be before the start date")
	case !owner.IsZero() && services.MemberRole(ws.Members, owner) == "" && owner != ws.AdminID:
		return utils.Fail(400, "the owner must be a workspace member")
	}
	return nil
}

func CreateProject(r *http.Request, u *models.User) (any, error) {
	var in projectInput
	utils.ReadJSON(r, &in)
	ws, err := utils.FindOne[models.Workspace]("workspaces", bson.M{"_id": utils.ToID(in.WorkspaceID)})
	if err != nil || !services.CanManage(services.WorkspaceRole(u, ws)) {
		return nil, utils.Fail(403, "you cannot create projects in this workspace")
	}
	if err := in.check(ws); err != nil {
		return nil, err
	}
	owner := utils.ToID(in.OwnerID)
	if owner.IsZero() {
		owner = u.ID
	}
	members := services.SetMember([]models.Member{{UserID: u.ID, Role: "manager"}}, owner, "manager")
	p := models.Project{ID: models.NewID(), WorkspaceID: ws.ID, AdminID: ws.AdminID, Name: in.Name, Description: in.Description,
		OwnerID: owner, Status: in.Status, StartDate: in.StartDate, Deadline: in.Deadline, Members: members, CreatedAt: time.Now()}
	database.Col("projects").InsertOne(database.Ctx, p)
	if owner != u.ID {
		services.Notify(owner, u.Name+" made you owner of project "+p.Name, "project.html?id="+p.ID.Hex(), true)
	}
	services.AddActivity(u, "created project "+p.Name, projectScope(&p))
	return p, nil
}

func GetProject(r *http.Request, u *models.User) (any, error) {
	p, ws, role, err := services.LoadProject(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	people := append(append(services.MemberIDs(p.Members), services.MemberIDs(ws.Members)...), ws.AdminID, p.OwnerID)
	return bson.M{"project": p, "workspace": bson.M{"id": ws.ID, "name": ws.Name, "members": ws.Members}, "myRole": role,
		"canManage": role == "manager", "progress": services.Progress(p.ID), "users": utils.UsersByID(people),
		"activity": services.RecentActivity(bson.M{"projectId": p.ID}, 25)}, nil
}

func UpdateProject(r *http.Request, u *models.User) (any, error) {
	p, ws, role, err := services.LoadProject(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	if role != "manager" {
		return nil, utils.Fail(403, "only project managers can edit the project")
	}
	var in projectInput
	utils.ReadJSON(r, &in)
	if err := in.check(ws); err != nil {
		return nil, err
	}
	changes := bson.M{"name": in.Name, "description": in.Description, "status": in.Status, "startDate": in.StartDate, "deadline": in.Deadline}
	if owner := utils.ToID(in.OwnerID); !owner.IsZero() && owner != p.OwnerID {
		changes["ownerId"], changes["members"] = owner, services.SetMember(p.Members, owner, "manager")
		services.Notify(owner, "You are now the owner of project "+p.Name, "project.html?id="+p.ID.Hex(), true)
	}
	utils.SetFields("projects", p.ID, changes)
	if in.Status != p.Status || in.Deadline != p.Deadline { // tell members about important changes
		for _, m := range p.Members {
			if m.UserID != u.ID {
				services.Notify(m.UserID, "Project "+in.Name+" was updated (status: "+in.Status+", deadline: "+in.Deadline+")", "project.html?id="+p.ID.Hex(), false)
			}
		}
	}
	services.AddActivity(u, "updated project "+in.Name, projectScope(p))
	return utils.Msg("Project saved"), nil
}

func DeleteProject(r *http.Request, u *models.User) (any, error) {
	p, ws, role, err := services.LoadProject(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	if role != "manager" {
		return nil, utils.Fail(403, "only project managers can delete the project")
	}
	services.DeleteProjectData(p.ID)
	services.AddActivity(u, "deleted project "+p.Name, wsScope(ws))
	return utils.Msg("Project deleted"), nil
}
