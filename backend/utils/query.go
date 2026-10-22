package utils

import (
	"net/http"
	"strings"

	"flowzen/database"
	"flowzen/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// FindOne returns one document, or a 404 error.
func FindOne[T any](collection string, filter bson.M) (*T, error) {
	var item T
	if err := database.Col(collection).FindOne(database.Ctx, filter).Decode(&item); err != nil {
		return nil, Fail(404, strings.TrimSuffix(collection, "s")+" not found")
	}
	return &item, nil
}

// FindAll returns every matching document (an empty list if none).
func FindAll[T any](collection string, filter bson.M, opts ...options.Lister[options.FindOptions]) []T {
	items := []T{}
	cursor, err := database.Col(collection).Find(database.Ctx, filter, opts...)
	if err == nil {
		err = cursor.All(database.Ctx, &items)
	}
	if err != nil {
		database.Log("error", "database read on "+collection+": "+err.Error())
	}
	return items
}

// FindPage returns one page of results with the total count.
func FindPage[T any](r *http.Request, collection string, filter bson.M, sort bson.D) bson.M {
	page, limit := PageParams(r)
	total, _ := database.Col(collection).CountDocuments(database.Ctx, filter)
	items := FindAll[T](collection, filter, options.Find().SetSort(sort).SetSkip((page-1)*limit).SetLimit(limit))
	return bson.M{"items": items, "total": total, "page": page, "pages": (total + limit - 1) / limit}
}

func FindUser(id models.ID) (*models.User, error) {
	return FindOne[models.User]("users", bson.M{"_id": id})
}

// SetFields updates some fields of one document.
func SetFields(collection string, id models.ID, fields bson.M) {
	database.Col(collection).UpdateOne(database.Ctx, bson.M{"_id": id}, bson.M{"$set": fields})
}

func SortBy(field string, direction int) *options.FindOptionsBuilder {
	return options.Find().SetSort(bson.D{{Key: field, Value: direction}})
}

// UsersByID returns public info (name, picture) for a list of users.
func UsersByID(ids []models.ID) []bson.M {
	result := []bson.M{}
	if len(ids) == 0 {
		return result
	}
	for _, u := range FindAll[models.User]("users", bson.M{"_id": bson.M{"$in": ids}}, SortBy("name", 1)) {
		result = append(result, bson.M{"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role, "avatar": u.Avatar})
	}
	return result
}
