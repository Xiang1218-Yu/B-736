package config

import (
	"os"
	"strconv"
)

type Config struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	JWTSecret  string
	UploadDir  string
	BackupDir  string
	BaseURL    string
	WebDir     string
}

func Load() Config {
	return Config{
		DBHost:     getEnv("DB_HOST", "db"),
		DBPort:     getEnvInt("DB_PORT", 3306),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", "root"),
		DBName:     getEnv("DB_NAME", "prompt736"),
		JWTSecret:  getEnv("JWT_SECRET", "prompt736_secret"),
		UploadDir:  getEnv("UPLOAD_DIR", "/data/uploads"),
		BackupDir:  getEnv("BACKUP_DIR", "/data/backups"),
		BaseURL:    getEnv("BASE_URL", "http://localhost:3000"),
		WebDir:     getEnv("WEB_DIR", "/app/web"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
