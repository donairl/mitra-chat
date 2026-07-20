package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds runtime configuration loaded from the environment.
type Config struct {
	Port        string
	DBDriver    string // sqlite | postgres
	DBDSN       string
	JWTSecret   string
	CORSOrigins []string
	UploadDir   string
}

// Load reads .env (if present) and environment variables, applying defaults.
func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		Port:        getenv("PORT", "3000"),
		DBDriver:    getenv("DB_DRIVER", "sqlite"),
		DBDSN:       getenv("DB_DSN", "mitrachat.db"),
		JWTSecret:   getenv("JWT_SECRET", "dev-secret-change-me"),
		CORSOrigins: splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173")),
		UploadDir:   getenv("UPLOAD_DIR", "uploads"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
