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

	// Check INVENTORY_GROUPS table
	fmt.Println("=== INVENTORY_GROUPS ===")
	rows1, err := db.Queryx("SELECT FIRST 5 GROUP_ID, BASE_ITEMPARTNO, GROUP_NAME, BASE_UOM FROM INVENTORY_GROUPS ORDER BY GROUP_ID DESC")
	if err != nil {
		log.Fatalf("Query groups failed: %v", err)
	}
	defer rows1.Close()
	count := 0
	for rows1.Next() {
		row := make(map[string]interface{})
		rows1.MapScan(row)
		fmt.Printf("%v\n", row)
		count++
	}
	if count == 0 {
		fmt.Println("(No groups found)")
	}

	fmt.Println("\n=== DMASTER (Latest Group Variants) ===")
	// Check DMASTER for our groups
	rows2, err := db.Queryx(`
		SELECT FIRST 10 ITEMPARTNO, DESCRIPTION, GROUP_ID, UOM, IS_BASE_VARIANT, IS_SELLABLE, PACKCOST, EACHCOST 
		FROM DMASTER 
		WHERE GROUP_ID >= 1007
		ORDER BY ITEMPARTNO DESC
	`)
	if err != nil {
		log.Fatalf("Query DMASTER failed: %v", err)
	}
	defer rows2.Close()
	count2 := 0
	for rows2.Next() {
		row := make(map[string]interface{})
		rows2.MapScan(row)
		fmt.Printf("%v\n", row)
		count2++
	}
	if count2 == 0 {
		fmt.Println("(No variants found)")
	}

	fmt.Println("\n=== STOCK_MOVEMENTS ===")
	// Check STOCK_MOVEMENTS table
	rows3, err := db.Queryx("SELECT FIRST 10 MOVEMENT_ID, GROUP_ID, MOVEMENT_TYPE, QTY_VARIANT FROM STOCK_MOVEMENTS")
	if err != nil {
		log.Fatalf("Query movements failed: %v", err)
	}
	defer rows3.Close()
	count3 := 0
	for rows3.Next() {
		row := make(map[string]interface{})
		rows3.MapScan(row)
		fmt.Printf("%v\n", row)
		count3++
	}
	if count3 == 0 {
		fmt.Println("(No movements yet)")
	}
}
