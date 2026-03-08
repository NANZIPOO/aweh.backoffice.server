package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/nakagami/firebirdsql"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run apply_migration.go <migration_file>")
	}

	migrationFile := os.Args[1]
	
	// Production database
	prodDSN := "SYSDBA:profes@100.100.206.41:3050//var/lib/firebird/3.0/data/aweh_test/dinem.fdb?auth_plugin_name=Srp256&wire_crypt=true"

	fmt.Println("=== MIGRATION APPLICATION TOOL ===")
	fmt.Printf("Migration file: %s\n", migrationFile)
	fmt.Println("Target: PRODUCTION database (100.100.206.41)")
	fmt.Println()

	// Read migration file
	content, err := os.ReadFile(migrationFile)
	if err != nil {
		log.Fatalf("Error reading migration file: %v", err)
	}

	fmt.Println("Migration SQL:")
	fmt.Println(string(content))
	fmt.Println()

	// Connect to production
	fmt.Println("Connecting to production database...")
	db, err := sqlx.Connect("firebirdsql", prodDSN)
	if err != nil {
		log.Fatalf("Connection error: %v", err)
	}
	defer db.Close()
	fmt.Println("✓ Connected")
	fmt.Println()

	// Parse and execute statements
	statements := parseSQL(string(content))
	
	fmt.Printf("Found %d SQL statements to execute\n", len(statements))
	fmt.Println()

	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}

		fmt.Printf("[%d/%d] Executing: %s...\n", i+1, len(statements), 
			truncate(stmt, 60))

		_, err := db.Exec(stmt)
		if err != nil {
			// Check if error is "column already exists" - not fatal
			if strings.Contains(err.Error(), "already exists") || 
			   strings.Contains(err.Error(), "attempt to store duplicate") {
				fmt.Printf("  ⚠️  Skipped (already exists)\n")
				continue
			}
			log.Fatalf("  ❌ Error: %v\n Statement: %s", err, stmt)
		}
		fmt.Println("  ✓ Success")
	}

	fmt.Println()
	fmt.Println("=== MIGRATION COMPLETE ===")
	fmt.Println("✓ All statements executed successfully")
	
	// Verify columns exist
	fmt.Println()
	fmt.Println("Verifying new columns...")
	verifyColumns(db, []string{"BULKSELLINGPRICE", "LINKEDSKU", "SUPPLIERSKU"})
}

func parseSQL(content string) []string {
	// Split by semicolons, but preserve COMMIT
	lines := strings.Split(content, "\n")
	var statements []string
	var current strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Skip comments
		if strings.HasPrefix(line, "--") {
			continue
		}

		current.WriteString(line)
		current.WriteString(" ")

		// Statement ends with semicolon
		if strings.HasSuffix(line, ";") {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" && stmt != ";" {
				statements = append(statements, stmt)
			}
			current.Reset()
		}
	}

	return statements
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func verifyColumns(db *sqlx.DB, columnNames []string) {
	query := `
		SELECT TRIM(rf.RDB$FIELD_NAME) AS field_name
		FROM RDB$RELATION_FIELDS rf
		WHERE rf.RDB$RELATION_NAME = 'DMASTER'
		  AND TRIM(rf.RDB$FIELD_NAME) IN (?, ?, ?)
	`

	type Row struct {
		FieldName string `db:"FIELD_NAME"`
	}

	var rows []Row
	err := db.Select(&rows, query, columnNames[0], columnNames[1], columnNames[2])
	if err != nil {
		fmt.Printf("  ⚠️  Could not verify: %v\n", err)
		return
	}

	found := make(map[string]bool)
	for _, r := range rows {
		found[r.FieldName] = true
	}

	for _, col := range columnNames {
		if found[col] {
			fmt.Printf("  ✓ %s exists\n", col)
		} else {
			fmt.Printf("  ❌ %s NOT FOUND\n", col)
		}
	}
}
