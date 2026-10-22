// Package handlers contains one function per API endpoint.
package handlers

import (
	"net/http"

	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Health checks that the server and the database are working.
func Health(r *http.Request, _ *models.User) (any, error) {
	if err := database.DB.Client().Ping(r.Context(), nil); err != nil {
		return nil, utils.Fail(503, "database not reachable: "+err.Error())
	}
	return bson.M{"status": "ok", "database": "connected"}, nil
}
