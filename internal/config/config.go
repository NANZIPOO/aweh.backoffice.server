package config

import (
	"fmt"
	"os"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	DBHost    string
	DBPort    string
	DBPath    string
	DBUser    string
	DBPass    string
	JWTSecret []byte
	Port      string
}

// Load reads configuration from environment variables.
// Safe defaults are provided for local development against the workspace dinem.fdb.
func Load() (*Config, error) {
	dbPath := getEnv("DB_PATH", `c:/Users/herna/aweh.pos/dinem.fdb`)
	if dbPath == "" {
		return nil, fmt.Errorf("DB_PATH environment variable is required")
	}

	return &Config{
		DBHost:    getEnv("DB_HOST", "localhost"),
		DBPort:    getEnv("DB_PORT", "3050"), // FB3 instance (legacy dinem.fdb)
		DBPath:    dbPath,
		DBUser:    getEnv("DB_USER", "SYSDBA"),
		DBPass:    getEnv("DB_PASS", "profes"),
		JWTSecret: []byte(getEnv("JWT_SECRET", "your-secret-key")),
		Port:      getEnv("PORT", "8081"),
	}, nil
}

// FirebirdDSN returns the DSN string for nakagami/firebirdsql.
// Format: user:password@host:port/path/to/db.fdb?params
// We force Legacy_Auth + no wire encryption because the FB3 instance uses
// Legacy_UserManager (SYSDBA has a legacy password hash, not an SRP hash).
func (c *Config) FirebirdDSN() string {
	return fmt.Sprintf("%s:%s@%s:%s/%s?auth_plugin_name=Legacy_Auth&wire_crypt=false",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBPath)
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
