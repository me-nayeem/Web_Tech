
package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	SupabaseURL string
	JWTSecret   string 
	FrontendURL string 
	Port        string
}


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
}.