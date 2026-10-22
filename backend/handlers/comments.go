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

func ListComments(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	comments := utils.FindAll[models.Comment]("comments", bson.M{"taskId": a.t.ID}, utils.SortBy("createdAt", 1))
	authors := []models.ID{}
	for _, c := range comments {
		authors = append(authors, c.UserID)
	}
	return bson.M{"items": comments, "users": utils.UsersByID(authors)}, nil
}

// AddComment saves a comment or reply and notifies mentioned people, the replied-to author,
// the creator and the assignees (each person once).
func AddComment(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	var in struct {
		Body, ParentID string
		Mentions       []string
	}
	utils.ReadJSON(r, &in)
	if in.Body = strings.TrimSpace(in.Body); in.Body == "" || len(in.Body) > 5000 {
		return nil, utils.Fail(400, "write a comment (max 5000 characters)")
	}
	c := models.Comment{ID: models.NewID(), TaskID: a.t.ID, UserID: u.ID, Body: in.Body, Mentions: []models.ID{}, CreatedAt: time.Now()}
	var parent *models.Comment
	if in.ParentID != "" {
		if parent, err = utils.FindOne[models.Comment]("comments", bson.M{"_id": utils.ToID(in.ParentID), "taskId": a.t.ID}); err != nil {
			return nil, utils.Fail(400, "the comment you are replying to was not found")
		}
		c.ParentID = &parent.ID
	}
	for _, id := range utils.ToIDs(in.Mentions) { // only people who can see the project can be mentioned
		if person, err := utils.FindUser(id); err == nil && services.ProjectRole(person, a.p, a.ws) != "" {
			c.Mentions = append(c.Mentions, id)
		}
	}
	database.Col("comments").InsertOne(database.Ctx, c)

	told := []models.ID{u.ID}
	tell := func(id models.ID, text string, email bool) {
		if !utils.HasID(told, id) {
			told = append(told, id)
			services.Notify(id, text, a.link(), email)
		}
	}
	for _, id := range c.Mentions {
		tell(id, u.Name+" mentioned you on \""+a.t.Title+"\"", true)
	}
	action := "commented"
	if parent != nil {
		action = "replied to a comment"
		tell(parent.UserID, u.Name+" replied to your comment on \""+a.t.Title+"\"", true)
	}
	for _, id := range append([]models.ID{a.t.CreatedBy}, a.t.Assignees...) {
		tell(id, u.Name+" commented on \""+a.t.Title+"\"", false)
	}
	services.AddActivity(u, action, a.scope())
	return c, nil
}

// loadComment finds a comment and checks the user can see its task.
func loadComment(r *http.Request, u *models.User) (*models.Comment, taskAccess, error) {
	c, err := utils.FindOne[models.Comment]("comments", bson.M{"_id": utils.PathID(r, "id")})
	if err != nil {
		return nil, taskAccess{}, err
	}
	a, err := loadTask(u, c.TaskID)
	return c, a, err
}

// EditComment: only the author.
func EditComment(r *http.Request, u *models.User) (any, error) {
	c, a, err := loadComment(r, u)
	if err != nil {
		return nil, err
	}
	if c.UserID != u.ID {
		return nil, utils.Fail(403, "you can only edit your own comments")
	}
	var in struct{ Body string }
	utils.ReadJSON(r, &in)
	if strings.TrimSpace(in.Body) == "" {
		return nil, utils.Fail(400, "comment cannot be empty")
	}
	utils.SetFields("comments", c.ID, bson.M{"body": strings.TrimSpace(in.Body), "editedAt": time.Now()})
	services.AddActivity(u, "edited a comment", a.scope())
	return utils.Msg("Comment saved"), nil
}

// DeleteComment: the author or a project manager. Its replies are deleted too.
func DeleteComment(r *http.Request, u *models.User) (any, error) {
	c, a, err := loadComment(r, u)
	if err != nil {
		return nil, err
	}
	if c.UserID != u.ID && a.role != "manager" {
		return nil, utils.Fail(403, "you can only delete your own comments")
	}
	database.Col("comments").DeleteMany(database.Ctx, bson.M{"$or": []bson.M{{"_id": c.ID}, {"parentId": c.ID}}})
	services.AddActivity(u, "deleted a comment", a.scope())
	return utils.Msg("Comment deleted"), nil
}
