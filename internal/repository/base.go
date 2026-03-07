package repository

import (
	"context"
	"fmt"
	"sync"

	"github.com/aweh-pos/gateway/internal/middleware"
	"github.com/jmoiron/sqlx"
	_ "github.com/nakagami/firebirdsql"
)

// TenantManager manages connection pools for multiple Firebird databases
type TenantManager struct {
	mu      sync.RWMutex
	conns   map[string]*sqlx.DB
	configs map[string]string // tenantID -> DSN
}

func NewTenantManager() *TenantManager {
	return &TenantManager{
		conns:   make(map[string]*sqlx.DB),
		configs: make(map[string]string),
	}
}

// RegisterTenantDB adds a DSN for a specific tenant
func (tm *TenantManager) RegisterTenantDB(tenantID, dsn string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.configs[tenantID] = dsn
}

// GetDB retrieves the pool for the current tenant from context
func (tm *TenantManager) GetDB(ctx context.Context) (*sqlx.DB, error) {
	tenantID, err := middleware.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}

	tm.mu.RLock()
	db, ok := tm.conns[tenantID]
	tm.mu.RUnlock()

	if ok {
		return db, nil
	}

	// Connect on first access
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double check
	if db, ok := tm.conns[tenantID]; ok {
		return db, nil
	}

	dsn, ok := tm.configs[tenantID]
	if !ok {
		return nil, fmt.Errorf("tenant %s not configured", tenantID)
	}

	db, err = sqlx.Connect("firebirdsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to firebird for tenant %s: %w", tenantID, err)
	}

	// Configure pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	tm.conns[tenantID] = db
	return db, nil
}

// PingTenant opens (or reuses) the connection pool for the tenant and pings
// the database. Returns an error if the DB is unreachable.
func (tm *TenantManager) PingTenant(tenantID string) error {
	// GetDB with a background context — no JWT needed for a direct ping.
	tm.mu.Lock()
	defer tm.mu.Unlock()

	dsn, ok := tm.configs[tenantID]
	if !ok {
		return fmt.Errorf("tenant %s not configured", tenantID)
	}

	// Reuse existing pool or create a new one.
	db, ok := tm.conns[tenantID]
	if !ok {
		var err error
		db, err = sqlx.Connect("firebirdsql", dsn)
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		tm.conns[tenantID] = db
	}

	return db.Ping()
}

// GetTenantDB retrieves the connection pool for a specific tenant (by ID, not context).
// Used for initialization tasks like running migrations where context may not be available.
func (tm *TenantManager) GetTenantDB(tenantID string) (*sqlx.DB, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	db, ok := tm.conns[tenantID]
	if !ok {
		return nil, fmt.Errorf("tenant %s not connected", tenantID)
	}

	return db, nil
}

// BaseRepository is embedded by specific repositories to access the tenant DB
type BaseRepository struct {
	TM *TenantManager
}
