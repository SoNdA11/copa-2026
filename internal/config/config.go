package config

import "os"

type Config struct {
	Port          string
	DatabaseURL   string
	SecretKey     string
	APIURL        string
	AdminPassword string
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/copa?sslmode=disable"
	}

	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		secretKey = "copa-2026-secret-key-change-in-production"
	}

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "https://worldcup26.ir"
	}

	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "admin123kol"
	}

	return &Config{
		Port:          port,
		DatabaseURL:   dbURL,
		SecretKey:     secretKey,
		APIURL:        apiURL,
		AdminPassword: adminPassword,
	}
}
