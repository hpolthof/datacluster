package config

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
)

type Config struct {
	DataDir     string
	AppPassword string
	AppToken    string // derived from AppPassword
	Port        string
}

func Load() *Config {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	password := os.Getenv("APP_PASSWORD")
	if password == "" {
		log.Println("WARNING: APP_PASSWORD not set — authentication is disabled")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	return &Config{
		DataDir:     dataDir,
		AppPassword: password,
		AppToken:    deriveToken(password),
		Port:        port,
	}
}

func deriveToken(password string) string {
	if password == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write([]byte("datacluster-session-v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

// TokenFromPassword derives the Bearer token for a given APP_PASSWORD.
// Used to authenticate outbound relay connections.
func TokenFromPassword(password string) string {
	return deriveToken(password)
}
