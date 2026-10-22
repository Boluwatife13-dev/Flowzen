package models

import "time"

// Request is a user asking to join an administrator's team.
type Request struct {
	ID        ID        `bson:"_id" json:"id"`
	UserID    ID        `bson:"userId" json:"userId"`
	AdminID   ID        `bson:"adminId" json:"adminId"`
	Message   string    `bson:"message" json:"message"`
	Status    string    `bson:"status" json:"status"` // pending, approved, rejected
	Note      string    `bson:"note" json:"note"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}
