package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"flowzen/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

var EmailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func HasID(list []models.ID, id models.ID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func Contains(list []string, value string) bool {
	for _, x := range list {
		if x == value {
			return true
		}
	}
	return false
}

// ValidDate accepts "" or a date like 2026-12-31.
func ValidDate(d string) bool {
	_, err := time.Parse("2006-01-02", d)
	return d == "" || err == nil
}

func Today() string { return time.Now().Format("2006-01-02") }

func RandomHex(bytes int) string {
	b := make([]byte, bytes)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// HashToken: email and reset tokens are stored hashed, like passwords.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// Search makes a safe, case-insensitive "contains" match.
func Search(text string) bson.M {
	return bson.M{"$regex": regexp.QuoteMeta(strings.TrimSpace(text)), "$options": "i"}
}
