package utils

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"flowzen/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ReadJSON reads the request body into dest.
func ReadJSON(r *http.Request, dest any) error {
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dest) != nil {
		return Fail(400, "invalid request body")
	}
	return nil
}

// ToID turns text into an id. Invalid text gives the zero id, which never matches anything.
func ToID(text string) models.ID {
	id, _ := bson.ObjectIDFromHex(text)
	return id
}

// ToIDs turns a list of texts into ids, skipping invalid ones and duplicates.
func ToIDs(texts []string) []models.ID {
	ids := []models.ID{}
	for _, t := range texts {
		if id := ToID(t); !id.IsZero() && !HasID(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// PathID reads an id from the URL, e.g. {id} in /api/tasks/{id}.
func PathID(r *http.Request, name string) models.ID { return ToID(r.PathValue(name)) }

// PageParams reads ?page= and ?limit= (10 per page by default, at most 200).
func PageParams(r *http.Request) (int64, int64) {
	page, _ := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64)
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 10
	}
	return page, limit
}
