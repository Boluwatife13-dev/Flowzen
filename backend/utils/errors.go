// Package utils has small helpers shared by the whole backend.
package utils

import (
	"net/http"

	"flowzen/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Handler is the shape of every API handler: it returns the data to send, or an error.
type Handler func(r *http.Request, user *models.User) (any, error)

// APIError is an error the user should see, with its HTTP status code.
type APIError struct {
	Code    int
	Message string
}

func (e APIError) Error() string { return e.Message }

// Fail creates a user-facing error, e.g. Fail(400, "name is too short").
func Fail(code int, message string) error { return APIError{code, message} }

// Msg is a simple success message response.
func Msg(text string) bson.M { return bson.M{"message": text} }

// Download is returned by handlers that send a file instead of JSON.
type Download struct{ Path, Name, Type string }
