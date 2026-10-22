package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Notification struct {
	ID        ID        `bson:"_id" json:"id"`
	UserID    ID        `bson:"userId" json:"userId"`
	Type      string    `bson:"type" json:"type"`
	Message   string    `bson:"message" json:"message"`
	Link      string    `bson:"link" json:"link"`
	Read      bool      `bson:"read" json:"read"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

type Activity struct {
	ID          ID        `bson:"_id" json:"id"`
	ActorID     ID        `bson:"actorId" json:"actorId"`
	ActorName   string    `bson:"actorName" json:"actorName"`
	Action      string    `bson:"action" json:"action"`
	Entity      string    `bson:"entity" json:"entity"`
	EntityID    ID        `bson:"entityId" json:"entityId"`
	ProjectID   *ID       `bson:"projectId,omitempty" json:"projectId,omitempty"`
	WorkspaceID *ID       `bson:"workspaceId,omitempty" json:"workspaceId,omitempty"`
	AdminID     *ID       `bson:"adminId,omitempty" json:"adminId,omitempty"`
	Details     string    `bson:"details" json:"details"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
}

// sendEmail delivers in the background with one retry; failures are logged as system events.
// With no SMTP_HOST configured, emails are printed to the console (development mode).
func sendEmail(to, subject, text string) {
	go func() {
		if conf("SMTP_HOST") == "" {
			log.Printf("[EMAIL dev] to=%s subject=%q\n%s\n", to, subject, text)
			return
		}
		from := conf("SMTP_FROM")
		addr := from
		if i := strings.Index(from, "<"); i >= 0 {
			addr = strings.Trim(from[i:], "<>")
		}
		m := "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + text + "\r\n\r\n— Flowzen"
		var auth smtp.Auth
		if conf("SMTP_USER") != "" {
			auth = smtp.PlainAuth("", conf("SMTP_USER"), conf("SMTP_PASS"), conf("SMTP_HOST"))
		}
		var err error
		for attempt := 0; attempt < 2; attempt++ {
			if err = smtp.SendMail(conf("SMTP_HOST")+":"+conf("SMTP_PORT"), auth, addr, []string{to}, []byte(m)); err == nil {
				return
			}
			time.Sleep(3 * time.Second)
		}
		sysLog("error", fmt.Sprintf("email to %s failed (%s): %v", to, subject, err))
	}()
}

// notify creates an in-app notification and, when withEmail is true, a matching email.
func notify(to ID, kind, message, link string, withEmail bool) {
	c := context.Background()
	col("notifications").InsertOne(c, Notification{ID: bson.NewObjectID(), UserID: to, Type: kind, Message: message, Link: link, CreatedAt: time.Now()})
	if withEmail {
		if u, found := findOne[User](c, "users", M{"_id": to}); found && u.Active {
			sendEmail(u.Email, "Flowzen: "+message, "Hi "+u.Name+",\n\n"+message+"\n\nOpen: "+conf("FRONTEND_URL")+"/"+link)
		}
	}
}

func activity(u *User, action, entity string, entityID ID, projectID, workspaceID, adminID *ID, details string) {
	col("activities").InsertOne(context.Background(), Activity{ID: bson.NewObjectID(), ActorID: u.ID, ActorName: u.Name, Action: action, Entity: entity,
		EntityID: entityID, ProjectID: projectID, WorkspaceID: workspaceID, AdminID: adminID, Details: details, CreatedAt: time.Now()})
}

func briefs(c context.Context, ids []ID) []Brief {
	if len(ids) == 0 {
		return []Brief{}
	}
	return findAll[Brief](c, "users", M{"_id": M{"$in": ids}}, bson.D{{Key: "name", Value: 1}}, 0)
}

func listNotifications(w http.ResponseWriter, r *http.Request, u *User) {
	f := M{"userId": u.ID}
	if r.URL.Query().Get("unread") == "1" {
		f["read"] = false
	}
	page, limit := paging(r)
	items := findAll[Notification](r.Context(), "notifications", f, bson.D{{Key: "createdAt", Value: -1}}, 500)
	res := paginate(items, page, limit)
	res["unread"], _ = col("notifications").CountDocuments(r.Context(), M{"userId": u.ID, "read": false})
	ok(w, res)
}

func unreadCount(w http.ResponseWriter, r *http.Request, u *User) {
	n, _ := col("notifications").CountDocuments(r.Context(), M{"userId": u.ID, "read": false})
	ok(w, M{"unread": n})
}

func readNotification(w http.ResponseWriter, r *http.Request, u *User) {
	id, good := pathID(w, r, "id")
	if !good {
		return
	}
	res, err := col("notifications").UpdateOne(r.Context(), M{"_id": id, "userId": u.ID}, M{"$set": M{"read": true}})
	if err != nil || res.MatchedCount == 0 {
		fail(w, 404, "notification not found")
		return
	}
	msg(w, "Marked as read")
}

func readAllNotifications(w http.ResponseWriter, r *http.Request, u *User) {
	col("notifications").UpdateMany(r.Context(), M{"userId": u.ID, "read": false}, M{"$set": M{"read": true}})
	msg(w, "All notifications marked as read")
}

// reminderLoop sends "due soon" and "overdue" notices once per task.
func reminderLoop() {
	for {
		c := context.Background()
		now, tomorrow := today(), time.Now().Add(24*time.Hour).Format("2006-01-02")
		for _, t := range findAll[Task](c, "tasks", M{"status": M{"$ne": "completed"}, "reminded": false, "dueDate": M{"$gte": now, "$lte": tomorrow}}, nil, 0) {
			for _, a := range t.Assignees {
				notify(a, "deadline", "Task \""+t.Title+"\" is due on "+t.DueDate, "task.html?id="+t.ID.Hex(), true)
			}
			col("tasks").UpdateOne(c, M{"_id": t.ID}, M{"$set": M{"reminded": true}})
		}
		for _, t := range findAll[Task](c, "tasks", M{"status": M{"$ne": "completed"}, "overdueNotified": false, "dueDate": M{"$lt": now, "$gt": ""}}, nil, 0) {
			for _, a := range uniq(append(t.Assignees, t.CreatedBy)) {
				notify(a, "overdue", "Task \""+t.Title+"\" is overdue (due "+t.DueDate+")", "task.html?id="+t.ID.Hex(), true)
			}
			col("tasks").UpdateOne(c, M{"_id": t.ID}, M{"$set": M{"overdueNotified": true}})
		}
		time.Sleep(15 * time.Minute)
	}
}
