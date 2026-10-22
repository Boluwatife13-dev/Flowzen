package handlers

import (
	"net/http"
	"strings"
	"time"

	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SaveSubtasks replaces the whole checklist: [{id, title, done}]. New items get an id.
func SaveSubtasks(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	if !a.canEdit(u) {
		return nil, utils.Fail(403, "you cannot edit this task")
	}
	var in struct{ Subtasks []models.Subtask }
	utils.ReadJSON(r, &in)
	list := []models.Subtask{}
	for _, s := range in.Subtasks {
		if s.Title = strings.TrimSpace(s.Title); s.Title != "" {
			if s.ID == "" {
				s.ID = utils.RandomHex(6)
			}
			list = append(list, s)
		}
	}
	utils.SetFields("tasks", a.t.ID, bson.M{"subtasks": list, "updatedAt": time.Now()})
	services.AddActivity(u, "updated the subtasks", a.scope())
	return list, nil
}
