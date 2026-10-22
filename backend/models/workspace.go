package models

import "time"

// Member is a person in a workspace (owner/manager/member) or project (manager/contributor/viewer).
type Member struct {
	UserID ID     `bson:"userId" json:"userId"`
	Role   string `bson:"role" json:"role"`
}

type Workspace struct {
	ID          ID        `bson:"_id" json:"id"`
	Name        string    `bson:"name" json:"name"`
	Description string    `bson:"description" json:"description"`
	OwnerID     ID        `bson:"ownerId" json:"ownerId"`
	AdminID     ID        `bson:"adminId" json:"adminId"` // the admin whose team owns this workspace
	Members     []Member  `bson:"members" json:"members"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
}
