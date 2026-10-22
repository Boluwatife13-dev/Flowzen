package handlers

import (
	"net/http"

	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ListNotifications returns my notifications (?unread=1 for unread only) and the unread count.
func ListNotifications(r *http.Request, u *models.User) (any, error) {
	filter := bson.M{"userId": u.ID}
	if r.URL.Query().Get("unread") == "1" {
		filter["read"] = false
	}
	result := utils.FindPage[models.Notification](r, "notifications", filter, bson.D{{Key: "createdAt", Value: -1}})
	result["unread"], _ = database.Col("notifications").CountDocuments(database.Ctx, bson.M{"userId": u.ID, "read": false})
	return result, nil
}

func MarkNotificationRead(r *http.Request, u *models.User) (any, error) {
	database.Col("notifications").UpdateOne(database.Ctx, bson.M{"_id": utils.PathID(r, "id"), "userId": u.ID}, bson.M{"$set": bson.M{"read": true}})
	return utils.Msg("Marked as read"), nil
}

func MarkAllRead(r *http.Request, u *models.User) (any, error) {
	database.Col("notifications").UpdateMany(database.Ctx, bson.M{"userId": u.ID}, bson.M{"$set": bson.M{"read": true}})
	return utils.Msg("All notifications marked as read"), nil
}
