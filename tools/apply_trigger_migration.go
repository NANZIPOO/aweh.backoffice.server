package main

import (
	"fmt"
	"log"
	"os"

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

	sqlContent, err := os.ReadFile("migrations/005_auto_sync_base_variant.sql")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Applying migration 005_auto_sync_base_variant.sql...")

	_, err = db.Exec(string(sqlContent))
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	fmt.Println("✅ Trigger TRG_DMASTER_BASE_VARIANT_SYNC created successfully")
}
