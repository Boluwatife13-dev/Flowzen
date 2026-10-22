// Package config reads settings from the .env file and environment variables.
package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvFile reads KEY=VALUE lines from .env. Real environment variables win.
func LoadEnvFile() {
	file, err := os.Open(".env")
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, found := strings.Cut(line, "=")
		if found && !strings.HasPrefix(line, "#") && os.Getenv(strings.TrimSpace(key)) == "" {
			os.Setenv(strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"`))
		}
	}
}

// Get returns a setting, or the fallback when it is not set.
func Get(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func Port() string        { return Get("PORT", "8080") }
func UploadDir() string   { return Get("UPLOAD_DIR", "uploads") }
func FrontendDir() string { return Get("FRONTEND_DIR", "../frontend") }
func JWTSecret() []byte   { return []byte(Get("JWT_SECRET", "change-this-secret")) }

// FrontendURL is the public address of the site, used for links in emails.
func FrontendURL() string {
	return strings.TrimRight(Get("FRONTEND_URL", "http://localhost:8080"), "/")
}
