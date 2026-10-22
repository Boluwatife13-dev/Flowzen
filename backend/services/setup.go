package services

import (
	"log"
	"strings"
	"time"

	"flowzen/config"
	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CreateSystemAdmin makes the first system administrator if there is none yet.
func CreateSystemAdmin() {
	if n, _ := database.Col("users").CountDocuments(database.Ctx, bson.M{"role": "sysadmin"}); n > 0 {
		return
	}
	hash, _ := utils.HashPassword(config.Get("SYSADMIN_PASSWORD", "ChangeMe123!"))
	email := strings.ToLower(config.Get("SYSADMIN_EMAIL", "admin@flowzen.app"))
	database.Col("users").InsertOne(database.Ctx, models.User{ID: models.NewID(), Name: "System Admin", Email: email,
		Password: hash, Role: "sysadmin", Active: true, Verified: true, CreatedAt: time.Now()})
	log.Println("created system administrator:", email)
}
