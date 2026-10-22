package services

import (
	"time"

	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ReminderLoop runs every 15 minutes: "due soon" reminders and "overdue" alerts, once per task.
func ReminderLoop() {
	for {
		today, tomorrow := utils.Today(), time.Now().AddDate(0, 0, 1).Format("2006-01-02")
		dueSoon := bson.M{"status": bson.M{"$ne": "completed"}, "reminded": false, "dueDate": bson.M{"$gte": today, "$lte": tomorrow}}
		for _, t := range utils.FindAll[models.Task]("tasks", dueSoon) {
			for _, id := range t.Assignees {
				Notify(id, "Reminder: \""+t.Title+"\" is due on "+t.DueDate, "task.html?id="+t.ID.Hex(), true)
			}
			utils.SetFields("tasks", t.ID, bson.M{"reminded": true})
		}
		overdue := bson.M{"status": bson.M{"$ne": "completed"}, "overdueSent": false, "dueDate": bson.M{"$gt": "", "$lt": today}}
		for _, t := range utils.FindAll[models.Task]("tasks", overdue) {
			people := t.Assignees
			if !utils.HasID(people, t.CreatedBy) {
				people = append(people, t.CreatedBy)
			}
			for _, id := range people {
				Notify(id, "\""+t.Title+"\" is overdue (was due "+t.DueDate+")", "task.html?id="+t.ID.Hex(), true)
			}
			utils.SetFields("tasks", t.ID, bson.M{"overdueSent": true})
		}
		time.Sleep(15 * time.Minute)
	}
}
