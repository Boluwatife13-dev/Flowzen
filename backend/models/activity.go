package models

import "time"

// Activity is one line of history: who did what, and where.
type Activity struct {
	ID          ID        `bson:"_id" json:"id"`
	UserID      ID        `bson:"userId" json:"userId"`
	UserName    string    `bson:"userName" json:"userName"`
	Action      string    `bson:"action" json:"action"`
	TaskID      *ID       `bson:"taskId,omitempty" json:"taskId,omitempty"`
	ProjectID   *ID       `bson:"projectId,omitempty" json:"projectId,omitempty"`
	WorkspaceID *ID       `bson:"workspaceId,omitempty" json:"workspaceId,omitempty"`
	AdminID     *ID       `bson:"adminId,omitempty" json:"adminId,omitempty"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
}

// LogEntry is a system event or error shown on the System Admin page.
type LogEntry struct {
	Level     string    `bson:"level" json:"level"`
	Message   string    `bson:"message" json:"message"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}
