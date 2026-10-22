package main

import (
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Comment struct {
	ID        ID         `bson:"_id" json:"id"`
	TaskID    ID         `bson:"taskId" json:"taskId"`
	AuthorID  ID         `bson:"authorId" json:"authorId"`
	Body      string     `bson:"body" json:"body"`
	ParentID  *ID        `bson:"parentId,omitempty" json:"parentId,omitempty"`
	Mentions  []ID       `bson:"mentions" json:"mentions"`
	CreatedAt time.Time  `bson:"createdAt" json:"createdAt"`
	EditedAt  *time.Time `bson:"editedAt,omitempty" json:"editedAt,omitempty"`
	Author    *Brief     `bson:"-" json:"author,omitempty"`
}

// ---------- comments ----------

func listComments(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	list := findAll[Comment](r.Context(), "comments", M{"taskId": x.t.ID}, bson.D{{Key: "createdAt", Value: 1}}, 0)
	ids := []ID{}
	for _, c := range list {
		ids = append(ids, c.AuthorID)
	}
	m := map[ID]Brief{}
	for _, b := range briefs(r.Context(), uniq(ids)) {
		m[b.ID] = b
	}
	for i := range list {
		a := m[list[i].AuthorID]
		list[i].Author = &a
	}
	ok(w, list)
}

func addComment(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	var b struct {
		Body     string
		ParentID string
		Mentions []string
	}
	if !body(w, r, &b) {
		return
	}
	b.Body = strings.TrimSpace(b.Body)
	if b.Body == "" || len(b.Body) > 5000 {
		fail(w, 400, "comment must be 1-5000 characters")
		return
	}
	c := r.Context()
	cm := Comment{ID: bson.NewObjectID(), TaskID: x.t.ID, AuthorID: u.ID, Body: b.Body, Mentions: []ID{}, CreatedAt: time.Now()}
	var parent *Comment
	if b.ParentID != "" {
		pid, good := oid(b.ParentID)
		if good {
			parent, good = findOne[Comment](c, "comments", M{"_id": pid, "taskId": x.t.ID})
		}
		if !good {
			fail(w, 400, "the comment you are replying to was not found")
			return
		}
		cm.ParentID = &parent.ID
	}
	mentions, _ := oids(b.Mentions)
	for _, id := range mentions {
		if m, found := findOne[User](c, "users", M{"_id": id, "active": true}); found && projRole(m, x.p, x.ws) != "" {
			cm.Mentions = append(cm.Mentions, id)
		}
	}
	if _, err := col("comments").InsertOne(c, cm); err != nil {
		fail(w, 500, "could not save comment")
		return
	}
	notified := []ID{u.ID}
	tell := func(id ID, kind, text string, email bool) {
		if !containsID(notified, id) {
			notified = append(notified, id)
			notify(id, kind, text, x.link(), email)
		}
	}
	for _, id := range cm.Mentions {
		tell(id, "mention", u.Name+" mentioned you on \""+x.t.Title+"\"", true)
	}
	if parent != nil {
		tell(parent.AuthorID, "reply", u.Name+" replied to your comment on \""+x.t.Title+"\"", true)
	}
	for _, id := range append(append([]ID{}, x.t.Assignees...), x.t.CreatedBy) {
		tell(id, "comment", u.Name+" commented on \""+x.t.Title+"\"", false)
	}
	action := "commented"
	if parent != nil {
		action = "replied to a comment"
	}
	activity(u, action, "task", x.t.ID, &x.p.ID, &x.ws.ID, &x.ws.AdminID, trim(cm.Body, 80))
	cm.Author = &Brief{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, Avatar: u.Avatar}
	send(w, 201, M{"success": true, "data": cm})
}

func trim(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func editComment(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	c := r.Context()
	cm, found := findOne[Comment](c, "comments", M{"_id": id})
	if !found {
		fail(w, 404, "comment not found")
		return
	}
	x, good := loadTask(w, r, u, cm.TaskID.Hex())
	if !good {
		return
	}
	if r.Method == http.MethodDelete {
		if cm.AuthorID != u.ID && x.role != "manager" {
			fail(w, 403, "you can only delete your own comments")
			return
		}
		col("comments").DeleteMany(c, M{"$or": []M{{"_id": id}, {"parentId": id}}})
		activity(u, "deleted a comment", "task", x.t.ID, &x.p.ID, &x.ws.ID, &x.ws.AdminID, "")
		msg(w, "Comment deleted")
		return
	}
	if cm.AuthorID != u.ID {
		fail(w, 403, "you can only edit your own comments")
		return
	}
	var b struct{ Body string }
	if !body(w, r, &b) {
		return
	}
	if b.Body = strings.TrimSpace(b.Body); b.Body == "" || len(b.Body) > 5000 {
		fail(w, 400, "comment must be 1-5000 characters")
		return
	}
	col("comments").UpdateOne(c, M{"_id": id}, M{"$set": M{"body": b.Body, "editedAt": time.Now()}})
	activity(u, "edited a comment", "task", x.t.ID, &x.p.ID, &x.ws.ID, &x.ws.AdminID, "")
	msg(w, "Comment updated")
}

// ---------- files ----------

var allowedExt = map[string]bool{".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true, ".txt": true,
	".csv": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".zip": true}
var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

// saveUpload validates and stores a multipart file; returns stored path, original name, size, mime type.
func saveUpload(w http.ResponseWriter, r *http.Request, field string, maxSize int64, exts map[string]bool, dir string) (string, string, int64, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSize+1024*10)
	file, hdr, err := r.FormFile(field)
	if err != nil {
		fail(w, 400, "file is missing or larger than the allowed size")
		return "", "", 0, "", false
	}
	defer file.Close()
	name := filepath.Base(hdr.Filename)
	ext := strings.ToLower(filepath.Ext(name))
	if !exts[ext] {
		fail(w, 400, "file type "+ext+" is not allowed")
		return "", "", 0, "", false
	}
	if hdr.Size > maxSize {
		fail(w, 400, "file is too large")
		return "", "", 0, "", false
	}
	head := make([]byte, 512)
	n, _ := file.Read(head)
	kind := http.DetectContentType(head[:n])
	if strings.HasPrefix(kind, "text/html") || (imageExt[ext] && !strings.HasPrefix(kind, "image/")) {
		fail(w, 400, "file content does not match its type")
		return "", "", 0, "", false
	}
	file.Seek(0, io.SeekStart)
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, randID()+randID()+ext)
	out, err := os.Create(path)
	if err != nil {
		sysLog("error", "file save failed: "+err.Error())
		fail(w, 500, "could not save file")
		return "", "", 0, "", false
	}
	size, err := io.Copy(out, file)
	out.Close()
	if err != nil {
		os.Remove(path)
		fail(w, 500, "could not save file")
		return "", "", 0, "", false
	}
	if t := mime.TypeByExtension(ext); t != "" {
		kind = t
	}
	return path, name, size, kind, true
}

