package main

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	_ "github.com/nakagami/firebirdsql"
)

func main() {
	dsn := "sysdba:profes@localhost:3050/c:\\Users\\herna\\aweh.pos\\dinem.fdb?auth_plugin_name=Legacy_Auth&wire_crypt=false"
	db, err := sqlx.Open("firebirdsql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Query column metadata
	rows, err := db.Queryx(`
		SELECT 
			f.rdb$field_name AS field_name,
			t.rdb$type_name AS field_type,
			f.rdb$field_length AS field_length,
			f.rdb$field_precision AS field_precision
		FROM rdb$relation_fields rf
		JOIN rdb$fields f ON rf.rdb$field_source = f.rdb$field_name
		LEFT JOIN rdb$types t ON f.rdb$field_type = t.rdb$type AND t.rdb$field_name = 'RDB$FIELD_TYPE'
		WHERE rf.rdb$relation_name = 'DMASTER'
		  AND rf.rdb$field_name IN ('IS_BASE_VARIANT', 'IS_SELLABLE', 'GROUP_ID')
		ORDER BY rf.rdb$field_position
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("DMASTER Column Definitions:")
	for rows.Next() {
		row := make(map[string]interface{})
		rows.MapScan(row)
		fmt.Printf("  %v\n", row)
	}
}
