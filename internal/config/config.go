package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App      AppConfig
	Auth     AuthConfig
	CoreDB   DBConfig
	RuntimeDB DBConfig
	DerivedDB DBConfig
	DNS      DNSConfig
	Routing  RoutingConfig
	Region   RegionConfig
}

type RegionConfig struct {
	Regions     []string
	RegionZones map[string][]string
	LocalRegion string
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
	AutoSchemaUpgrade      bool
	AuthoritativeRegion    string
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
	StateDir           string
	CloudflareAPIToken string
	RequireCFTunnelHealth bool
	CFTunnelSourceName    string
}

type RoutingConfig struct {
	Enabled                 bool
	ObservationEnabled      bool
	SnapshotEnabled         bool
	PublisherEnabled        bool
	PublishIntervalSec      int
	SnapshotTTLSeconds      int
	TopN                    int
	CloudflareAccountID     string
	CloudflareAPIToken      string
	CloudflareKVNamespaceID string
}

func Load() (*Config, error) {
	loadEnvFile(getEnv("AXIS_ENV_FILE", ".env"))

	cfg := &Config{
		App: AppConfig{
			HTTPAddress:            getEnv("AXIS_HTTP_ADDRESS", ":9090"),
			NodeTimeoutSec:         getEnvInt("AXIS_NODE_TIMEOUT_SEC", 30),
			NodeMonitorIntervalSec: getEnvInt("AXIS_NODE_MONITOR_INTERVAL_SEC", 5),
			AutoSchemaUpgrade:      getEnvBool("AXIS_AUTO_SCHEMA_UPGRADE", false),
			AuthoritativeRegion:    strings.TrimSpace(strings.ToLower(getEnv("AXIS_AUTHORITATIVE_REGION", "asia"))),
		},
		Auth: AuthConfig{
			AdminUsername:   getEnv("AXIS_ADMIN_USERNAME", ""),
			AdminPassword:   getEnv("AXIS_ADMIN_PASSWORD", ""),
			NodeSharedToken: getEnv("AXIS_NODE_SHARED_TOKEN", ""),
			Realm:           getEnv("AXIS_AUTH_REALM", "Axis Admin"),
		},
		CoreDB:    loadDBConfig("AXIS_CORE_DB", "AXIS_DB", "platform_core"),
		RuntimeDB: loadDBConfig("AXIS_RUNTIME_DB", "AXIS_DB", "platform_runtime"),
		DerivedDB: loadDBConfig("AXIS_DERIVED_DB", "AXIS_DB", "platform_derived"),
		DNS: DNSConfig{
			Enabled:                getEnvBool("AXIS_DNS_ENABLED", false),
			Provider:               strings.ToLower(getEnv("AXIS_DNS_PROVIDER", "")),
			Zone:                   strings.TrimSpace(getEnv("AXIS_DNS_ZONE", "")),
			RecordPrefix:           getEnv("AXIS_DNS_RECORD_PREFIX", "dl-"),
			RecordType:             strings.ToUpper(getEnv("AXIS_DNS_RECORD_TYPE", "A")),
			TTL:                    getEnvInt("AXIS_DNS_TTL", 1),
			Proxied:                getEnvBool("AXIS_DNS_PROXIED", false),
			StateDir:               strings.TrimSpace(getEnv("AXIS_DNS_STATE_DIR", "/data/axis/dns-state")),
			CloudflareAPIToken:     getEnv("AXIS_DNS_CLOUDFLARE_API_TOKEN", ""),
			RequireCFTunnelHealth:  getEnvBool("AXIS_NODE_REQUIRE_CF_TUNNEL_HEALTH", false),
			CFTunnelSourceName:     strings.TrimSpace(getEnv("AXIS_NODE_CF_TUNNEL_SOURCE_NAME", "cloudflared")),
		},
		Routing: RoutingConfig{
			Enabled:                 getEnvBool("AXIS_ROUTING_ENABLED", false),
			ObservationEnabled:      getEnvBool("AXIS_ROUTING_OBSERVATION_ENABLED", false),
			SnapshotEnabled:         getEnvBool("AXIS_ROUTING_SNAPSHOT_ENABLED", false),
			PublisherEnabled:        getEnvBool("AXIS_ROUTING_PUBLISHER_ENABLED", false),
			PublishIntervalSec:      getEnvInt("AXIS_ROUTING_PUBLISH_INTERVAL_SEC", 60),
			SnapshotTTLSeconds:      getEnvInt("AXIS_ROUTING_SNAPSHOT_TTL_SEC", 90),
			TopN:                    getEnvInt("AXIS_ROUTING_TOPN", 3),
			CloudflareAccountID:     strings.TrimSpace(getEnv("AXIS_ROUTING_CF_ACCOUNT_ID", "")),
			CloudflareAPIToken:      getEnv("AXIS_ROUTING_CF_API_TOKEN", ""),
			CloudflareKVNamespaceID: strings.TrimSpace(getEnv("AXIS_ROUTING_CF_KV_NAMESPACE_ID", "")),
		},
		Region: loadRegionConfig(),
	}

	for _, item := range []struct {
		name string
		cfg  DBConfig
	}{
		{name: "AXIS_CORE_DB", cfg: cfg.CoreDB},
		{name: "AXIS_RUNTIME_DB", cfg: cfg.RuntimeDB},
		{name: "AXIS_DERIVED_DB", cfg: cfg.DerivedDB},
	} {
		if strings.TrimSpace(item.cfg.Host) == "" {
			return nil, fmt.Errorf("%s_HOST must be set", item.name)
		}
		if item.cfg.Port <= 0 {
			return nil, fmt.Errorf("%s_PORT must be positive", item.name)
		}
		if strings.TrimSpace(item.cfg.User) == "" {
			return nil, fmt.Errorf("%s_USER must be set", item.name)
		}
		if strings.TrimSpace(item.cfg.Database) == "" {
			return nil, fmt.Errorf("%s_NAME must be set", item.name)
		}
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
	if cfg.App.AuthoritativeRegion == "" {
		cfg.App.AuthoritativeRegion = "asia"
	}
	if strings.TrimSpace(cfg.DNS.RecordPrefix) == "" {
		cfg.DNS.RecordPrefix = "dl-"
	}
	if strings.TrimSpace(cfg.DNS.CFTunnelSourceName) == "" {
		cfg.DNS.CFTunnelSourceName = "cloudflared"
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
		if cfg.DNS.Proxied {
			return nil, fmt.Errorf("AXIS_DNS_PROXIED must be false when AXIS_DNS_ENABLED is true")
		}
		if strings.TrimSpace(cfg.DNS.CloudflareAPIToken) == "" {
			return nil, fmt.Errorf("AXIS_DNS_CLOUDFLARE_API_TOKEN must be set when AXIS_DNS_ENABLED is true")
		}
	}

	if cfg.Routing.PublishIntervalSec <= 0 {
		cfg.Routing.PublishIntervalSec = 60
	}
	if cfg.Routing.SnapshotTTLSeconds <= 0 {
		cfg.Routing.SnapshotTTLSeconds = 90
	}
	if cfg.Routing.TopN <= 0 {
		cfg.Routing.TopN = 3
	}

	if !cfg.Routing.Enabled {
		if cfg.Routing.ObservationEnabled || cfg.Routing.SnapshotEnabled || cfg.Routing.PublisherEnabled {
			return nil, fmt.Errorf("AXIS_ROUTING_ENABLED must be true when routing subfeatures are enabled")
		}
		return cfg, nil
	}

	if cfg.Routing.PublisherEnabled && !cfg.Routing.SnapshotEnabled {
		return nil, fmt.Errorf("AXIS_ROUTING_SNAPSHOT_ENABLED must be true when AXIS_ROUTING_PUBLISHER_ENABLED is true")
	}
	if cfg.Routing.PublisherEnabled {
		if cfg.Routing.CloudflareAccountID == "" {
			return nil, fmt.Errorf("AXIS_ROUTING_CF_ACCOUNT_ID must be set when AXIS_ROUTING_PUBLISHER_ENABLED is true")
		}
		if strings.TrimSpace(cfg.Routing.CloudflareAPIToken) == "" {
			return nil, fmt.Errorf("AXIS_ROUTING_CF_API_TOKEN must be set when AXIS_ROUTING_PUBLISHER_ENABLED is true")
		}
		if cfg.Routing.CloudflareKVNamespaceID == "" {
			return nil, fmt.Errorf("AXIS_ROUTING_CF_KV_NAMESPACE_ID must be set when AXIS_ROUTING_PUBLISHER_ENABLED is true")
		}
	}

	return cfg, nil
}

func loadDBConfig(primaryPrefix, legacyPrefix, defaultDB string) DBConfig {
	hostKey := primaryPrefix + "_HOST"
	portKey := primaryPrefix + "_PORT"
	userKey := primaryPrefix + "_USER"
	passwordKey := primaryPrefix + "_PASSWORD"
	nameKey := primaryPrefix + "_NAME"
	maxOpenKey := primaryPrefix + "_MAX_OPEN_CONNS"
	maxIdleKey := primaryPrefix + "_MAX_IDLE_CONNS"

	legacyHostKey := legacyPrefix + "_HOST"
	legacyPortKey := legacyPrefix + "_PORT"
	legacyUserKey := legacyPrefix + "_USER"
	legacyPasswordKey := legacyPrefix + "_PASSWORD"
	legacyNameKey := legacyPrefix + "_NAME"
	legacyMaxOpenKey := legacyPrefix + "_MAX_OPEN_CONNS"
	legacyMaxIdleKey := legacyPrefix + "_MAX_IDLE_CONNS"

	return DBConfig{
		Host:         getEnv(hostKey, getEnv(legacyHostKey, getEnv("DB_MASTER_HOST", "127.0.0.1"))),
		Port:         getEnvInt(portKey, getEnvInt(legacyPortKey, getEnvInt("DB_PORT", 4000))),
		User:         getEnv(userKey, getEnv(legacyUserKey, getEnv("DB_USER", "root"))),
		Password:     getEnv(passwordKey, getEnv(legacyPasswordKey, getEnv("DB_PASSWORD", ""))),
		Database:     getEnv(nameKey, getEnv(legacyNameKey, defaultDB)),
		MaxOpenConns: getEnvInt(maxOpenKey, getEnvInt(legacyMaxOpenKey, 10)),
		MaxIdleConns: getEnvInt(maxIdleKey, getEnvInt(legacyMaxIdleKey, 5)),
	}
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
	rawRegions := getEnvSlice("AXIS_REGIONS", ",", "asia,europe,australia,north_america,south_america")
	regions := make([]string, 0, len(rawRegions))
	regionZones := make(map[string][]string)
	for _, regionName := range rawRegions {
		normalizedRegion := strings.TrimSpace(strings.ToLower(regionName))
		if normalizedRegion == "" {
			continue
		}
		regions = append(regions, normalizedRegion)
		key := "AXIS_REGION_" + strings.ToUpper(strings.ReplaceAll(normalizedRegion, "-", "_")) + "_ZONES"
		zones := getEnvSlice(key, ",", "")
		if len(zones) > 0 {
			normalizedZones := make([]string, 0, len(zones))
			for _, zone := range zones {
				normalizedZone := strings.TrimSpace(strings.ToUpper(zone))
				if normalizedZone != "" {
					normalizedZones = append(normalizedZones, normalizedZone)
				}
			}
			if len(normalizedZones) > 0 {
				regionZones[normalizedRegion] = normalizedZones
			}
		}
	}
	return RegionConfig{
		Regions:     regions,
		RegionZones: regionZones,
		LocalRegion: strings.TrimSpace(strings.ToLower(getEnv("AXIS_LOCAL_REGION", ""))),
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
	if !c.HasRegion(region) {
		return fmt.Errorf("region %q is not configured", region)
	}
	allowedZones, configured := c.RegionZones[region]
	if !configured || len(allowedZones) == 0 {
		return fmt.Errorf("region %q has no configured zones", region)
	}
	for _, z := range allowedZones {
		if strings.TrimSpace(strings.ToUpper(z)) == zone {
			return nil
		}
	}
	return fmt.Errorf("zone %q is not allowed for region %q", zone, region)
}

func (c *RegionConfig) HasRegion(region string) bool {
	region = strings.TrimSpace(strings.ToLower(region))
	for _, configured := range c.Regions {
		if strings.TrimSpace(strings.ToLower(configured)) == region {
			return true
		}
	}
	return false
}

func (c *RegionConfig) ValidateRegion(region string) error {
	region = strings.TrimSpace(strings.ToLower(region))
	if region == "" {
		return fmt.Errorf("region is required")
	}
	if !c.HasRegion(region) {
		return fmt.Errorf("region %q is not configured", region)
	}
	return nil
}

func (c *RegionConfig) ValidateZone(zone string) error {
	zone = strings.TrimSpace(strings.ToUpper(zone))
	if zone == "" {
		return fmt.Errorf("zone is required")
	}
	for _, allowedZones := range c.RegionZones {
		for _, configuredZone := range allowedZones {
			if strings.TrimSpace(strings.ToUpper(configuredZone)) == zone {
				return nil
			}
		}
	}
	return fmt.Errorf("zone %q is not configured", zone)
}

func (c *RegionConfig) AllZones() []string {
	seen := make(map[string]struct{})
	var zones []string
	for _, allowedZones := range c.RegionZones {
		for _, zone := range allowedZones {
			normalizedZone := strings.TrimSpace(strings.ToUpper(zone))
			if normalizedZone == "" {
				continue
			}
			if _, ok := seen[normalizedZone]; ok {
				continue
			}
			seen[normalizedZone] = struct{}{}
			zones = append(zones, normalizedZone)
		}
	}
	return zones
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
