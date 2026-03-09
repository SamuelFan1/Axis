package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App    AppConfig
	Auth   AuthConfig
	DB     DBConfig
	DNS    DNSConfig
	Region RegionConfig
}

type RegionConfig struct {
	Regions    []string
	RegionZones map[string][]string
}

type CLIAuthConfig struct {
	APIURL        string
	AdminUsername string
	AdminPassword string
	Profile       string
}

type AppConfig struct {
	HTTPAddress            string
	NodeTimeoutSec         int
	NodeMonitorIntervalSec int
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

type DNSConfig struct {
	Enabled            bool
	Provider           string
	Zone               string
	RecordPrefix       string
	RecordType         string
	TTL                int
	Proxied            bool
	CloudflareAPIToken string
}

func Load() (*Config, error) {
	loadEnvFile(getEnv("AXIS_ENV_FILE", ".env"))

	cfg := &Config{
		App: AppConfig{
			HTTPAddress:            getEnv("AXIS_HTTP_ADDRESS", ":9090"),
			NodeTimeoutSec:         getEnvInt("AXIS_NODE_TIMEOUT_SEC", 30),
			NodeMonitorIntervalSec: getEnvInt("AXIS_NODE_MONITOR_INTERVAL_SEC", 5),
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
		DNS: DNSConfig{
			Enabled:            getEnvBool("AXIS_DNS_ENABLED", false),
			Provider:           strings.ToLower(getEnv("AXIS_DNS_PROVIDER", "")),
			Zone:               strings.TrimSpace(getEnv("AXIS_DNS_ZONE", "")),
			RecordPrefix:       getEnv("AXIS_DNS_RECORD_PREFIX", "dl-"),
			RecordType:         strings.ToUpper(getEnv("AXIS_DNS_RECORD_TYPE", "A")),
			TTL:                getEnvInt("AXIS_DNS_TTL", 1),
			Proxied:            getEnvBool("AXIS_DNS_PROXIED", false),
			CloudflareAPIToken: getEnv("AXIS_DNS_CLOUDFLARE_API_TOKEN", ""),
		},
		Region: loadRegionConfig(),
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
	if cfg.App.NodeTimeoutSec <= 0 {
		cfg.App.NodeTimeoutSec = 30
	}
	if cfg.App.NodeMonitorIntervalSec <= 0 {
		cfg.App.NodeMonitorIntervalSec = 5
	}
	if strings.TrimSpace(cfg.DNS.RecordPrefix) == "" {
		cfg.DNS.RecordPrefix = "dl-"
	}
	if cfg.DNS.RecordType == "" {
		cfg.DNS.RecordType = "A"
	}
	if cfg.DNS.TTL < 0 {
		cfg.DNS.TTL = 1
	}
	if cfg.DNS.Enabled {
		if cfg.DNS.Provider != "cloudflare" {
			return nil, fmt.Errorf("AXIS_DNS_PROVIDER must be cloudflare when AXIS_DNS_ENABLED is true")
		}
		if strings.TrimSpace(cfg.DNS.Zone) == "" {
			return nil, fmt.Errorf("AXIS_DNS_ZONE must be set when AXIS_DNS_ENABLED is true")
		}
		if strings.TrimSpace(cfg.DNS.RecordPrefix) == "" {
			return nil, fmt.Errorf("AXIS_DNS_RECORD_PREFIX must be set when AXIS_DNS_ENABLED is true")
		}
		if cfg.DNS.RecordType != "A" {
			return nil, fmt.Errorf("AXIS_DNS_RECORD_TYPE must be A")
		}
		if strings.TrimSpace(cfg.DNS.CloudflareAPIToken) == "" {
			return nil, fmt.Errorf("AXIS_DNS_CLOUDFLARE_API_TOKEN must be set when AXIS_DNS_ENABLED is true")
		}
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

func loadRegionConfig() RegionConfig {
	regions := getEnvSlice("AXIS_REGIONS", ",", "asia,europe,australia,north_america,south_america")
	regionZones := make(map[string][]string)
	for _, r := range regions {
		key := "AXIS_REGION_" + strings.ToUpper(strings.ReplaceAll(r, "-", "_")) + "_ZONES"
		zones := getEnvSlice(key, ",", "")
		if len(zones) > 0 {
			regionZones[r] = zones
		}
	}
	return RegionConfig{
		Regions:     regions,
		RegionZones: regionZones,
	}
}

func getEnvSlice(key, sep, defaultValue string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		value = defaultValue
	}
	if value == "" {
		return nil
	}
	parts := strings.Split(value, sep)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func (c *RegionConfig) ValidateRegionZone(region, zone string) error {
	region = strings.TrimSpace(strings.ToLower(region))
	zone = strings.TrimSpace(strings.ToUpper(zone))
	if region == "" {
		return fmt.Errorf("region is required")
	}
	if zone == "" {
		return fmt.Errorf("zone is required")
	}
	allowedZones, configured := c.RegionZones[region]
	if !configured {
		return nil
	}
	for _, z := range allowedZones {
		if strings.TrimSpace(strings.ToUpper(z)) == zone {
			return nil
		}
	}
	return fmt.Errorf("zone %q is not allowed for region %q", zone, region)
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

func getEnvBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
