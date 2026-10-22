package models

import "time"

type Comment struct {
	ID        ID         `bson:"_id" json:"id"`
	TaskID    ID         `bson:"taskId" json:"taskId"`
	ParentID  *ID        `bson:"parentId" json:"parentId"` // set when this is a reply
	UserID    ID         `bson:"userId" json:"userId"`
	Body      string     `bson:"body" json:"body"`
	Mentions  []ID       `bson:"mentions" json:"mentions"`
	CreatedAt time.Time  `bson:"createdAt" json:"createdAt"`
	EditedAt  *time.Time `bson:"editedAt" json:"editedAt"`
}
