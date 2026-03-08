package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/nakagami/firebirdsql"
)

type ColumnInfo struct {
	Name          string
	Type          string
	Length        int
	Precision     int
	Scale         int
	IsNullable    string
	DefaultSource string
}

func main() {
	// Local database
	localDSN := "SYSDBA:profes@localhost:3050/c:/Users/herna/aweh.pos/dinem.fdb?auth_plugin_name=Legacy_Auth&wire_crypt=false"
	
	// Production database
	prodDSN := "SYSDBA:profes@100.100.206.41:3050//var/lib/firebird/3.0/data/aweh_test/dinem.fdb?auth_plugin_name=Srp256&wire_crypt=true"

	fmt.Println("=== FIREBIRD SCHEMA COMPARISON TOOL ===")
	fmt.Println()

	// Get local schema
	fmt.Println("Connecting to LOCAL database...")
	localCols, err := getTableColumns(localDSN, "DMASTER")
	if err != nil {
		log.Fatalf("Error reading local schema: %v", err)
	}
	fmt.Printf("✓ Found %d columns in LOCAL DMASTER\n\n", len(localCols))

	// Get production schema
	fmt.Println("Connecting to PRODUCTION database...")
	prodCols, err := getTableColumns(prodDSN, "DMASTER")
	if err != nil {
		log.Fatalf("Error reading production schema: %v", err)
	}
	fmt.Printf("✓ Found %d columns in PRODUCTION DMASTER\n\n", len(prodCols))

	// Compare
	fmt.Println("=== COMPARISON RESULTS ===")
	fmt.Println()

	missingInProd := findMissing(localCols, prodCols)
	missingInLocal := findMissing(prodCols, localCols)
	
	if len(missingInProd) > 0 {
		fmt.Printf("❌ %d columns exist in LOCAL but MISSING in PRODUCTION:\n", len(missingInProd))
		for _, col := range missingInProd {
			fmt.Printf("   - %s (%s)\n", col.Name, col.Type)
		}
		fmt.Println()
	} else {
		fmt.Println("✓ No columns missing in production")
		fmt.Println()
	}

	if len(missingInLocal) > 0 {
		fmt.Printf("⚠️  %d columns exist in PRODUCTION but not in LOCAL:\n", len(missingInLocal))
		for _, col := range missingInLocal {
			fmt.Printf("   - %s (%s)\n", col.Name, col.Type)
		}
		fmt.Println()
	}

	// Generate migration SQL
	if len(missingInProd) > 0 {
		fmt.Println("=== GENERATED MIGRATION SQL ===")
		fmt.Println()
		migrationSQL := generateMigrationSQL("DMASTER", missingInProd, localCols)
		fmt.Println(migrationSQL)
		fmt.Println()
		fmt.Println("=== SAVE THIS TO: migrations/010_add_missing_columns.sql ===")
	} else {
		fmt.Println("✓ Schemas are in sync - no migration needed!")
	}
}

func getTableColumns(dsn string, tableName string) (map[string]ColumnInfo, error) {
	db, err := sqlx.Connect("firebirdsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}
	defer db.Close()

	query := `
		SELECT 
			TRIM(rf.RDB$FIELD_NAME) AS field_name,
			TRIM(t.RDB$TYPE_NAME) AS field_type,
			f.RDB$FIELD_LENGTH AS field_length,
			f.RDB$FIELD_PRECISION AS field_precision,
			f.RDB$FIELD_SCALE AS field_scale,
			CASE WHEN rf.RDB$NULL_FLAG = 1 THEN 'NO' ELSE 'YES' END AS is_nullable,
			TRIM(rf.RDB$DEFAULT_SOURCE) AS default_source
		FROM RDB$RELATION_FIELDS rf
		JOIN RDB$FIELDS f ON rf.RDB$FIELD_SOURCE = f.RDB$FIELD_NAME
		LEFT JOIN RDB$TYPES t ON f.RDB$FIELD_TYPE = t.RDB$TYPE 
			AND t.RDB$FIELD_NAME = 'RDB$FIELD_TYPE'
		WHERE rf.RDB$RELATION_NAME = ?
		ORDER BY rf.RDB$FIELD_POSITION
	`

	type Row struct {
		FieldName     string `db:"FIELD_NAME"`
		FieldType     string `db:"FIELD_TYPE"`
		FieldLength   int    `db:"FIELD_LENGTH"`
		FieldPrecision *int   `db:"FIELD_PRECISION"`
		FieldScale    int    `db:"FIELD_SCALE"`
		IsNullable    string `db:"IS_NULLABLE"`
		DefaultSource *string `db:"DEFAULT_SOURCE"`
	}

	var rows []Row
	err = db.Select(&rows, query, tableName)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}

	cols := make(map[string]ColumnInfo)
	for _, r := range rows {
		precision := 0
		if r.FieldPrecision != nil {
			precision = *r.FieldPrecision
		}
		defaultSrc := ""
		if r.DefaultSource != nil {
			defaultSrc = *r.DefaultSource
		}
		
		cols[r.FieldName] = ColumnInfo{
			Name:          r.FieldName,
			Type:          r.FieldType,
			Length:        r.FieldLength,
			Precision:     precision,
			Scale:         r.FieldScale,
			IsNullable:    r.IsNullable,
			DefaultSource: defaultSrc,
		}
	}

	return cols, nil
}

