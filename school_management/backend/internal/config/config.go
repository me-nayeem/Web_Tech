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
	local, err := readEnvFile(".env")
	if err != nil {
		return nil, err
	}
	c := &Config{
		DatabaseURL: strings.TrimSpace(envValue("DATABASE_URL", local)),
		SupabaseURL: strings.TrimRight(strings.TrimSpace(envValue("SUPABASE_URL", local)), "/"),
		JWTSecret:   strings.TrimSpace(envValue("SUPABASE_JWT_SECRET", local)),
		FrontendURL: strings.TrimRight(strings.TrimSpace(envValue("FRONTEND_URL", local)), "/"),
		Port:        strings.TrimSpace(envValue("PORT", local)),
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

func envValue(name string, local map[string]string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return local[name]
}
