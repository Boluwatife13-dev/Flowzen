package handlers

import (
	"net/http"

	"flowzen/config"
	"flowzen/database"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func findFile(t *models.Task, id string) (models.File, error) {
	for _, f := range t.Files {
		if f.ID == id {
			return f, nil
		}
	}
	return models.File{}, utils.Fail(404, "file not found")
}

// UploadFile attaches a file to a task (max 10 MB, documents, images, CSV, TXT, ZIP).
func UploadFile(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	if !a.canEdit(u) && a.role != "contributor" {
		return nil, utils.Fail(403, "you cannot add files to this task")
	}
	file, err := utils.SaveUpload(r, "file", 10<<20, utils.FileTypes, config.UploadDir()+"/files")
	if err != nil {
		return nil, err
	}
	file.UploadedBy = u.ID
	database.Col("tasks").UpdateOne(database.Ctx, bson.M{"_id": a.t.ID}, bson.M{"$push": bson.M{"files": file}})
	services.AddActivity(u, "attached file "+file.Name, a.scope())
	return file, nil
}

// DownloadFile: only people who can see the task can download its files.
func DownloadFile(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	f, err := findFile(a.t, r.PathValue("fileId"))
	if err != nil {
		return nil, err
	}
	return utils.Download{Path: f.Path, Name: f.Name, Type: f.Type}, nil
}

// DeleteFile: the uploader or a manager.
func DeleteFile(r *http.Request, u *models.User) (any, error) {
	a, err := loadTask(u, utils.PathID(r, "id"))
	if err != nil {
		return nil, err
	}
	f, err := findFile(a.t, r.PathValue("fileId"))
	if err != nil {
		return nil, err
	}
	if f.UploadedBy != u.ID && !a.canManage(u) {
		return nil, utils.Fail(403, "you cannot delete this file")
	}
	kept := []models.File{}
	for _, other := range a.t.Files {
		if other.ID != f.ID {
			kept = append(kept, other)
		}
	}
	utils.SetFields("tasks", a.t.ID, bson.M{"files": kept})
	utils.RemoveFile(f.Path)
	services.AddActivity(u, "deleted file "+f.Name, a.scope())
	return utils.Msg("File deleted"), nil
}
