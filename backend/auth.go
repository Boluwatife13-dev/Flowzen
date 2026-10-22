package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           ID        `bson:"_id" json:"id"`
	Name         string    `bson:"name" json:"name"`
	Email        string    `bson:"email" json:"email"`
	Password     string    `bson:"password" json:"-"`
	Role         string    `bson:"role" json:"role"` // sysadmin | admin | manager | member
	Active       bool      `bson:"active" json:"active"`
	Verified     bool      `bson:"verified" json:"verified"`
	Avatar       string    `bson:"avatar" json:"avatar"`
	Title        string    `bson:"title" json:"title"`
	Phone        string    `bson:"phone" json:"phone"`
	Bio          string    `bson:"bio" json:"bio"`
	AdminID      *ID       `bson:"adminId,omitempty" json:"adminId,omitempty"`
	TokenVersion int       `bson:"tokenVersion" json:"-"`
	VerifyToken  string    `bson:"verifyToken,omitempty" json:"-"`
	ResetToken   string    `bson:"resetToken,omitempty" json:"-"`
	ResetExp     time.Time `bson:"resetExp,omitempty" json:"-"`
	CreatedAt    time.Time `bson:"createdAt" json:"createdAt"`
}

type Brief struct {
	ID     ID     `bson:"_id" json:"id"`
	Name   string `bson:"name" json:"name"`
	Email  string `bson:"email" json:"email"`
	Role   string `bson:"role" json:"role"`
	Avatar string `bson:"avatar" json:"avatar"`
}

type H func(w http.ResponseWriter, r *http.Request, u *User)

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func hashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), err
}

func newToken() (plain, hashed string) {
	b := make([]byte, 32)
	rand.Read(b)
	plain = hex.EncodeToString(b)
	return plain, sha(plain)
}

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func makeJWT(u *User) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": u.ID.Hex(), "tv": u.TokenVersion, "role": u.Role, "exp": time.Now().Add(24 * time.Hour).Unix(),
	}).SignedString([]byte(conf("JWT_SECRET")))
}

// auth loads the current user on every request, so role changes and deactivation apply instantly.
func auth(h H) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims := jwt.MapClaims{}
		tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
			if _, good := t.Method.(*jwt.SigningMethodHMAC); !good {
				return nil, errors.New("bad signing method")
			}
			return []byte(conf("JWT_SECRET")), nil
		})
		if err != nil || !tok.Valid {
			fail(w, 401, "session expired, please log in again")
			return
		}
		sub, _ := claims["sub"].(string)
		tv, _ := claims["tv"].(float64)
		id, _ := oid(sub)
		u, found := findOne[User](r.Context(), "users", M{"_id": id})
		if !found || int(tv) != u.TokenVersion {
			fail(w, 401, "session expired, please log in again")
			return
		}
		if !u.Active {
			fail(w, 403, "your account has been deactivated")
			return
		}
		h(w, r, u)
	}
}

func approved(u *User) bool { return u.Role == "sysadmin" || u.Role == "admin" || u.AdminID != nil }

// team requires an approved member of an administrator's team (or an admin).
func team(h H) http.HandlerFunc {
	return auth(func(w http.ResponseWriter, r *http.Request, u *User) {
		if !approved(u) {
			fail(w, 403, "you need an approved access request from an administrator first")
			return
		}
		h(w, r, u)
	})
}

func roles(h H, allowed ...string) http.HandlerFunc {
	return auth(func(w http.ResponseWriter, r *http.Request, u *User) {
		if !in(u.Role, allowed...) {
			fail(w, 403, "you do not have permission for this action")
			return
		}
		h(w, r, u)
	})
}

func register(w http.ResponseWriter, r *http.Request) {
	var p struct{ Name, Email, Password, AdminID, Message string }
	if !body(w, r, &p) {
		return
	}
	p.Name, p.Email = strings.TrimSpace(p.Name), strings.ToLower(strings.TrimSpace(p.Email))
	if len(p.Name) < 2 || len(p.Name) > 80 || !emailRe.MatchString(p.Email) || len(p.Password) < 8 {
		fail(w, 400, "name (2-80 chars), valid email and password (min 8 chars) are required")
		return
	}
	hash, err := hashPassword(p.Password)
	if err != nil {
		fail(w, 500, "could not secure password")
		return
	}
	plain, hashed := newToken()
	u := User{ID: bson.NewObjectID(), Name: p.Name, Email: p.Email, Password: hash, Role: "member", Active: true, VerifyToken: hashed, CreatedAt: time.Now()}
	if _, err := col("users").InsertOne(r.Context(), u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			fail(w, 409, "an account with this email already exists")
			return
		}
		fail(w, 500, "could not create account")
		return
	}
	sendVerification(&u, plain)
	if adminID, good := oid(p.AdminID); good {
		createRequest(r, &u, adminID, p.Message)
	}
	activity(&u, "registered an account", "user", u.ID, nil, nil, nil, "")
	send(w, 201, M{"success": true, "message": "Account created. Check your email to verify your account."})
}

func sendVerification(u *User, plain string) {
	link := conf("FRONTEND_URL") + "/verify.html?token=" + plain
	sendEmail(u.Email, "Verify your Flowzen account", "Hi "+u.Name+",\n\nWelcome to Flowzen. Verify your email here:\n"+link)
}

