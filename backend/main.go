package main

import (
	"log"
	"net/http"
	"os"

	"flowzen/config"
	"flowzen/database"
	"flowzen/routes"
	"flowzen/services"
)

func main() {
	config.LoadEnvFile()
	if err := database.Connect(); err != nil {
		log.Fatal("mongo ping: ", err)
	}
	os.MkdirAll(config.UploadDir()+"/avatars", 0755)
	os.MkdirAll(config.UploadDir()+"/files", 0755)
	services.CreateSystemAdmin()
	go services.ReminderLoop()

	log.Println("Flowzen API running on :" + config.Port())
	log.Fatal(http.ListenAndServe(":"+config.Port(), routes.Setup()))
}
