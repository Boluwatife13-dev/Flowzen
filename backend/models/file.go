package models

import "time"

// File is an attachment on a task.
type File struct {
	ID         string    `bson:"id" json:"id"`
	Name       string    `bson:"name" json:"name"`
	Type       string    `bson:"type" json:"type"`
	Size       int64     `bson:"size" json:"size"`
	Path       string    `bson:"path" json:"-"`
	UploadedBy ID        `bson:"uploadedBy" json:"uploadedBy"`
	UploadedAt time.Time `bson:"uploadedAt" json:"uploadedAt"`
}
