package main

import (
	"fmt"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Subtask struct {
	ID    string `bson:"id" json:"id"`
	Title string `bson:"title" json:"title"`
	Done  bool   `bson:"done" json:"done"`
}

type Attachment struct {
	ID         string    `bson:"id" json:"id"`
	Name       string    `bson:"name" json:"name"`
	Type       string    `bson:"type" json:"type"`
	Size       int64     `bson:"size" json:"size"`
	Path       string    `bson:"path" json:"-"`
	UploadedBy ID        `bson:"uploadedBy" json:"uploadedBy"`
	UploadedAt time.Time `bson:"uploadedAt" json:"uploadedAt"`
}

type Task struct {
	ID              ID           `bson:"_id" json:"id"`
	ProjectID       ID           `bson:"projectId" json:"projectId"`
	WorkspaceID     ID           `bson:"workspaceId" json:"workspaceId"`
	AdminID         ID           `bson:"adminId" json:"adminId"`
	CreatedBy       ID           `bson:"createdBy" json:"createdBy"`
	Title           string       `bson:"title" json:"title"`
	Description     string       `bson:"description" json:"description"`
	Priority        string       `bson:"priority" json:"priority"`
	Status          string       `bson:"status" json:"status"`
	StartDate       string       `bson:"startDate" json:"startDate"`
	DueDate         string       `bson:"dueDate" json:"dueDate"`
	Assignees       []ID         `bson:"assignees" json:"assignees"`
	Labels          []string     `bson:"labels" json:"labels"`
	Subtasks        []Subtask    `bson:"subtasks" json:"subtasks"`
	Dependencies    []ID         `bson:"dependencies" json:"dependencies"`
	Attachments     []Attachment `bson:"attachments" json:"attachments"`
	CreatedAt       time.Time    `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time    `bson:"updatedAt" json:"updatedAt"`
	CompletedAt     *time.Time   `bson:"completedAt,omitempty" json:"completedAt,omitempty"`
	Reminded        bool         `bson:"reminded" json:"-"`
	OverdueNotified bool         `bson:"overdueNotified" json:"-"`
}

var priorities = []string{"low", "medium", "high", "urgent"}
var statuses = []string{"pending", "in_progress", "review", "blocked", "completed"}

func (t *Task) overdue() bool { return t.Status != "completed" && t.DueDate != "" && t.DueDate < today() }

func randID() string { b := make([]byte, 6); rand.Read(b); return hex.EncodeToString(b) }

type taskCtx struct {
	t    *Task
	p    *Project
	ws   *Workspace
	role string
}

func (x taskCtx) canEdit(u *User) bool {
	return x.role == "manager" || x.t.CreatedBy == u.ID || containsID(x.t.Assignees, u.ID)
}
func (x taskCtx) canManage(u *User) bool { return x.role == "manager" || x.t.CreatedBy == u.ID }
func (x taskCtx) link() string        { return "task.html?id=" + x.t.ID.Hex() }

func loadTask(w http.ResponseWriter, r *http.Request, u *User, idStr string) (taskCtx, bool) {
	id, good := oid(idStr)
	if !good {
		fail(w, 400, "invalid task id")
		return taskCtx{}, false
	}
	t, found := findOne[Task](r.Context(), "tasks", M{"_id": id})
	if !found {
		fail(w, 404, "task not found")
		return taskCtx{}, false
	}
	p, ws, role, good := loadProject(w, r, u, t.ProjectID, false)
	if !good {
		return taskCtx{}, false
	}
	return taskCtx{t, p, ws, role}, true
}

type taskInput struct {
	ProjectID                                               string
	Title, Description, Priority, Status, StartDate, DueDate *string
	Assignees, Labels, Dependencies                         *[]string
}

// checkAssignees ensures every assignee is active and can see the project.
func checkAssignees(r *http.Request, list []string, p *Project, ws *Workspace) ([]ID, string) {
	ids, good := oids(list)
	if !good {
		return nil, "invalid assignee id"
	}
	for _, id := range ids {
		m, found := findOne[User](r.Context(), "users", M{"_id": id, "active": true})
		if !found || projRole(m, p, ws) == "" {
			return nil, "every assignee must be a member of this project"
		}
	}
	return ids, ""
}

func checkDeps(r *http.Request, list []string, self ID, projectID ID) ([]ID, string) {
	ids, good := oids(list)
	if !good {
		return nil, "invalid dependency id"
	}
	for _, id := range ids {
		if id == self {
			return nil, "a task cannot depend on itself"
		}
		if n, _ := col("tasks").CountDocuments(r.Context(), M{"_id": id, "projectId": projectID}); n == 0 {
			return nil, "dependencies must be tasks in the same project"
		}
	}
	return ids, ""
}

func cleanLabels(list []string) []string {
	out := []string{}
	for _, l := range list {
		if l = strings.TrimSpace(strings.ToLower(l)); l != "" && len(l) <= 30 && !in(l, out...) {
			out = append(out, l)
		}
	}
	return out
}

func assignMessage(u *User, t *Task, p *Project) string {
	m := u.Name + " assigned you to \"" + t.Title + "\" in project " + p.Name
	if t.DueDate != "" {
		m += " (due " + t.DueDate + ")"
	}
	return m
}

func createTask(w http.ResponseWriter, r *http.Request, u *User) {
	var b taskInput
	if !body(w, r, &b) {
		return
	}
	pid, good := oid(b.ProjectID)
	if !good {
		fail(w, 400, "choose a project")
		return
	}
	p, ws, role, good := loadProject(w, r, u, pid, false)
	if !good {
		return
	}
	if role != "manager" && role != "contributor" {
		fail(w, 403, "you cannot create tasks in this project")
		return
	}
	now := time.Now()
	t := Task{ID: bson.NewObjectID(), ProjectID: p.ID, WorkspaceID: ws.ID, AdminID: ws.AdminID, CreatedBy: u.ID, Priority: "medium", Status: "pending",
		Assignees: []ID{}, Labels: []string{}, Subtasks: []Subtask{}, Dependencies: []ID{}, Attachments: []Attachment{}, CreatedAt: now, UpdatedAt: now}
	if e := applyFields(r, &t, &b, p, ws, true); e != "" {
		fail(w, 400, e)
		return
	}
	if len(t.Assignees) > 0 && role != "manager" {
		fail(w, 403, "only project managers can assign tasks")
		return
	}
	if t.Status == "completed" {
		t.CompletedAt = &now
	}
	if _, err := col("tasks").InsertOne(r.Context(), t); err != nil {
		fail(w, 500, "could not create task")
		return
	}
	for _, a := range t.Assignees {
		if a != u.ID {
			notify(a, "assignment", assignMessage(u, &t, p), "task.html?id="+t.ID.Hex(), true)
		}
	}
	activity(u, "created task \""+t.Title+"\"", "task", t.ID, &p.ID, &ws.ID, &ws.AdminID, "")
	send(w, 201, M{"success": true, "data": t})
}

// applyFields validates and copies provided fields onto the task.
func applyFields(r *http.Request, t *Task, b *taskInput, p *Project, ws *Workspace, creating bool) string {
	if b.Title != nil || creating {
		if b.Title == nil || len(strings.TrimSpace(*b.Title)) < 2 || len(*b.Title) > 200 {
			return "task title must be 2-200 characters"
		}
		t.Title = strings.TrimSpace(*b.Title)
	}
	if b.Description != nil {
		t.Description = *b.Description
	}
	if b.Priority != nil {
		if !in(*b.Priority, priorities...) {
			return "priority must be low, medium, high or urgent"
		}
		t.Priority = *b.Priority
	}
	if b.Status != nil {
		if !in(*b.Status, statuses...) {
			return "invalid status"
		}
		t.Status = *b.Status
	}
	if b.StartDate != nil {
		t.StartDate = *b.StartDate
	}
	if b.DueDate != nil {
		t.DueDate = *b.DueDate
	}
	if !validDate(t.StartDate) || !validDate(t.DueDate) {
		return "dates must be in YYYY-MM-DD format"
	}
	if t.StartDate != "" && t.DueDate != "" && t.DueDate < t.StartDate {
		return "due date cannot be before the start date"
	}
	if b.Labels != nil {
		t.Labels = cleanLabels(*b.Labels)
	}
	if b.Assignees != nil {
		ids, e := checkAssignees(r, *b.Assignees, p, ws)
		if e != "" {
			return e
		}
		t.Assignees = ids
	}
	if b.Dependencies != nil {
		ids, e := checkDeps(r, *b.Dependencies, t.ID, p.ID)
		if e != "" {
			return e
		}
		t.Dependencies = ids
	}
	if t.Status == "completed" && len(t.Dependencies) > 0 {
		if n, _ := col("tasks").CountDocuments(r.Context(), M{"_id": M{"$in": t.Dependencies}, "status": M{"$ne": "completed"}}); n > 0 {
			return "complete the tasks this one depends on first"
		}
	}
	return ""
}

func updateTask(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	if !x.canEdit(u) {
		fail(w, 403, "you cannot edit this task")
		return
	}
	var b taskInput
	if !body(w, r, &b) {
		return
	}
	if (b.Assignees != nil || b.Dependencies != nil) && !x.canManage(u) {
		fail(w, 403, "only project managers or the task creator can reassign tasks or change dependencies")
		return
	}
	old := *x.t
	t := x.t
	if e := applyFields(r, t, &b, x.p, x.ws, false); e != "" {
		fail(w, 400, e)
		return
	}
	now := time.Now()
	t.UpdatedAt = now
	set := M{"title": t.Title, "description": t.Description, "priority": t.Priority, "status": t.Status, "startDate": t.StartDate, "dueDate": t.DueDate,
		"assignees": t.Assignees, "labels": t.Labels, "dependencies": t.Dependencies, "updatedAt": now}
	update := M{"$set": set}
	if t.Status == "completed" && old.Status != "completed" {
		t.CompletedAt, set["completedAt"] = &now, now
	} else if t.Status != "completed" {
		t.CompletedAt, update["$unset"] = nil, M{"completedAt": ""}
	}
	if t.DueDate != old.DueDate {
		set["reminded"], set["overdueNotified"] = false, false
	}
	col("tasks").UpdateOne(r.Context(), M{"_id": t.ID}, update)

	// activity history + notifications for important changes
	ids := []*ID{&x.p.ID, &x.ws.ID, &x.ws.AdminID}
	log := func(action, details string) { activity(u, action, "task", t.ID, ids[0], ids[1], ids[2], details) }
	changes := []string{}
	if old.Status != t.Status {
		changes = append(changes, "status "+old.Status+" → "+t.Status)
		log("changed status", old.Status+" → "+t.Status)
	}
	if old.Priority != t.Priority {
		changes = append(changes, "priority "+old.Priority+" → "+t.Priority)
		log("changed priority", old.Priority+" → "+t.Priority)
	}
	if old.DueDate != t.DueDate {
		changes = append(changes, "due date "+t.DueDate)
		log("changed due date", old.DueDate+" → "+t.DueDate)
	}
	if old.Title != t.Title || old.Description != t.Description || old.StartDate != t.StartDate || strings.Join(old.Labels, ",") != strings.Join(t.Labels, ",") {
		log("updated task details", "")
		if old.Title != t.Title || old.Description != t.Description {
			changes = append(changes, "details")
		}
	}
	if fmt.Sprint(old.Dependencies) != fmt.Sprint(t.Dependencies) {
		log("updated dependencies", "")
	}
	for _, a := range t.Assignees {
		if !containsID(old.Assignees, a) {
			log("assigned a user", "")
			if a != u.ID {
				notify(a, "assignment", assignMessage(u, t, x.p), x.link(), true)
			}
		}
	}
	for _, a := range old.Assignees {
		if !containsID(t.Assignees, a) {
			log("unassigned a user", "")
			if a != u.ID {
				notify(a, "assignment", u.Name+" removed you from task \""+t.Title+"\"", x.link(), true)
			}
		}
	}
	if len(changes) > 0 {
		for _, a := range uniq(append(append([]ID{}, old.Assignees...), t.CreatedBy)) {
			if a != u.ID && containsID(append(t.Assignees, t.CreatedBy), a) {
				notify(a, "task_update", u.Name+" updated \""+t.Title+"\": "+strings.Join(changes, ", "), x.link(), old.Status != t.Status || old.DueDate != t.DueDate)
			}
		}
	}
	ok(w, t)
}

func getTask(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	c := r.Context()
	deps := findAll[M](c, "tasks", M{"_id": M{"$in": x.t.Dependencies}}, nil, 0)
	depList := []M{}
	for _, d := range deps {
		depList = append(depList, M{"id": d["_id"], "title": d["title"], "status": d["status"]})
	}
	others := []M{}
	for _, o := range findAll[M](c, "tasks", M{"projectId": x.p.ID, "_id": M{"$ne": x.t.ID}}, bson.D{{Key: "title", Value: 1}}, 500) {
		others = append(others, M{"id": o["_id"], "title": o["title"]})
	}
	creator, _ := findOne[Brief](c, "users", M{"_id": x.t.CreatedBy})
	ok(w, M{"task": x.t, "overdue": x.t.overdue(), "project": M{"id": x.p.ID, "name": x.p.Name}, "assignees": briefs(c, x.t.Assignees),
		"members": briefs(c, uniq(append(append([]ID{}, x.p.MemberIDs...), x.ws.AdminID, x.p.OwnerID))), "dependencies": depList, "projectTasks": others,
		"creator": creator, "canEdit": x.canEdit(u), "canManage": x.canManage(u), "myRole": x.role,
		"activity": findAll[Activity](c, "activities", M{"entity": "task", "entityId": x.t.ID}, bson.D{{Key: "createdAt", Value: -1}}, 100)})
}

func deleteTask(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	if !x.canManage(u) {
		fail(w, 403, "only project managers or the task creator can delete this task")
		return
	}
	c := r.Context()
	for _, a := range x.t.Attachments {
		removeFile(a.Path)
	}
	col("comments").DeleteMany(c, M{"taskId": x.t.ID})
	col("tasks").UpdateMany(c, M{"dependencies": x.t.ID}, M{"$pull": M{"dependencies": x.t.ID}})
	col("tasks").DeleteOne(c, M{"_id": x.t.ID})
	for _, a := range x.t.Assignees {
		if a != u.ID {
			notify(a, "task_update", u.Name+" deleted task \""+x.t.Title+"\"", "tasks.html", false)
		}
	}
	activity(u, "deleted task \""+x.t.Title+"\"", "project", x.p.ID, &x.p.ID, &x.ws.ID, &x.ws.AdminID, "")
	msg(w, "Task deleted")
}

var priorityRank = map[string]int{"low": 0, "medium": 1, "high": 2, "urgent": 3}

// GET /api/tasks — search, filter, sort and paginate visible tasks.
func listTasks(w http.ResponseWriter, r *http.Request, u *User) {
	c, q := r.Context(), r.URL.Query()
	projects, _ := visibleProjects(c, u)
	names, ids := map[ID]string{}, []ID{}
	for _, p := range projects {
		names[p.ID] = p.Name
		ids = append(ids, p.ID)
	}
	f := M{"projectId": M{"$in": ids}}
	if id, good := oid(q.Get("projectId")); good {
		f["projectId"] = id
		if _, visible := names[id]; !visible {
			f["projectId"] = M{"$in": []ID{}}
		}
	}
	if id, good := oid(q.Get("workspaceId")); good {
		f["workspaceId"] = id
	}
	if s := q.Get("status"); s == "open" {
		f["status"] = M{"$ne": "completed"}
	} else if s != "" {
		f["status"] = s
	}
	if p := q.Get("priority"); p != "" {
		f["priority"] = p
	}
	if a := q.Get("assignee"); a == "me" {
		f["assignees"] = u.ID
	} else if id, good := oid(a); good {
		f["assignees"] = id
	}
	if s := q.Get("q"); s != "" {
		f["$or"] = []M{{"title": search(s)}, {"description": search(s)}, {"labels": strings.ToLower(strings.TrimSpace(s))}}
	}
	if l := q.Get("label"); l != "" {
		f["labels"] = strings.ToLower(l)
	}
	list := []Task{}
	if len(ids) > 0 {
		list = findAll[Task](c, "tasks", f, nil, 0)
	}
	now, week := today(), time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	from, to := q.Get("dueFrom"), q.Get("dueTo")
	filtered := []Task{}
	for _, t := range list {
		keep := true
		switch q.Get("due") {
		case "overdue":
			keep = t.overdue()
		case "today":
			keep = t.DueDate == now
		case "week":
			keep = t.DueDate >= now && t.DueDate <= week
		case "none":
			keep = t.DueDate == ""
		}
		if (from != "" && (t.DueDate == "" || t.DueDate < from)) || (to != "" && (t.DueDate == "" || t.DueDate > to)) {
			keep = false
		}
		if keep {
			filtered = append(filtered, t)
		}
	}
	key, desc := q.Get("sort"), q.Get("order") == "desc"
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		var less bool
		switch key {
		case "dueDate": // tasks without due date go last
			less = a.DueDate != "" && (b.DueDate == "" || a.DueDate < b.DueDate)
		case "priority":
			less = priorityRank[a.Priority] < priorityRank[b.Priority]
		case "title":
			less = strings.ToLower(a.Title) < strings.ToLower(b.Title)
		case "status":
			less = a.Status < b.Status
		case "updatedAt":
			less = a.UpdatedAt.Before(b.UpdatedAt)
		default:
			less = a.CreatedAt.After(b.CreatedAt) // newest first by default
		}
		if desc {
			return !less
		}
		return less
	})
	page, limit := paging(r)
	res := paginate(filtered, page, limit)
	people := []ID{}
	for _, t := range res["items"].([]Task) {
		people = append(people, t.Assignees...)
	}
	pm := map[ID]Brief{}
	for _, b := range briefs(c, uniq(people)) {
		pm[b.ID] = b
	}
	items := []M{}
	for _, t := range res["items"].([]Task) {
		as := []Brief{}
		for _, a := range t.Assignees {
			as = append(as, pm[a])
		}
		items = append(items, M{"task": t, "project": names[t.ProjectID], "assignees": as, "overdue": t.overdue()})
	}
	res["items"] = items
	ok(w, res)
}

func subtask(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	if !x.canEdit(u) {
		fail(w, 403, "you cannot edit this task")
		return
	}
	var b struct {
		Title string
		Done  *bool
	}
	if r.Method != http.MethodDelete && !body(w, r, &b) {
		return
	}
	b.Title = strings.TrimSpace(b.Title)
	subs, sid, action := x.t.Subtasks, r.PathValue("sid"), ""
	switch r.Method {
	case http.MethodPost:
		if b.Title == "" || len(b.Title) > 200 {
			fail(w, 400, "subtask title is required")
			return
		}
		subs = append(subs, Subtask{ID: randID(), Title: b.Title})
		action = "added subtask \"" + b.Title + "\""
	default:
		idx := -1
		for i, s := range subs {
			if s.ID == sid {
				idx = i
			}
		}
		if idx < 0 {
			fail(w, 404, "subtask not found")
			return
		}
		if r.Method == http.MethodDelete {
			action = "removed subtask \"" + subs[idx].Title + "\""
			subs = append(subs[:idx], subs[idx+1:]...)
		} else {
			if b.Title != "" {
				subs[idx].Title = b.Title
			}
			if b.Done != nil {
				subs[idx].Done = *b.Done
				if *b.Done {
					action = "completed subtask \"" + subs[idx].Title + "\""
				}
			}
		}
	}
	col("tasks").UpdateOne(r.Context(), M{"_id": x.t.ID}, M{"$set": M{"subtasks": subs, "updatedAt": time.Now()}})
	if action != "" {
		activity(u, action, "task", x.t.ID, &x.p.ID, &x.ws.ID, &x.ws.AdminID, "")
	}
	ok(w, subs)
}
