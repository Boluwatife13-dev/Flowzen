package handlers

import (
	"net/http"
	"strings"
	"time"

	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// loadWorkspace finds the workspace in the URL and the user's role in it.
func loadWorkspace(r *http.Request, u *models.User) (*models.Workspace, string, error) {
	ws, err := utils.FindOne[models.Workspace]("workspaces", bson.M{"_id": utils.PathID(r, "id")})
	if err != nil || services.WorkspaceRole(u, ws) == "" {
		return nil, "", utils.Fail(404, "workspace not found")
	}
	return ws, services.WorkspaceRole(u, ws), nil
}

func wsScope(ws *models.Workspace) services.Scope {
	return services.Scope{WorkspaceID: &ws.ID, AdminID: &ws.AdminID}
}

func ListWorkspaces(r *http.Request, u *models.User) (any, error) {
	filter := services.MyWorkspacesFilter(u)
	if q := r.URL.Query().Get("q"); q != "" {
		filter["name"] = utils.Search(q)
	}
	result := utils.FindPage[models.Workspace](r, "workspaces", filter, bson.D{{Key: "createdAt", Value: -1}})
	items := []bson.M{}
	for _, ws := range result["items"].([]models.Workspace) {
		projects, _ := database.Col("projects").CountDocuments(database.Ctx, bson.M{"workspaceId": ws.ID})
		items = append(items, bson.M{"workspace": ws, "myRole": services.WorkspaceRole(u, &ws), "projects": projects})
	}
	result["items"] = items
	return result, nil
}

// CreateWorkspace: admins and managers only. A manager creates it inside their admin's team.
func CreateWorkspace(r *http.Request, u *models.User) (any, error) {
	if u.Role == "member" {
		return nil, utils.Fail(403, "only administrators and managers can create workspaces")
	}
	var in struct{ Name, Description string }
	utils.ReadJSON(r, &in)
	if len(strings.TrimSpace(in.Name)) < 2 {
		return nil, utils.Fail(400, "workspace name is too short")
	}
	adminID := u.ID
	if u.Role == "manager" {
		adminID = *u.AdminID
	}
	ws := models.Workspace{ID: models.NewID(), Name: strings.TrimSpace(in.Name), Description: in.Description, OwnerID: u.ID,
		AdminID: adminID, Members: []models.Member{{UserID: u.ID, Role: "owner"}}, CreatedAt: time.Now()}
	database.Col("workspaces").InsertOne(database.Ctx, ws)
	services.AddActivity(u, "created workspace "+ws.Name, wsScope(&ws))
	return ws, nil
}

func GetWorkspace(r *http.Request, u *models.User) (any, error) {
	ws, role, err := loadWorkspace(r, u)
	if err != nil {
		return nil, err
	}
	projects := []bson.M{}
	for _, p := range utils.FindAll[models.Project]("projects", bson.M{"workspaceId": ws.ID}, utils.SortBy("createdAt", -1)) {
		if services.ProjectRole(u, &p, ws) != "" {
			projects = append(projects, bson.M{"project": p, "progress": services.Progress(p.ID)})
		}
	}
	return bson.M{"workspace": ws, "myRole": role, "canManage": services.CanManage(role), "projects": projects,
		"users": utils.UsersByID(services.MemberIDs(ws.Members)), "activity": services.RecentActivity(bson.M{"workspaceId": ws.ID}, 20)}, nil
}

// UpdateWorkspace changes the name and description, and (admin or owner only) the owner.
func UpdateWorkspace(r *http.Request, u *models.User) (any, error) {
	ws, role, err := loadWorkspace(r, u)
	if err != nil {
		return nil, err
	}
	if !services.CanManage(role) {
		return nil, utils.Fail(403, "you cannot edit this workspace")
	}
	var in struct{ Name, Description, OwnerID string }
	utils.ReadJSON(r, &in)
	if len(strings.TrimSpace(in.Name)) < 2 {
		return nil, utils.Fail(400, "workspace name is too short")
	}
	changes := bson.M{"name": strings.TrimSpace(in.Name), "description": in.Description}
	if newOwner := utils.ToID(in.OwnerID); !newOwner.IsZero() && newOwner != ws.OwnerID {
		if role != "admin" && role != "owner" {
			return nil, utils.Fail(403, "only the owner or administrator can change the owner")
		}
		if services.MemberRole(ws.Members, newOwner) == "" {
			return nil, utils.Fail(400, "the new owner must be a workspace member")
		}
		members := services.SetMember(ws.Members, newOwner, "owner")
		if services.MemberRole(members, ws.OwnerID) != "" {
			members = services.SetMember(members, ws.OwnerID, "manager") // the old owner stays as a manager
		}
		changes["ownerId"], changes["members"] = newOwner, members
		services.Notify(newOwner, "You are now the owner of workspace "+ws.Name, "workspace.html?id="+ws.ID.Hex(), true)
	}
	utils.SetFields("workspaces", ws.ID, changes)
	services.AddActivity(u, "updated workspace "+ws.Name, wsScope(ws))
	return utils.Msg("Workspace saved"), nil
}

// DeleteWorkspace removes the workspace with all its projects, tasks, comments and files.
func DeleteWorkspace(r *http.Request, u *models.User) (any, error) {
	ws, role, err := loadWorkspace(r, u)
	if err != nil {
		return nil, err
	}
	if role != "admin" && role != "owner" {
		return nil, utils.Fail(403, "only the owner or administrator can delete a workspace")
	}
	for _, p := range utils.FindAll[models.Project]("projects", bson.M{"workspaceId": ws.ID}) {
		services.DeleteProjectData(p.ID)
	}
	database.Col("workspaces").DeleteOne(database.Ctx, bson.M{"_id": ws.ID})
	services.AddActivity(u, "deleted workspace "+ws.Name, services.Scope{AdminID: &ws.AdminID})
	return utils.Msg("Workspace deleted"), nil
}
