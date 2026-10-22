// Package middleware checks who is calling the API and turns handler results into JSON.
package middleware

import (
	"net/http"
	"strings"
	"time"

	"flowzen/config"
	"flowzen/models"
	"flowzen/utils"

	"github.com/golang-jwt/jwt/v5"
)

// CreateToken makes a login token that lasts 24 hours.
func CreateToken(u *models.User) (string, error) {
	claims := jwt.MapClaims{"sub": u.ID.Hex(), "ver": u.TokenVersion, "exp": time.Now().Add(24 * time.Hour).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(config.JWTSecret())
}

// CheckAccess reads the login token and loads the user fresh from the database.
// Access levels: "login", "team" (approved by an admin), "admin", "sysadmin".
func CheckAccess(r *http.Request, access string) (*models.User, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), claims,
		func(t *jwt.Token) (any, error) { return config.JWTSecret(), nil }, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return nil, utils.Fail(401, "please log in again")
	}
	subject, _ := claims["sub"].(string)
	version, _ := claims["ver"].(float64)
	user, err := utils.FindUser(utils.ToID(subject))
	if err != nil || int(version) != user.TokenVersion { // tokenVersion changes on logout
		return nil, utils.Fail(401, "please log in again")
	}
	switch {
	case !user.Active:
		return nil, utils.Fail(403, "your account has been deactivated")
	case access == "team" && !user.IsApproved():
		return nil, utils.Fail(403, "an administrator has to approve your access request first")
	case access == "admin" && !user.IsAdmin(), access == "sysadmin" && user.Role != "sysadmin":
		return nil, utils.Fail(403, "you do not have permission to do this")
	}
	return user, nil
}
