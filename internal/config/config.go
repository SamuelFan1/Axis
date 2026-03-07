package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App AppConfig
	DB  DBConfig
}

type AppConfig struct {
	HTTPAddress string
}

type DBConfig struct {
	Host         string
	Port         int
	User         string
	Password     string
	Database     string
	MaxOpenConns int
	MaxIdleConns int
}

func Load() (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			HTTPAddress: getEnv("AXIS_HTTP_ADDRESS", ":9090"),
		},
		DB: DBConfig{
			Host:         getEnv("AXIS_DB_HOST", getEnv("DB_MASTER_HOST", "127.0.0.1")),
			Port:         getEnvInt("AXIS_DB_PORT", getEnvInt("DB_PORT", 4000)),
			User:         getEnv("AXIS_DB_USER", getEnv("DB_USER", "root")),
			Password:     getEnv("AXIS_DB_PASSWORD", getEnv("DB_PASSWORD", "")),
			Database:     getEnv("AXIS_DB_NAME", "AXIS"),
			MaxOpenConns: getEnvInt("AXIS_DB_MAX_OPEN_CONNS", 10),
			MaxIdleConns: getEnvInt("AXIS_DB_MAX_IDLE_CONNS", 5),
		},
	}

	if strings.TrimSpace(cfg.DB.Host) == "" {
		return nil, fmt.Errorf("AXIS_DB_HOST must be set")
	}
	if cfg.DB.Port <= 0 {
		return nil, fmt.Errorf("AXIS_DB_PORT must be positive")
	}
	if strings.TrimSpace(cfg.DB.User) == "" {
		return nil, fmt.Errorf("AXIS_DB_USER must be set")
	}
	if strings.TrimSpace(cfg.DB.Database) == "" {
		return nil, fmt.Errorf("AXIS_DB_NAME must be set")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
