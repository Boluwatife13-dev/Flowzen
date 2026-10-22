package services

import (
	"time"

	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"
)

// Notify shows a notification on the website AND sends the same message to the person's email.
// The email has no link; it tells them to log in to Flowzen to see it and act on it.
func Notify(userID models.ID, message, link string, withEmail bool) {
	database.Col("notifications").InsertOne(database.Ctx, models.Notification{
		ID: models.NewID(), UserID: userID, Message: message, Link: link, CreatedAt: time.Now()})
	if user, err := utils.FindUser(userID); err == nil && user.Active {
		SendEmail(user.Email, "Flowzen: "+message,
			"Hi "+user.Name+",\n\n"+message+"\n\nLog in to Flowzen to see this notification.")
	}
}