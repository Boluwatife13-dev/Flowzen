package middleware

import (
	"encoding/json"
	"mime"
	"net/http"

	"flowzen/database"
	"flowzen/models"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func SendJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(body)
}

// Wrap checks access, runs the handler and sends {success, data} or {success, error}.
func Wrap(h utils.Handler, access string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user *models.User
		var err error
		if access != "public" {
			user, err = CheckAccess(r, access)
		}
		var data any
		if err == nil {
			data, err = h(r, user)
		}
		if err != nil {
			code := 500
			if e, ok := err.(utils.APIError); ok {
				code = e.Code
			} else {
				database.Log("error", r.Method+" "+r.URL.Path+": "+err.Error())
			}
			SendJSON(w, code, bson.M{"success": false, "error": err.Error()})
			return
		}
		if file, ok := data.(utils.Download); ok {
			w.Header().Set("Content-Type", file.Type)
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			http.ServeFile(w, r, file.Path)
			return
		}
		SendJSON(w, 200, bson.M{"success": true, "data": data})
	}
}
