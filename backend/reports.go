package main

import (
	"context"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func buildReport(c context.Context, u *User, from, to, projectID string) M {
	projects, wsMap := visibleProjects(c, u)
	ids, names := []ID{}, map[ID]string{}
	for _, p := range projects {
		if projectID == "" || p.ID.Hex() == projectID {
			ids = append(ids, p.ID)
			names[p.ID] = p.Name
		}
	}
	tasks := []Task{}
	if len(ids) > 0 {
		for _, t := range findAll[Task](c, "tasks", M{"projectId": M{"$in": ids}}, nil, 0) {
			d := t.CreatedAt.Format("2006-01-02")
			if (from == "" || d >= from) && (to == "" || d <= to) {
				tasks = append(tasks, t)
			}
		}
	}
	byStatus, byPriority := M{}, M{}
	for _, s := range statuses {
		byStatus[s] = 0
	}
	for _, p := range priorities {
		byPriority[p] = 0
	}
	type agg struct{ Total, Completed, Overdue int }
	perProject, workload := map[ID]*agg{}, map[ID]*agg{}
	for _, id := range ids {
		perProject[id] = &agg{}
	}
	overdue, completed := 0, 0
	upcoming := []M{}
	week := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	for _, t := range tasks {
		byStatus[t.Status] = byStatus[t.Status].(int) + 1
		byPriority[t.Priority] = byPriority[t.Priority].(int) + 1
		pp := perProject[t.ProjectID]
		pp.Total++
		done, late := t.Status == "completed", t.overdue()
		if done {
			completed++
			pp.Completed++
		}
		if late {
			overdue++
			pp.Overdue++
		}
		if !done && t.DueDate >= today() && t.DueDate <= week {
			upcoming = append(upcoming, M{"id": t.ID, "title": t.Title, "dueDate": t.DueDate, "priority": t.Priority, "project": names[t.ProjectID]})
		}
		for _, a := range t.Assignees {
			if workload[a] == nil {
				workload[a] = &agg{}
			}
			workload[a].Total++
			if done {
				workload[a].Completed++
			}
			if late {
				workload[a].Overdue++
			}
		}
	}
	sort.Slice(upcoming, func(i, j int) bool { return upcoming[i]["dueDate"].(string) < upcoming[j]["dueDate"].(string) })
	if len(upcoming) > 10 {
		upcoming = upcoming[:10]
	}
	projList := []M{}
	for _, p := range projects {
		if a := perProject[p.ID]; a != nil {
			pct := 0
			if a.Total > 0 {
				pct = a.Completed * 100 / a.Total
			}
			projList = append(projList, M{"id": p.ID, "name": p.Name, "status": p.Status, "deadline": p.Deadline, "workspace": wsMap[p.WorkspaceID].Name,
				"total": a.Total, "completed": a.Completed, "overdue": a.Overdue, "percent": pct})
		}
	}
	people := []ID{}
	for id := range workload {
		people = append(people, id)
	}
	load := []M{}
	for _, b := range briefs(c, people) {
		a := workload[b.ID]
		load = append(load, M{"user": b, "total": a.Total, "open": a.Total - a.Completed, "completed": a.Completed, "overdue": a.Overdue})
	}
	sort.Slice(load, func(i, j int) bool { return load[i]["open"].(int) > load[j]["open"].(int) })

	actFilter := M{}
	if u.Role != "sysadmin" {
		actFilter = scopeFilter(c, u)
	}
	summary := M{"workspaces": len(wsMap), "projects": len(projects)}
	switch u.Role {
	case "sysadmin":
		count := func(f M) int64 { n, _ := col("users").CountDocuments(c, f); return n }
		summary["users"], summary["activeUsers"] = count(M{}), count(M{"active": true})
		summary["admins"], summary["managers"], summary["members"] = count(M{"role": "admin"}), count(M{"role": "manager"}), count(M{"role": "member"})
		summary["pendingRequests"], _ = col("requests").CountDocuments(c, M{"status": "pending"})
	case "admin":
		summary["teamSize"], _ = col("users").CountDocuments(c, M{"adminId": u.ID})
		summary["pendingRequests"], _ = col("requests").CountDocuments(c, M{"adminId": u.ID, "status": "pending"})
	}
	return M{"total": len(tasks), "completed": completed, "incomplete": len(tasks) - completed, "overdue": overdue,
		"byStatus": byStatus, "byPriority": byPriority, "projects": projList, "workload": load, "upcoming": upcoming, "summary": summary,
		"activity": findAll[Activity](c, "activities", actFilter, bson.D{{Key: "createdAt", Value: -1}}, 15)}
}

func reports(w http.ResponseWriter, r *http.Request, u *User) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if !validDate(from) || !validDate(to) {
		fail(w, 400, "dates must be in YYYY-MM-DD format")
		return
	}
	ok(w, buildReport(r.Context(), u, from, to, q.Get("projectId")))
}

func dashboard(w http.ResponseWriter, r *http.Request, u *User) {
	c := r.Context()
	rep := M{}
	if approved(u) {
		rep = buildReport(c, u, "", "", "")
	}
	mine := findAll[Task](c, "tasks", M{"assignees": u.ID}, nil, 0)
	stats := M{"pending": 0, "in_progress": 0, "completed": 0, "overdue": 0, "total": len(mine)}
	open := []M{}
	for _, t := range mine {
		if _, tracked := stats[t.Status]; tracked {
			stats[t.Status] = stats[t.Status].(int) + 1
		}
		if t.overdue() {
			stats["overdue"] = stats["overdue"].(int) + 1
		}
		if t.Status != "completed" {
			open = append(open, M{"id": t.ID, "title": t.Title, "status": t.Status, "priority": t.Priority, "dueDate": t.DueDate, "overdue": t.overdue()})
		}
	}
	sort.SliceStable(open, func(i, j int) bool {
		a, b := open[i]["dueDate"].(string), open[j]["dueDate"].(string)
		return a != "" && (b == "" || a < b)
	})
	if len(open) > 8 {
		open = open[:8]
	}
	ok(w, M{"report": rep, "myStats": stats, "myTasks": open, "approved": approved(u),
		"notifications": findAll[Notification](c, "notifications", M{"userId": u.ID}, bson.D{{Key: "createdAt", Value: -1}}, 5)})
}

