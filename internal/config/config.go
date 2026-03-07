package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App  AppConfig
	Auth AuthConfig
	DB   DBConfig
}

type CLIAuthConfig struct {
	APIURL        string
	AdminUsername string
	AdminPassword string
	Profile       string
}

type AppConfig struct {
	HTTPAddress string
}

type AuthConfig struct {
	AdminUsername   string
	AdminPassword   string
	NodeSharedToken string
	Realm           string
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
	loadEnvFile(getEnv("AXIS_ENV_FILE", ".env"))

	cfg := &Config{
		App: AppConfig{
			HTTPAddress: getEnv("AXIS_HTTP_ADDRESS", ":9090"),
		},
		Auth: AuthConfig{
			AdminUsername:   getEnv("AXIS_ADMIN_USERNAME", ""),
			AdminPassword:   getEnv("AXIS_ADMIN_PASSWORD", ""),
			NodeSharedToken: getEnv("AXIS_NODE_SHARED_TOKEN", ""),
			Realm:           getEnv("AXIS_AUTH_REALM", "Axis Admin"),
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
	if strings.TrimSpace(cfg.Auth.AdminUsername) == "" {
		return nil, fmt.Errorf("AXIS_ADMIN_USERNAME must be set")
	}
	if strings.TrimSpace(cfg.Auth.AdminPassword) == "" {
		return nil, fmt.Errorf("AXIS_ADMIN_PASSWORD must be set")
	}
	if strings.TrimSpace(cfg.Auth.NodeSharedToken) == "" {
		return nil, fmt.Errorf("AXIS_NODE_SHARED_TOKEN must be set")
	}

	return cfg, nil
}

func LoadCLIAuth() (*CLIAuthConfig, error) {
	cfg := &CLIAuthConfig{
		APIURL:        getEnv("AXIS_API_URL", ""),
		AdminUsername: getEnv("AXIS_ADMIN_USERNAME", ""),
		AdminPassword: getEnv("AXIS_ADMIN_PASSWORD", ""),
		Profile:       getEnv("AXIS_PROFILE", ""),
	}

	if strings.TrimSpace(cfg.APIURL) == "" {
		return nil, fmt.Errorf("AXIS_API_URL must be set")
	}
	if strings.TrimSpace(cfg.AdminUsername) == "" {
		return nil, fmt.Errorf("AXIS_ADMIN_USERNAME must be set")
	}
	if strings.TrimSpace(cfg.AdminPassword) == "" {
		return nil, fmt.Errorf("AXIS_ADMIN_PASSWORD must be set")
	}

	return cfg, nil
}

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		_ = os.Setenv(key, value)
	}
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
