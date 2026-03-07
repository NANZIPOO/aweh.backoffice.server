package db

import (
	"fmt"
	"log"
	"time"

	"github.com/aweh-pos/gateway/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/nakagami/firebirdsql"
)

// Open creates and validates a Firebird connection pool using the provided config.
// Pool settings per Project Constitution §4:
//
//	SetMaxOpenConns(10), SetMaxIdleConns(5), SetConnMaxLifetime(5min)
func Open(cfg *config.Config) (*sqlx.DB, error) {
	db, err := sqlx.Open("firebirdsql", cfg.FirebirdDSN())
	if err != nil {
		return nil, fmt.Errorf("db.Open: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db.Ping: %w", err)
	}

	log.Printf("DB: connected to Firebird at %s:%s", cfg.DBHost, cfg.DBPort)
	return db, nil
}