func routes(mux *http.ServeMux) {
	h := mux.HandleFunc
	h("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Client().Ping(r.Context(), nil); err != nil {
			fail(w, 503, "database not reachable: "+err.Error())
			return
		}
		ok(w, M{"status": "ok", "database": "connected", "name": db.Name()})
	})
	// auth & profile
	h("POST /api/auth/register", register)
	h("POST /api/auth/verify", verifyEmail)
	h("POST /api/auth/resend-verification", resendVerification)
	h("POST /api/auth/login", login)
	h("POST /api/auth/forgot", forgotPassword)
	h("POST /api/auth/reset", resetPassword)
	h("POST /api/auth/logout", auth(logout))
	h("GET /api/me", auth(getMe))
	h("PUT /api/me", auth(updateMe))
	h("POST /api/me/password", auth(changePassword))
	h("POST /api/me/avatar", auth(uploadAvatar))
	// administrators, access requests, teams
	h("GET /api/admins", listAdmins)
	h("POST /api/requests", auth(submitRequest))
	h("GET /api/requests/mine", auth(myRequests))
	h("GET /api/requests/incoming", roles(incomingRequests, "admin", "sysadmin"))
	h("POST /api/requests/{id}/decide", roles(decideRequest, "admin", "sysadmin"))
	h("GET /api/team", team(listTeam))
	h("PUT /api/team/{id}", roles(updateTeamMember, "admin", "sysadmin"))
	// workspaces
	h("GET /api/workspaces", team(listWorkspaces))
	h("POST /api/workspaces", team(func(w http.ResponseWriter, r *http.Request, u *User) {
		if !in(u.Role, "sysadmin", "admin", "manager") {
			fail(w, 403, "only administrators and managers can create workspaces")
			return
		}
		createWorkspace(w, r, u)
	}))
	h("GET /api/workspaces/{id}", team(getWorkspace))
	h("PUT /api/workspaces/{id}", team(updateWorkspace))
	h("DELETE /api/workspaces/{id}", team(deleteWorkspace))
	h("POST /api/workspaces/{id}/members", team(addWSMember))
	h("PUT /api/workspaces/{id}/members/{uid}", team(updateWSMember))
	h("DELETE /api/workspaces/{id}/members/{uid}", team(removeWSMember))
	// projects
	h("GET /api/projects", team(listProjects))
	h("POST /api/projects", team(createProject))
	h("GET /api/projects/{id}", team(getProject))
	h("PUT /api/projects/{id}", team(updateProject))
	h("DELETE /api/projects/{id}", team(deleteProject))
	h("POST /api/projects/{id}/members", team(projectMember))
	h("PUT /api/projects/{id}/members/{uid}", team(projectMember))
	h("DELETE /api/projects/{id}/members/{uid}", team(projectMember))
	// tasks, subtasks, comments, files
	h("GET /api/tasks", team(listTasks))
	h("POST /api/tasks", team(createTask))
	h("GET /api/tasks/{id}", team(getTask))
	h("PUT /api/tasks/{id}", team(updateTask))
	h("DELETE /api/tasks/{id}", team(deleteTask))
	h("POST /api/tasks/{id}/subtasks", team(subtask))
	h("PUT /api/tasks/{id}/subtasks/{sid}", team(subtask))
	h("DELETE /api/tasks/{id}/subtasks/{sid}", team(subtask))
	h("GET /api/tasks/{id}/comments", team(listComments))
	h("POST /api/tasks/{id}/comments", team(addComment))
	h("PUT /api/comments/{id}", team(editComment))
	h("DELETE /api/comments/{id}", team(editComment))
	h("POST /api/tasks/{id}/attachments", team(uploadAttachment))
	h("GET /api/tasks/{id}/attachments/{aid}", team(downloadAttachment))
	h("DELETE /api/tasks/{id}/attachments/{aid}", team(deleteAttachment))
	// notifications, activity, dashboards, reports
	h("GET /api/notifications", auth(listNotifications))
	h("GET /api/notifications/count", auth(unreadCount))
	h("POST /api/notifications/{id}/read", auth(readNotification))
	h("POST /api/notifications/read-all", auth(readAllNotifications))
	h("GET /api/activity", team(listActivity))
	h("GET /api/dashboard", auth(dashboard))
	h("GET /api/reports", team(reports))
	// system administration
	h("GET /api/admin/users", roles(adminUsers, "sysadmin"))
	h("GET /api/admin/users/{id}", roles(adminUserDetail, "sysadmin"))
	h("PUT /api/admin/users/{id}", roles(adminUpdateUser, "sysadmin"))
	h("GET /api/admin/logs", roles(adminLogs, "sysadmin"))
	// public profile pictures (no directory listing)
	avatars := http.StripPrefix("/uploads/avatars/", http.FileServer(http.Dir(filepath.Join(conf("UPLOAD_DIR"), "avatars"))))
	mux.Handle("GET /uploads/avatars/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		avatars.ServeHTTP(w, r)
	}))
	// The backend also serves the frontend pages, so one server runs the whole app.
	pages := http.FileServer(http.Dir(conf("FRONTEND_DIR")))
	h("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			fail(w, 404, "route not found")
			return
		}
		pages.ServeHTTP(w, r)
	})
}