// Package models describes the data saved in MongoDB.
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type ID = bson.ObjectID

func NewID() ID { return bson.NewObjectID() }

// User roles: sysadmin (whole platform), admin (leads a team), manager, member.
type User struct {
	ID           ID        `bson:"_id" json:"id"`
	Name         string    `bson:"name" json:"name"`
	Email        string    `bson:"email" json:"email"`
	Password     string    `bson:"password" json:"-"`
	Role         string    `bson:"role" json:"role"`
	Active       bool      `bson:"active" json:"active"`
	Verified     bool      `bson:"verified" json:"verified"`
	Avatar       string    `bson:"avatar" json:"avatar"`
	Title        string    `bson:"title" json:"title"`
	Phone        string    `bson:"phone" json:"phone"`
	AdminID      *ID       `bson:"adminId,omitempty" json:"adminId,omitempty"` // admin this user works under
	TokenVersion int       `bson:"tokenVersion" json:"-"`                      // +1 on logout or password change
	VerifyToken  string    `bson:"verifyToken,omitempty" json:"-"`
	ResetToken   string    `bson:"resetToken,omitempty" json:"-"`
	ResetExpires time.Time `bson:"resetExpires,omitempty" json:"-"`
	CreatedAt    time.Time `bson:"createdAt" json:"createdAt"`
}

// IsApproved: admins, or users an admin has accepted into their team.
func (u *User) IsApproved() bool {
	return u.Role == "sysadmin" || u.Role == "admin" || u.AdminID != nil
}

// IsAdmin: team administrators and the system administrator.
func (u *User) IsAdmin() bool { return u.Role == "admin" || u.Role == "sysadmin" }
