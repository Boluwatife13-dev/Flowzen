package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"flowzen/config"
	"flowzen/database"
)

// SendEmail sends in the background and tries twice; failures are saved for the system admin.
// It uses the first option that is set up:
//  1. BREVO_API_KEY  → Brevo's web API (works on Render, which blocks SMTP)
//  2. SMTP_HOST      → normal SMTP (fine on your own computer)
//  3. nothing        → the email is printed in the terminal (development)
func SendEmail(to, subject, body string) {
	go func() {
		send := sendWithBrevo
		if config.Get("BREVO_API_KEY", "") == "" {
			send = sendWithSMTP
		}
		if config.Get("BREVO_API_KEY", "") == "" && config.Get("SMTP_HOST", "") == "" {
			log.Printf("[EMAIL] to=%s subject=%q\n%s\n", to, subject, body)
			return
		}
		var err error
		for try := 0; try < 2; try++ {
			if err = send(to, subject, body); err == nil {
				return
			}
			time.Sleep(3 * time.Second)
		}
		database.Log("error", "email to "+to+" failed: "+err.Error())
	}()
}

// sendWithBrevo calls https://api.brevo.com/v3/smtp/email.
// EMAIL_FROM must be a sender address you verified in Brevo.
func sendWithBrevo(to, subject, body string) error {
	payload, _ := json.Marshal(map[string]any{
		"sender":      map[string]string{"name": config.Get("EMAIL_FROM_NAME", "Flowzen"), "email": config.Get("EMAIL_FROM", "")},
		"to":          []map[string]string{{"email": to}},
		"subject":     subject,
		"textContent": body + "\n\n- Flowzen",
	})
	req, _ := http.NewRequest("POST", config.Get("BREVO_API_URL", "https://api.brevo.com/v3/smtp/email"), bytes.NewReader(payload))
	req.Header.Set("api-key", config.Get("BREVO_API_KEY", ""))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var reply struct{ Message string }
		json.NewDecoder(res.Body).Decode(&reply)
		return fmt.Errorf("brevo returned %d: %s", res.StatusCode, reply.Message)
	}
	return nil
}

func sendWithSMTP(to, subject, body string) error {
	host := config.Get("SMTP_HOST", "")
	from := config.Get("EMAIL_FROM", "no-reply@flowzen.app")
	name := config.Get("EMAIL_FROM_NAME", "Flowzen")
	message := "From: " + name + " <" + from + ">\r\nTo: " + to + "\r\nSubject: " + subject +
		"\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + strings.ReplaceAll(body, "\n", "\r\n")
	auth := smtp.PlainAuth("", config.Get("SMTP_USER", ""), config.Get("SMTP_PASS", ""), host)
	return smtp.SendMail(host+":"+config.Get("SMTP_PORT", "587"), auth, from, []string{to}, []byte(message))
}