package models

import "time"

type Notification struct {
	ID        ID        `bson:"_id" json:"id"`
	UserID    ID        `bson:"userId" json:"userId"`
	Message   string    `bson:"message" json:"message"`
	Link      string    `bson:"link" json:"link"`
	Read      bool      `bson:"read" json:"read"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}