func verifyEmail(w http.ResponseWriter, r *http.Request) {
	var p struct{ Token string }
	if !body(w, r, &p) {
		return
	}
	res, err := col("users").UpdateOne(r.Context(), M{"verifyToken": sha(p.Token)}, M{"$set": M{"verified": true}, "$unset": M{"verifyToken": ""}})
	if err != nil || res.MatchedCount == 0 {
		fail(w, 400, "invalid or already used verification link")
		return
	}
	msg(w, "Email verified. You can now log in.")
}

func resendVerification(w http.ResponseWriter, r *http.Request) {
	var p struct{ Email string }
	if !body(w, r, &p) {
		return
	}
	if u, found := findOne[User](r.Context(), "users", M{"email": strings.ToLower(strings.TrimSpace(p.Email)), "verified": false}); found {
		plain, hashed := newToken()
		col("users").UpdateOne(r.Context(), M{"_id": u.ID}, M{"$set": M{"verifyToken": hashed}})
		sendVerification(u, plain)
	}
	msg(w, "If the account exists and is unverified, a new link has been sent.")
}

func login(w http.ResponseWriter, r *http.Request) {
	var p struct{ Email, Password string }
	if !body(w, r, &p) {
		return
	}
	u, found := findOne[User](r.Context(), "users", M{"email": strings.ToLower(strings.TrimSpace(p.Email))})
	if !found || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(p.Password)) != nil {
		fail(w, 401, "invalid email or password")
		return
	}
	if !u.Verified {
		fail(w, 403, "please verify your email before logging in")
		return
	}
	if !u.Active {
		fail(w, 403, "your account has been deactivated")
		return
	}
	token, err := makeJWT(u)
	if err != nil {
		fail(w, 500, "could not create session")
		return
	}
	ok(w, M{"token": token, "user": u})
}

func forgotPassword(w http.ResponseWriter, r *http.Request) {
	var p struct{ Email string }
	if !body(w, r, &p) {
		return
	}
	if u, found := findOne[User](r.Context(), "users", M{"email": strings.ToLower(strings.TrimSpace(p.Email))}); found {
		plain, hashed := newToken()
		col("users").UpdateOne(r.Context(), M{"_id": u.ID}, M{"$set": M{"resetToken": hashed, "resetExp": time.Now().Add(time.Hour)}})
		sendEmail(u.Email, "Reset your Flowzen password", "Hi "+u.Name+",\n\nReset your password here (valid for 1 hour, single use):\n"+conf("FRONTEND_URL")+"/reset.html?token="+plain)
	}
	msg(w, "If that email exists, a reset link has been sent.")
}

func resetPassword(w http.ResponseWriter, r *http.Request) {
	var p struct{ Token, Password string }
	if !body(w, r, &p) {
		return
	}
	if len(p.Password) < 8 {
		fail(w, 400, "password must be at least 8 characters")
		return
	}
	u, found := findOne[User](r.Context(), "users", M{"resetToken": sha(p.Token)})
	if p.Token == "" || !found || time.Now().After(u.ResetExp) {
		fail(w, 400, "reset link is invalid or has expired")
		return
	}
	hash, _ := hashPassword(p.Password)
	col("users").UpdateOne(r.Context(), M{"_id": u.ID}, M{"$set": M{"password": hash}, "$unset": M{"resetToken": "", "resetExp": ""}, "$inc": M{"tokenVersion": 1}})
	msg(w, "Password reset. You can now log in.")
}

func logout(w http.ResponseWriter, r *http.Request, u *User) {
	col("users").UpdateOne(r.Context(), M{"_id": u.ID}, M{"$inc": M{"tokenVersion": 1}})
	msg(w, "Logged out")
}

func getMe(w http.ResponseWriter, r *http.Request, u *User) {
	var admin *Brief
	if u.AdminID != nil {
		admin, _ = findOne[Brief](r.Context(), "users", M{"_id": *u.AdminID})
	}
	ok(w, M{"user": u, "admin": admin})
}

func updateMe(w http.ResponseWriter, r *http.Request, u *User) {
	var p struct{ Name, Title, Phone, Bio string }
	if !body(w, r, &p) {
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if len(p.Name) < 2 || len(p.Name) > 80 || len(p.Bio) > 500 || len(p.Title) > 80 || len(p.Phone) > 30 {
		fail(w, 400, "please check the profile fields")
		return
	}
	col("users").UpdateOne(r.Context(), M{"_id": u.ID}, M{"$set": M{"name": p.Name, "title": p.Title, "phone": p.Phone, "bio": p.Bio}})
	msg(w, "Profile updated")
}

func changePassword(w http.ResponseWriter, r *http.Request, u *User) {
	var p struct{ Current, New string }
	if !body(w, r, &p) {
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(p.Current)) != nil {
		fail(w, 400, "current password is incorrect")
		return
	}
	if len(p.New) < 8 {
		fail(w, 400, "new password must be at least 8 characters")
		return
	}
	hash, _ := hashPassword(p.New)
	col("users").UpdateOne(r.Context(), M{"_id": u.ID}, M{"$set": M{"password": hash}, "$inc": M{"tokenVersion": 1}})
	msg(w, "Password changed. Please log in again.")
}
