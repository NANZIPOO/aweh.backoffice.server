package models

import (
	"database/sql"
	"encoding/json"
)

// ─── NullString ───────────────────────────────────────────────────────────────

// NullString wraps sql.NullString so that it marshals to a plain JSON string
// or JSON null — instead of {"String":"...","Valid":true/false}.
// sqlx can still scan Firebird columns into it via the embedded Scan method.
type NullString struct {
	sql.NullString
}

func (n NullString) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.String)
}

// NullStringFrom creates a valid NullString from a plain string.
func NullStringFrom(s string) NullString {
	return NullString{sql.NullString{String: s, Valid: true}}
}

// ─── NullInt64 ────────────────────────────────────────────────────────────────

// NullInt64 wraps sql.NullInt64 so that it marshals to a plain JSON number
// or JSON null — instead of {"Int64":...,"Valid":true/false}.
// sqlx can still scan Firebird columns into it via the embedded Scan method.
type NullInt64 struct {
	sql.NullInt64
}

func (n NullInt64) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Int64)
}

// NullInt64From creates a valid NullInt64 from a plain int64.
func NullInt64From(i int64) NullInt64 {
	return NullInt64{sql.NullInt64{Int64: i, Valid: true}}
}

// ─── Shared custom type for VARCHAR(1) flags ('T'/'F') ───────────────────────
type FBBoolChar string

func (f FBBoolChar) IsTrue() bool { return f == "T" || f == "t" }

// Generators extracted from Firebird schema
const (
	GenEmployee   = "employee_gen"
	GenBills      = "bills_gen"
	GenOrders     = "ORDERS_GEN"       // EXPENSES.ORDERNO / CREDITORSLEDGER.ORDERNO
	GenOrderItems = "LINE_ORDERNO_GEN" // PITEMS.ITEMNO
	GenSuppliers  = "suppliers_gen"    // SUPPLIERS.ITEMNO
)

// Struct for EMPLOYEE table as seen in Dinem_MetaData_Dll.sql
type Employee struct {
	UserNo       int16          `db:"USERNO"              json:"user_no"`
	ID           sql.NullInt32  `db:"ID"                  json:"id,omitempty"`
	PIN          string         `db:"PIN"                 json:"-"` // Hidden from JSON
	CardNo       sql.NullString `db:"CARDNO"              json:"card_no,omitempty"`
	FirstName    sql.NullString `db:"FIRSTNAME"           json:"first_name,omitempty"`
	LastName     sql.NullString `db:"LASTNAME"            json:"last_name,omitempty"`
	AccessLevel  sql.NullString `db:"ACCESSLEVEL"         json:"access_level,omitempty"`
	IsClockedIn  int16          `db:"ISCLOCKEDIN"         json:"is_clocked_in"`
	ClockedIn    sql.NullTime   `db:"CLOCKEDIN"           json:"clocked_in,omitempty"`
	ClockedOut   sql.NullTime   `db:"CLOCKEDOUT"          json:"clocked_out,omitempty"`
	HourlyRate   float64        `db:"HOURLYRATE"          json:"hourly_rate"`
	TOC          int16          `db:"TOC"                 json:"toc"`
	ConfirmOrder int16          `db:"CONFIRMORDER"        json:"confirm_order"`
	CanVoid      int16          `db:"CANVOID"             json:"can_void"`
	// Add additional fields as needed from mapping logic
}

// SaleItems struct mappings
type SaleItem struct {
	ItemNo      int32          `db:"ITEMNO"        json:"item_no"`
	CheckNo     int32          `db:"CHECKNO"       json:"check_no"`
	Description sql.NullString `db:"DESCRIPTION"   json:"description,omitempty"`
	Qty         float64        `db:"QTY"           json:"qty"`
	Price       float64        `db:"PRICE"         json:"price"`
	Discount    float64        `db:"DISCOUNT"      json:"discount"`
}

// Helper to construct Generator Queries
func NextIDQuery(genName string) string {
	return "SELECT GEN_ID(" + genName + ", 1) FROM RDB$DATABASE"
}
