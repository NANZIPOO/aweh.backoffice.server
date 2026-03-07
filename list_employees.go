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

	var emps []map[string]interface{}
	rows, err := db.Queryx("SELECT USERNO, FIRSTNAME, PIN, ACCESSLEVEL FROM EMPLOYEE")
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		row := make(map[string]interface{})
		err := rows.MapScan(row)
		if err != nil {
			log.Fatal(err)
		}
		emps = append(emps, row)
	}

	for i, emp := range emps {
		fmt.Printf("%d: %v\n", i, emp)
	}
}
