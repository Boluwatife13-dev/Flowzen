package handlers

import (
	"net/http"
	"path/filepath"
	"strings"

	"flowzen/config"
	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// GetMe returns my account and the administrator I work under.
func GetMe(r *http.Request, u *models.User) (any, error) {
	var admin *models.User
	if u.AdminID != nil {
		admin, _ = utils.FindUser(*u.AdminID)
	}
	return bson.M{"user": u, "admin": admin}, nil
}

func UpdateMe(r *http.Request, u *models.User) (any, error) {
	var in struct{ Name, Title, Phone string }
	if err := utils.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(in.Name)) < 2 {
		return nil, utils.Fail(400, "name is too short")
	}
	utils.SetFields("users", u.ID, bson.M{"name": strings.TrimSpace(in.Name), "title": in.Title, "phone": in.Phone})
	return utils.Msg("Profile saved"), nil
}

func ChangePassword(r *http.Request, u *models.User) (any, error) {
	var in struct{ Current, New string }
	utils.ReadJSON(r, &in)
	if !utils.CheckPassword(u.Password, in.Current) {
		return nil, utils.Fail(400, "current password is wrong")
	}
	if len(in.New) < 8 {
		return nil, utils.Fail(400, "new password must be at least 8 characters")
	}
	hash, _ := utils.HashPassword(in.New)
	database.Col("users").UpdateOne(database.Ctx, bson.M{"_id": u.ID}, bson.M{"$set": bson.M{"password": hash}, "$inc": bson.M{"tokenVersion": 1}})
	return utils.Msg("Password changed. Please log in again."), nil
}

// UploadAvatar saves a profile picture (images only, max 2 MB).
func UploadAvatar(r *http.Request, u *models.User) (any, error) {
	file, err := utils.SaveUpload(r, "avatar", 2<<20, utils.ImageTypes, config.UploadDir()+"/avatars")
	if err != nil {
		return nil, err
	}
	if u.Avatar != "" {
		utils.RemoveFile(config.UploadDir() + "/avatars/" + filepath.Base(u.Avatar))
	}
	url := "/uploads/avatars/" + filepath.Base(file.Path)
	utils.SetFields("users", u.ID, bson.M{"avatar": url})
	return bson.M{"avatar": url}, nil
}
