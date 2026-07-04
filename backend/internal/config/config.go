package config

import (
	"os"
	"time"
)

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string

	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	EncryptionKey string

	SourceDir string
	RepoSlug  string
}

func Load() Config {
	return Config{
		HTTPAddr:        getEnv("PANEL_HTTP_ADDR", ":8080"),
		DatabaseURL:     getEnv("PANEL_DATABASE_URL", "postgres://panel:panel@localhost:5432/panel?sslmode=disable"),
		RedisAddr:       getEnv("PANEL_REDIS_ADDR", "localhost:6379"),
		RedisPassword:   getEnv("PANEL_REDIS_PASSWORD", ""),
		JWTSecret:       getEnv("PANEL_JWT_SECRET", "change-me-in-production"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		EncryptionKey:   getEnv("PANEL_ENCRYPTION_KEY", ""),
		SourceDir:       getEnv("PANEL_SOURCE_DIR", ""),
		RepoSlug:        getEnv("PANEL_UPDATE_REPO", "superbodik/sbPanel"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
