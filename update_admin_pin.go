package main

import (
	"database/sql"
	"fmt"

	_ "github.com/nakagami/firebirdsql"
)

func updateAdminPin() {
	// Connect to Firebird database
	dsn := "sysdba:masterkey@localhost:3050/C:\\Users\\herna\\aweh.pos\\dinem.fdb?auth_plugin_name=Legacy_Auth&wire_crypt=false"

	db, err := sql.Open("firebirdsql", dsn)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer db.Close()

	// Update ADMIN PIN to 28985
	query := `UPDATE EMPLOYEE SET PIN = '28985' WHERE USERNO = 1`
	result, err := db.Exec(query)
	if err != nil {
		fmt.Printf("Failed to update: %v\n", err)
		return
	}

	rows, err := result.RowsAffected()
	if err != nil {
		fmt.Printf("Failed to get rows affected: %v\n", err)
		return
	}

	fmt.Printf("✅ Updated %d row(s). ADMIN PIN is now: 28985\n", rows)
}
