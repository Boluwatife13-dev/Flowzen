package models

import "time"

type Subtask struct {
	ID    string `bson:"id" json:"id"`
	Title string `bson:"title" json:"title"`
	Done  bool   `bson:"done" json:"done"`
}

type Task struct {
	ID            ID         `bson:"_id" json:"id"`
	ProjectID     ID         `bson:"projectId" json:"projectId"`
	WorkspaceID   ID         `bson:"workspaceId" json:"workspaceId"`
	AdminID       ID         `bson:"adminId" json:"adminId"`
	Title         string     `bson:"title" json:"title"`
	Description   string     `bson:"description" json:"description"`
	Priority      string     `bson:"priority" json:"priority"`
	PriorityLevel int        `bson:"priorityLevel" json:"-"` // for sorting: low 1 ... urgent 4
	Status        string     `bson:"status" json:"status"`
	StartDate     string     `bson:"startDate" json:"startDate"`
	DueDate       string     `bson:"dueDate" json:"dueDate"`
	Assignees     []ID       `bson:"assignees" json:"assignees"`
	Labels        []string   `bson:"labels" json:"labels"`
	Dependencies  []ID       `bson:"dependencies" json:"dependencies"`
	Subtasks      []Subtask  `bson:"subtasks" json:"subtasks"`
	Files         []File     `bson:"files" json:"files"`
	CreatedBy     ID         `bson:"createdBy" json:"createdBy"`
	CreatedAt     time.Time  `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time  `bson:"updatedAt" json:"updatedAt"`
	CompletedAt   *time.Time `bson:"completedAt" json:"completedAt"`
	Reminded      bool       `bson:"reminded" json:"-"`
	OverdueSent   bool       `bson:"overdueSent" json:"-"`
}

// IsOverdue: not completed and the due date has passed.
func (t *Task) IsOverdue() bool {
	return t.Status != "completed" && t.DueDate != "" && t.DueDate < time.Now().Format("2006-01-02")
}
