package config

import (
	"fmt"
	"os"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	DBHost      string
	DBPort      string
	DBPath      string
	DBUser      string
	DBPass      string
	AuthPlugin  string // Legacy_Auth or Srp256
	WireCrypt   bool
	JWTSecret   []byte
	Port        string
	AutoMigrate bool
}

// Load reads configuration from environment variables.
// Safe defaults are provided for local development against the workspace dinem.fdb.
func Load() (*Config, error) {
	dbPath := getEnv("DB_PATH", `c:/Users/herna/aweh.pos/dinem.fdb`)
	if dbPath == "" {
		return nil, fmt.Errorf("DB_PATH environment variable is required")
	}

	return &Config{
		DBHost:      getEnv("DB_HOST", "localhost"),
		DBPort:      getEnv("DB_PORT", "3050"),
		DBPath:      dbPath,
		DBUser:      getEnv("DB_USER", "SYSDBA"),
		DBPass:      getEnv("DB_PASS", "profes"),
		AuthPlugin:  getEnv("AUTH_PLUGIN", "Srp256"),    // Srp256 or Legacy_Auth
		WireCrypt:   getEnvBool("WIRE_CRYPT", true),      // true or false
		JWTSecret:   []byte(getEnv("JWT_SECRET", "your-secret-key")),
		Port:        getEnv("PORT", "8081"),
		AutoMigrate: getEnvBool("AUTO_MIGRATE", false),
	}, nil
}

// FirebirdDSN returns the DSN string for nakagami/firebirdsql.
// Format: user:password@host:port/path/to/db.fdb?auth_plugin_name=X&wire_crypt=Y
// Supports both Legacy_Auth (legacy password hashes) and Srp256 (modern SRP).
func (c *Config) FirebirdDSN() string {
	wireCryptStr := "false"
	if c.WireCrypt {
		wireCryptStr = "true"
	}
	return fmt.Sprintf("%s:%s@%s:%s/%s?auth_plugin_name=%s&wire_crypt=%s",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBPath, c.AuthPlugin, wireCryptStr)
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1" || v == "yes"
	}
	return defaultVal
}
