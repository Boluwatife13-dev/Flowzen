package handlers

import (
	"net/http"
	"strings"
	"time"

	"flowzen/config"
	"flowzen/database"
	"flowzen/middleware"
	"flowzen/models"
	"flowzen/services"
	"flowzen/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func findByEmail(email string, extra bson.M) (*models.User, error) {
	extra["email"] = strings.ToLower(strings.TrimSpace(email))
	return utils.FindOne[models.User]("users", extra)
}

func sendVerificationEmail(u *models.User, token string) {
	services.SendEmail(u.Email, "Verify your Flowzen account",
		"Hi "+u.Name+",\n\nVerify your email here:\n"+config.FrontendURL()+"/verify.html?token="+token)
}

// Register creates a member account. The user may also pick an administrator to work under.
func Register(r *http.Request, _ *models.User) (any, error) {
	var in struct{ Name, Email, Password, AdminID, Message string }
	if err := utils.ReadJSON(r, &in); err != nil {
		return nil, err
	}
	in.Name, in.Email = strings.TrimSpace(in.Name), strings.ToLower(strings.TrimSpace(in.Email))
	if len(in.Name) < 2 || !utils.EmailPattern.MatchString(in.Email) || len(in.Password) < 8 {
		return nil, utils.Fail(400, "enter your name, a valid email and a password of at least 8 characters")
	}
	hash, _ := utils.HashPassword(in.Password)
	token := utils.RandomHex(32)
	user := models.User{ID: models.NewID(), Name: in.Name, Email: in.Email, Password: hash, Role: "member",
		Active: true, VerifyToken: utils.HashToken(token), CreatedAt: time.Now()}
	if _, err := database.Col("users").InsertOne(database.Ctx, user); mongo.IsDuplicateKeyError(err) {
		return nil, utils.Fail(409, "an account with this email already exists")
	} else if err != nil {
		return nil, err
	}
	sendVerificationEmail(&user, token)
	if adminID := utils.ToID(in.AdminID); !adminID.IsZero() {
		services.SubmitRequest(&user, adminID, in.Message)
	}
	return utils.Msg("Account created. Check your email to verify your account."), nil
}

func VerifyEmail(r *http.Request, _ *models.User) (any, error) {
	var in struct{ Token string }
	utils.ReadJSON(r, &in)
	res, err := database.Col("users").UpdateOne(database.Ctx, bson.M{"verifyToken": utils.HashToken(in.Token)},
		bson.M{"$set": bson.M{"verified": true}, "$unset": bson.M{"verifyToken": ""}})
	if err != nil || res.MatchedCount == 0 {
		return nil, utils.Fail(400, "this verification link is invalid or was already used")
	}
	return utils.Msg("Email verified. You can now log in."), nil
}

func ResendVerification(r *http.Request, _ *models.User) (any, error) {
	var in struct{ Email string }
	utils.ReadJSON(r, &in)
	if user, err := findByEmail(in.Email, bson.M{"verified": false}); err == nil {
		token := utils.RandomHex(32)
		utils.SetFields("users", user.ID, bson.M{"verifyToken": utils.HashToken(token)})
		sendVerificationEmail(user, token)
	}
	return utils.Msg("If that account exists and is not verified, a new link has been sent."), nil
}

func Login(r *http.Request, _ *models.User) (any, error) {
	var in struct{ Email, Password string }
	utils.ReadJSON(r, &in)
	user, err := findByEmail(in.Email, bson.M{})
	switch {
	case err != nil || !utils.CheckPassword(user.Password, in.Password):
		return nil, utils.Fail(401, "wrong email or password")
	case !user.Verified:
		return nil, utils.Fail(403, "please verify your email before logging in")
	case !user.Active:
		return nil, utils.Fail(403, "your account has been deactivated")
	}
	token, err := middleware.CreateToken(user)
	return bson.M{"token": token, "user": user}, err
}

func ForgotPassword(r *http.Request, _ *models.User) (any, error) {
	var in struct{ Email string }
	utils.ReadJSON(r, &in)
	if user, err := findByEmail(in.Email, bson.M{}); err == nil {
		token := utils.RandomHex(32)
		utils.SetFields("users", user.ID, bson.M{"resetToken": utils.HashToken(token), "resetExpires": time.Now().Add(time.Hour)})
		services.SendEmail(user.Email, "Reset your Flowzen password",
			"Hi "+user.Name+",\n\nReset your password here (works once, for 1 hour):\n"+config.FrontendURL()+"/reset.html?token="+token)
	}
	return utils.Msg("If that email exists, a reset link has been sent."), nil
}

func ResetPassword(r *http.Request, _ *models.User) (any, error) {
	var in struct{ Token, Password string }
	utils.ReadJSON(r, &in)
	if len(in.Password) < 8 {
		return nil, utils.Fail(400, "password must be at least 8 characters")
	}
	user, err := utils.FindOne[models.User]("users", bson.M{"resetToken": utils.HashToken(in.Token)})
	if err != nil || in.Token == "" || time.Now().After(user.ResetExpires) {
		return nil, utils.Fail(400, "this reset link is invalid or has expired")
	}
	hash, _ := utils.HashPassword(in.Password)
	// removing the token makes the link single-use; tokenVersion+1 logs out old sessions
	database.Col("users").UpdateOne(database.Ctx, bson.M{"_id": user.ID}, bson.M{"$set": bson.M{"password": hash},
		"$unset": bson.M{"resetToken": "", "resetExpires": ""}, "$inc": bson.M{"tokenVersion": 1}})
	return utils.Msg("Password changed. You can now log in."), nil
}

func Logout(r *http.Request, u *models.User) (any, error) {
	database.Col("users").UpdateOne(database.Ctx, bson.M{"_id": u.ID}, bson.M{"$inc": bson.M{"tokenVersion": 1}})
	return utils.Msg("Logged out"), nil
}
