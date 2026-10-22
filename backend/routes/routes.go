// Package routes connects every URL to its handler and says who may use it.
package routes

import (
	"net/http"
	"strings"

	"flowzen/config"
	"flowzen/handlers"
	"flowzen/middleware"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Access levels: "public", "login" (any logged-in user), "team" (approved by an admin),
// "admin" (team admins and the system admin), "sysadmin".
func Setup() http.Handler {
	mux := http.NewServeMux()
	add := func(pattern, access string, h utils.Handler) { mux.HandleFunc(pattern, middleware.Wrap(h, access)) }

	add("GET /api/health", "public", handlers.Health)

	// sign up, login, passwords
	add("POST /api/auth/register", "public", handlers.Register)
	add("POST /api/auth/verify", "public", handlers.VerifyEmail)
	add("POST /api/auth/resend-verification", "public", handlers.ResendVerification)
	add("POST /api/auth/login", "public", handlers.Login)
	add("POST /api/auth/forgot", "public", handlers.ForgotPassword)
	add("POST /api/auth/reset", "public", handlers.ResetPassword)
	add("POST /api/auth/logout", "login", handlers.Logout)

	// my profile
	add("GET /api/me", "login", handlers.GetMe)
	add("PUT /api/me", "login", handlers.UpdateMe)
	add("POST /api/me/password", "login", handlers.ChangePassword)
	add("POST /api/me/avatar", "login", handlers.UploadAvatar)

	// administrators, access requests and teams
	add("GET /api/admins", "public", handlers.ListAdmins)
	add("POST /api/requests", "login", handlers.CreateRequest)
	add("GET /api/requests/mine", "login", handlers.MyRequests)
	add("GET /api/requests/incoming", "admin", handlers.IncomingRequests)
	add("POST /api/requests/{id}/decide", "admin", handlers.DecideRequest)
	add("GET /api/team", "team", handlers.ListTeam)
	add("PUT /api/team/{id}", "admin", handlers.UpdateTeamMember)

	// workspaces
	add("GET /api/workspaces", "team", handlers.ListWorkspaces)
	add("POST /api/workspaces", "team", handlers.CreateWorkspace)
	add("GET /api/workspaces/{id}", "team", handlers.GetWorkspace)
	add("PUT /api/workspaces/{id}", "team", handlers.UpdateWorkspace)
	add("DELETE /api/workspaces/{id}", "team", handlers.DeleteWorkspace)
	add("POST /api/workspaces/{id}/members", "team", handlers.SaveWorkspaceMember)
	add("DELETE /api/workspaces/{id}/members/{userId}", "team", handlers.RemoveWorkspaceMember)

	// projects
	add("GET /api/projects", "team", handlers.ListProjects)
	add("POST /api/projects", "team", handlers.CreateProject)
	add("GET /api/projects/{id}", "team", handlers.GetProject)
	add("PUT /api/projects/{id}", "team", handlers.UpdateProject)
	add("DELETE /api/projects/{id}", "team", handlers.DeleteProject)
	add("POST /api/projects/{id}/members", "team", handlers.SaveProjectMember)
	add("DELETE /api/projects/{id}/members/{userId}", "team", handlers.RemoveProjectMember)

	// tasks, subtasks, comments and files
	add("GET /api/tasks", "team", handlers.ListTasks)
	add("POST /api/tasks", "team", handlers.CreateTask)
	add("GET /api/tasks/{id}", "team", handlers.GetTask)
	add("PUT /api/tasks/{id}", "team", handlers.UpdateTask)
	add("DELETE /api/tasks/{id}", "team", handlers.DeleteTask)
	add("PUT /api/tasks/{id}/subtasks", "team", handlers.SaveSubtasks)
	add("GET /api/tasks/{id}/comments", "team", handlers.ListComments)
	add("POST /api/tasks/{id}/comments", "team", handlers.AddComment)
	add("PUT /api/comments/{id}", "team", handlers.EditComment)
	add("DELETE /api/comments/{id}", "team", handlers.DeleteComment)
	add("POST /api/tasks/{id}/files", "team", handlers.UploadFile)
	add("GET /api/tasks/{id}/files/{fileId}", "team", handlers.DownloadFile)
	add("DELETE /api/tasks/{id}/files/{fileId}", "team", handlers.DeleteFile)

	// notifications, dashboard, reports, activity
	add("GET /api/notifications", "login", handlers.ListNotifications)
	add("POST /api/notifications/{id}/read", "login", handlers.MarkNotificationRead)
	add("POST /api/notifications/read-all", "login", handlers.MarkAllRead)
	add("GET /api/dashboard", "login", handlers.Dashboard)
	add("GET /api/reports", "team", handlers.Reports)
	add("GET /api/activity", "team", handlers.ListActivity)

	// system administration
	add("GET /api/admin/users", "sysadmin", handlers.AdminListUsers)
	add("PUT /api/admin/users/{id}", "sysadmin", handlers.AdminUpdateUser)
	add("GET /api/admin/logs", "sysadmin", handlers.AdminLogs)

	// profile pictures and the website pages
	mux.Handle("GET /uploads/avatars/", http.StripPrefix("/uploads/avatars/", http.FileServer(http.Dir(config.UploadDir()+"/avatars"))))
	pages := http.FileServer(http.Dir(config.FrontendDir()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.Method != http.MethodGet {
			middleware.SendJSON(w, 404, bson.M{"success": false, "error": "route not found"})
			return
		}
		pages.ServeHTTP(w, r)
	})
	return middleware.CORS(mux)
}