func removeFile(path string) {
	if path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			sysLog("error", "file delete failed: "+err.Error())
		}
	}
}

func uploadAttachment(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	if !x.canEdit(u) && x.role != "contributor" {
		fail(w, 403, "you cannot add files to this task")
		return
	}
	path, name, size, kind, good := saveUpload(w, r, "file", 10<<20, allowedExt, filepath.Join(conf("UPLOAD_DIR"), "tasks"))
	if !good {
		return
	}
	a := Attachment{ID: randID(), Name: name, Type: kind, Size: size, Path: path, UploadedBy: u.ID, UploadedAt: time.Now()}
	col("tasks").UpdateOne(r.Context(), M{"_id": x.t.ID}, M{"$push": M{"attachments": a}})
	activity(u, "attached file "+name, "task", x.t.ID, &x.p.ID, &x.ws.ID, &x.ws.AdminID, "")
	send(w, 201, M{"success": true, "data": a})
}

func findAttachment(x taskCtx, aid string) (Attachment, bool) {
	for _, a := range x.t.Attachments {
		if a.ID == aid {
			return a, true
		}
	}
	return Attachment{}, false
}

func downloadAttachment(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	a, found := findAttachment(x, r.PathValue("aid"))
	f, err := os.Open(a.Path)
	if !found || err != nil {
		fail(w, 404, "file not found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", a.Type)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.Copy(w, f)
}

func deleteAttachment(w http.ResponseWriter, r *http.Request, u *User) {
	x, good := loadTask(w, r, u, r.PathValue("id"))
	if !good {
		return
	}
	a, found := findAttachment(x, r.PathValue("aid"))
	if !found {
		fail(w, 404, "file not found")
		return
	}
	if a.UploadedBy != u.ID && !x.canManage(u) {
		fail(w, 403, "you cannot delete this file")
		return
	}
	col("tasks").UpdateOne(r.Context(), M{"_id": x.t.ID}, M{"$pull": M{"attachments": M{"id": a.ID}}})
	removeFile(a.Path)
	activity(u, "deleted file "+a.Name, "task", x.t.ID, &x.p.ID, &x.ws.ID, &x.ws.AdminID, "")
	msg(w, "File deleted")
}

func uploadAvatar(w http.ResponseWriter, r *http.Request, u *User) {
	path, _, _, _, good := saveUpload(w, r, "avatar", 2<<20, imageExt, filepath.Join(conf("UPLOAD_DIR"), "avatars"))
	if !good {
		return
	}
	if u.Avatar != "" {
		removeFile(filepath.Join(conf("UPLOAD_DIR"), "avatars", filepath.Base(u.Avatar)))
	}
	url := "/uploads/avatars/" + filepath.Base(path)
	col("users").UpdateOne(r.Context(), M{"_id": u.ID}, M{"$set": M{"avatar": url}})
	ok(w, M{"avatar": url})
}

// ---------- activity ----------

func listActivity(w http.ResponseWriter, r *http.Request, u *User) {
	c, q := r.Context(), r.URL.Query()
	f := M{}
	if id, good := oid(q.Get("projectId")); good {
		if _, _, _, good := loadProject(w, r, u, id, false); !good {
			return
		}
		f["projectId"] = id
	} else if u.Role != "sysadmin" {
		f = scopeFilter(c, u)
	}
	page, limit := paging(r)
	ok(w, paginate(findAll[Activity](c, "activities", f, bson.D{{Key: "createdAt", Value: -1}}, 500), page, limit))
}

// scopeFilter: activities a non-sysadmin may see.
func scopeFilter(c context.Context, u *User) M {
	or := []M{{"actorId": u.ID}}
	if u.Role == "admin" {
		or = append(or, M{"adminId": u.ID})
	}
	projects, _ := visibleProjects(c, u)
	ids := []ID{}
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	if len(ids) > 0 {
		or = append(or, M{"projectId": M{"$in": ids}})
	}
	return M{"$or": or}
}