func findMissing(source, target map[string]ColumnInfo) []ColumnInfo {
	var missing []ColumnInfo
	for name, col := range source {
		if _, exists := target[name]; !exists {
			missing = append(missing, col)
		}
	}
	
	// Sort alphabetically
	sort.Slice(missing, func(i, j int) bool {
		return missing[i].Name < missing[j].Name
	})
	
	return missing
}

func generateMigrationSQL(tableName string, missingCols []ColumnInfo, allLocalCols map[string]ColumnInfo) string {
	var sb strings.Builder
	
	sb.WriteString("-- Migration: Add missing columns to production\n")
	sb.WriteString("-- Generated: " + strings.Replace(fmt.Sprintf("%v", missingCols), "\n", "", -1) + "\n")
	sb.WriteString("-- Apply this to production database\n\n")
	
	for _, col := range missingCols {
		sqlType := mapFirebirdType(col)
		nullable := ""
		if col.IsNullable == "NO" {
			nullable = " NOT NULL"
		}
		
		defaultVal := ""
		if col.DefaultSource != "" {
			defaultVal = " " + col.DefaultSource
		}
		
		sb.WriteString(fmt.Sprintf("ALTER TABLE %s ADD %s %s%s%s;\n", 
			tableName, col.Name, sqlType, defaultVal, nullable))
	}
	
	sb.WriteString("\n-- Add indexes for new columns if needed\n")
	for _, col := range missingCols {
		// Add index for commonly queried columns
		if strings.Contains(col.Name, "SKU") || strings.Contains(col.Name, "ID") {
			sb.WriteString(fmt.Sprintf("CREATE INDEX IDX_%s_%s ON %s(%s);\n", 
				tableName, col.Name, tableName, col.Name))
		}
	}
	
	sb.WriteString("\nCOMMIT;\n")
	
	return sb.String()
}

func mapFirebirdType(col ColumnInfo) string {
	switch col.Type {
	case "SHORT":
		return "SMALLINT"
	case "LONG":
		if col.Scale < 0 {
			return fmt.Sprintf("NUMERIC(9,%d)", -col.Scale)
		}
		return "INTEGER"
	case "INT64":
		if col.Scale < 0 {
			return fmt.Sprintf("NUMERIC(18,%d)", -col.Scale)
		}
		return "BIGINT"
	case "DOUBLE":
		return "DOUBLE PRECISION"
	case "FLOAT":
		return "FLOAT"
	case "TEXT":
		if col.Length > 0 {
			return fmt.Sprintf("VARCHAR(%d)", col.Length)
		}
		return "VARCHAR(255)"
	case "VARYING":
		return fmt.Sprintf("VARCHAR(%d)", col.Length)
	case "BLOB":
		return "BLOB SUB_TYPE TEXT"
	case "TIMESTAMP":
		return "TIMESTAMP"
	case "DATE":
		return "DATE"
	case "TIME":
		return "TIME"
	case "BOOLEAN":
		return "BOOLEAN"
	default:
		if col.Length > 0 {
			return fmt.Sprintf("VARCHAR(%d)", col.Length)
		}
		return "VARCHAR(255)"
	}
}
