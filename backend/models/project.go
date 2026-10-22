package models

import "time"

type Project struct {
	ID          ID        `bson:"_id" json:"id"`
	WorkspaceID ID        `bson:"workspaceId" json:"workspaceId"`
	AdminID     ID        `bson:"adminId" json:"adminId"`
	Name        string    `bson:"name" json:"name"`
	Description string    `bson:"description" json:"description"`
	OwnerID     ID        `bson:"ownerId" json:"ownerId"`
	Status      string    `bson:"status" json:"status"`       // planning, active, on_hold, completed, cancelled
	StartDate   string    `bson:"startDate" json:"startDate"` // dates are "YYYY-MM-DD"
	Deadline    string    `bson:"deadline" json:"deadline"`
	Members     []Member  `bson:"members" json:"members"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
}
