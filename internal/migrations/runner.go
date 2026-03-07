package migrations

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RunMigrations executes all pending migrations from the migrations folder.
// It tracks applied migrations in a dedicated table to ensure idempotency.
// If migrate is false or AUTO_MIGRATE env is not set, this is a no-op.
func RunMigrations(db *sql.DB, shouldMigrate bool) error {
	if !shouldMigrate {
		log.Println("migrations: AUTO_MIGRATE not enabled, skipping startup migrator")
		return nil
	}

	log.Println("migrations: Starting automatic migration runner...")

	// 1. Create migrations tracking table if it doesn't exist
	if err := createMigrationsTable(db); err != nil {
		return fmt.Errorf("migrations: failed to create tracking table: %w", err)
	}

	// 2. Get the migrations folder
	migrationsDir, err := getMigrationsDir()
	if err != nil {
		return fmt.Errorf("migrations: failed to locate migrations folder: %w", err)
	}

	// 3. Read all .sql files from the migrations folder
	files, err := ioutil.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("migrations: failed to read migrations folder: %w", err)
	}

	// Filter for .sql files and sort by name (ensures consistent order)
	var sqlFiles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".sql") {
			sqlFiles = append(sqlFiles, f.Name())
		}
	}
	sort.Strings(sqlFiles)

	if len(sqlFiles) == 0 {
		log.Println("migrations: No SQL files found in migrations folder")
		return nil
	}

	log.Printf("migrations: Found %d migration file(s)\n", len(sqlFiles))

	// 4. For each migration file, check if it's been applied already
	for _, fileName := range sqlFiles {
		filePath := filepath.Join(migrationsDir, fileName)

		// Check if already applied
		applied, err := isMigrationApplied(db, fileName)
		if err != nil {
			return fmt.Errorf("migrations: failed to check if %s was applied: %w", fileName, err)
		}

		if applied {
			log.Printf("migrations: %s already applied, skipping\n", fileName)
			continue
		}

		// Read and execute the migration
		log.Printf("migrations: Applying %s...\n", fileName)
		if err := applyMigration(db, filePath, fileName); err != nil {
			return fmt.Errorf("migrations: failed to apply %s: %w", fileName, err)
		}

		log.Printf("migrations: ✓ %s applied successfully\n", fileName)
	}

	log.Println("migrations: All pending migrations applied successfully")
	return nil
}

// createMigrationsTable creates the tracking table for migrations if it doesn't exist.
// Uses Firebird syntax with GEN_ID for auto-increment.
func createMigrationsTable(db *sql.DB) error {
	// Firebird doesn't support IF NOT EXISTS for CREATE TABLE/GENERATOR,
	// so we must probe metadata first.
	var tableCount int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM RDB$RELATIONS
		WHERE RDB$RELATION_NAME = 'RDB$MIGRATIONS'
	`).Scan(&tableCount)
	if err != nil {
		return fmt.Errorf("failed to query relation metadata: %w", err)
	}

	if tableCount == 0 {
		createTableSQL := `
			CREATE TABLE RDB$MIGRATIONS (
				ID INTEGER PRIMARY KEY,
				NAME VARCHAR(255) NOT NULL UNIQUE,
				APPLIED_AT TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				STATUS VARCHAR(50) DEFAULT 'APPLIED'
			)
		`
		if _, err := db.Exec(createTableSQL); err != nil {
			return fmt.Errorf("failed to create RDB$MIGRATIONS table: %w", err)
		}
		log.Println("migrations: RDB$MIGRATIONS tracking table created")
	} else {
		log.Println("migrations: RDB$MIGRATIONS table already exists")
	}

	var generatorCount int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM RDB$GENERATORS
		WHERE RDB$GENERATOR_NAME = 'GEN_RDB_MIGRATIONS_ID'
	`).Scan(&generatorCount)
	if err != nil {
		return fmt.Errorf("failed to query generator metadata: %w", err)
	}

	if generatorCount == 0 {
		if _, err := db.Exec("CREATE GENERATOR GEN_RDB_MIGRATIONS_ID"); err != nil {
			return fmt.Errorf("failed to create GEN_RDB_MIGRATIONS_ID: %w", err)
		}
		log.Println("migrations: GEN_RDB_MIGRATIONS_ID created")
	}

	return nil
}

// isMigrationApplied checks if a migration has already been applied.
func isMigrationApplied(db *sql.DB, fileName string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM RDB$MIGRATIONS WHERE NAME = ?",
		fileName,
	).Scan(&count)

	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	return count > 0, nil
}

// applyMigration reads and executes a single migration file.
func applyMigration(db *sql.DB, filePath, fileName string) error {
	// Read the SQL file
	sqlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	sqlContent := stripSQLComments(string(sqlBytes))

	// Start a transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute the migration SQL
	// Split by semicolon to handle multiple statements
	statements := strings.Split(sqlContent, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		_, err := tx.Exec(stmt)
		if err != nil {
			return fmt.Errorf("failed to execute statement in %s: %w\nSQL: %s", fileName, err, stmt)
		}
	}

	// Record the migration in the tracking table
	_, err = tx.Exec(
		"INSERT INTO RDB$MIGRATIONS (ID, NAME, APPLIED_AT, STATUS) VALUES (GEN_ID(GEN_RDB_MIGRATIONS_ID, 1), ?, ?, 'APPLIED')",
		fileName,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// stripSQLComments removes single-line SQL comments so comment-only blocks
// are not sent to Firebird as executable statements.
func stripSQLComments(sqlContent string) string {
	lines := strings.Split(sqlContent, "\n")
	var cleaned []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip full-line comments
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		// Strip inline comment suffix, if present
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}

		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, "\n")
}

// getMigrationsDir locates the migrations folder.
// It checks relative to the executable, then relative to the current working directory.
func getMigrationsDir() (string, error) {
	// Try relative to current working directory first
	cwd, err := os.Getwd()
	if err == nil {
		path := filepath.Join(cwd, "migrations")
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}

	// Try relative to gateway folder
	path := filepath.Join(cwd, "gateway", "migrations")
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path, nil
	}

	// Try parent directory
	parentPath := filepath.Join(filepath.Dir(cwd), "migrations")
	if info, err := os.Stat(parentPath); err == nil && info.IsDir() {
		return parentPath, nil
	}

	return "", fmt.Errorf("migrations folder not found in expected locations")
}
