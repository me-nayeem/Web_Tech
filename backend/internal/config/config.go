// Package config reads environment variables.
package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	SupabaseURL string // without trailing slash
	JWTSecret   string // only for projects that sign tokens with HS256
	FrontendURL string // allowed CORS origin, without trailing slash
	Port        string
}

// Load reads and checks the environment. It returns an error that names every
// missing variable so the problem is clear at startup.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		SupabaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("SUPABASE_URL")), "/"),
		JWTSecret:   strings.TrimSpace(os.Getenv("SUPABASE_JWT_SECRET")),
		FrontendURL: strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_URL")), "/"),
		Port:        strings.TrimSpace(os.Getenv("PORT")),
	}
	if c.Port == "" {
		c.Port = "8080"
	}

	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if c.FrontendURL == "" {
		missing = append(missing, "FRONTEND_URL")
	}
	if c.SupabaseURL == "" && c.JWTSecret == "" {
		missing = append(missing, "SUPABASE_URL (or SUPABASE_JWT_SECRET)")
	}
	if len(missing) > 0 {
		return nil, errors.New("missing environment variables: " + strings.Join(missing, ", "))
	}
	return c, nil
}