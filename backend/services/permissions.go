package services

import (
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// WorkspaceRole: "admin", "owner", "manager", "member" or "" (no access).
func WorkspaceRole(u *models.User, ws *models.Workspace) string {
	if u.Role == "sysadmin" || ws.AdminID == u.ID {
		return "admin"
	}
	if ws.OwnerID == u.ID {
		return "owner"
	}
	return MemberRole(ws.Members, u.ID)
}

// CanManage: admins, owners and managers can manage a workspace and all its projects.
func CanManage(workspaceRole string) bool {
	return workspaceRole == "admin" || workspaceRole == "owner" || workspaceRole == "manager"
}

// ProjectRole: "manager", "contributor", "viewer" or "" (no access).
func ProjectRole(u *models.User, p *models.Project, ws *models.Workspace) string {
	if CanManage(WorkspaceRole(u, ws)) || p.OwnerID == u.ID {
		return "manager"
	}
	return MemberRole(p.Members, u.ID)
}

// LoadProject finds a project and checks the user can see it.
func LoadProject(u *models.User, id models.ID) (*models.Project, *models.Workspace, string, error) {
	p, err := utils.FindOne[models.Project]("projects", bson.M{"_id": id})
	if err != nil {
		return nil, nil, "", err
	}
	ws, err := utils.FindOne[models.Workspace]("workspaces", bson.M{"_id": p.WorkspaceID})
	if err != nil || ProjectRole(u, p, ws) == "" {
		return nil, nil, "", utils.Fail(404, "project not found")
	}
	return p, ws, ProjectRole(u, p, ws), nil
}

// MyWorkspacesFilter matches the workspaces a user belongs to (all of them for the system admin).
func MyWorkspacesFilter(u *models.User) bson.M {
	if u.Role == "sysadmin" {
		return bson.M{}
	}
	return bson.M{"$or": []bson.M{{"adminId": u.ID}, {"members.userId": u.ID}}}
}
