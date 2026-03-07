package models

import (
	"database/sql"
	"time"
)

// Bill maps directly to the BILLS Firebird table (1:1, no joins).
// DOUBLE PRECISION DEFAULT 0 columns are float64 — they are POS display aggregates,
// not ledger entries, and are always non-null in practice.
// Columns without NOT NULL / DEFAULT are sql.Null* so a NULL scan never panics.
type Bill struct {
	CheckNo       int32          `db:"CHECKNO"       json:"check_no"`
	TillNo        sql.NullInt32  `db:"TILLNO"        json:"till_no,omitempty"`
	TableNo       sql.NullString `db:"TABLENO"       json:"table_no,omitempty"`
	TabName       sql.NullString `db:"TABNAME"       json:"tab_name,omitempty"`
	BillOpen      FBBoolChar     `db:"BILLOPEN"      json:"bill_open"`
	Printed       FBBoolChar     `db:"PRINTED"       json:"printed"`
	InUse         FBBoolChar     `db:"INUSE"         json:"in_use"`
	CashedUp      FBBoolChar     `db:"CASHEDUP"      json:"cashed_up"`
	UserNo        sql.NullInt32  `db:"USERNO"        json:"user_no,omitempty"`
	Cashier       sql.NullString `db:"CASHIER"       json:"cashier,omitempty"`
	Pax           sql.NullInt32  `db:"PAX"           json:"pax,omitempty"`
	NetAmount     float64        `db:"NETAMOUNT"     json:"net_amount"`
	Cash          float64        `db:"CASH"          json:"cash"`
	CreditCard    float64        `db:"CREDITCARD"    json:"credit_card"`
	Voucher       float64        `db:"VOUCHER"       json:"voucher"`
	Checks        float64        `db:"CHECKS"        json:"checks"`
	PaidOut       float64        `db:"PAIDOUT"       json:"paid_out"`
	Promos        float64        `db:"PROMOS"        json:"promos"`
	Discount      float64        `db:"DISCOUNT"      json:"discount"`
	Voids         float64        `db:"VOIDS"         json:"voids"`
	Staff         float64        `db:"STAFF"         json:"staff"`
	Account       float64        `db:"ACCOUNT"       json:"account"`
	Surcharge     float64        `db:"SURCHARGE"     json:"surcharge"`
	SalesTax      float64        `db:"SALESTAX"      json:"sales_tax"`
	Tip           float64        `db:"TIP"           json:"tip"`
	GrandTotal    float64        `db:"GRANDTOTAL"    json:"grand_total"`
	PayType       sql.NullString `db:"PAYTYPE"       json:"pay_type,omitempty"`
	AccReceived   float64        `db:"ACCRECEIVED"   json:"acc_received"`
	Breakages     float64        `db:"BREAKAGES"     json:"breakages"`
	PoolTips      float64        `db:"POOLTIPS"      json:"pool_tips"`
	CardComm      float64        `db:"CARDCOMM"      json:"card_comm"`
	OutletNo      sql.NullInt32  `db:"OUTLETNO"      json:"outlet_no,omitempty"`
	TDate         sql.NullTime   `db:"TDATE"         json:"t_date,omitempty"`
	OpenTime      sql.NullTime   `db:"OPENTIME"      json:"open_time,omitempty"`
	ClosedTime    sql.NullTime   `db:"CLOSEDTIME"    json:"closed_time,omitempty"`
	ClosedBy      sql.NullString `db:"CLOSEDBY"      json:"closed_by,omitempty"`
	BusinessDay   sql.NullTime   `db:"BUSINESSDAY"   json:"business_day,omitempty"`
	OutletName    sql.NullString `db:"OUTLETNAME"    json:"outlet_name,omitempty"`
	SetCalc       FBBoolChar     `db:"SETCALC"       json:"set_calc"`
	OrderMemo     sql.NullString `db:"ORDERMEMO"     json:"order_memo,omitempty"`
	SalesCategory sql.NullString `db:"SALESCATEGORY" json:"sales_category,omitempty"`
	FooterMemo    sql.NullString `db:"FOOTERMEMO"    json:"footer_memo,omitempty"`
	// BILLEDTIME is a Firebird TIME-only column; nakagami maps it to time.Time
	BilledTime sql.NullTime `db:"BILLEDTIME"    json:"billed_time,omitempty"`
}

// CreateBillRequest is the body the caller POSTs to open a new bill.
// The system assigns: CHECKNO (GEN_ID), TDATE, OPENTIME, status flags, zeroed totals.
type CreateBillRequest struct {
	TillNo        *int32     `json:"till_no"`
	TableNo       *string    `json:"table_no"`
	TabName       *string    `json:"tab_name"`
	UserNo        *int32     `json:"user_no"`
	Cashier       *string    `json:"cashier"`
	Pax           *int32     `json:"pax"`
	OutletNo      *int32     `json:"outlet_no"`
	OutletName    *string    `json:"outlet_name"`
	SalesCategory *string    `json:"sales_category"`
	BusinessDay   *time.Time `json:"business_day"`
	OrderMemo     *string    `json:"order_memo"`
}

// CloseBillRequest is the body sent when settling a bill at the POS terminal.
// All monetary fields default to 0 if absent — the caller must supply the full
// payment breakdown so totals on the BILLS row are audit-ready at close time.
type CloseBillRequest struct {
	ClosedBy    string  `json:"closed_by"`
	PayType     string  `json:"pay_type"`
	NetAmount   float64 `json:"net_amount"`
	GrandTotal  float64 `json:"grand_total"`
	Cash        float64 `json:"cash"`
	CreditCard  float64 `json:"credit_card"`
	Voucher     float64 `json:"voucher"`
	Checks      float64 `json:"checks"`
	PaidOut     float64 `json:"paid_out"`
	Promos      float64 `json:"promos"`
	Discount    float64 `json:"discount"`
	Voids       float64 `json:"voids"`
	Staff       float64 `json:"staff"`
	Account     float64 `json:"account"`
	Surcharge   float64 `json:"surcharge"`
	SalesTax    float64 `json:"sales_tax"`
	Tip         float64 `json:"tip"`
	AccReceived float64 `json:"acc_received"`
	Breakages   float64 `json:"breakages"`
	PoolTips    float64 `json:"pool_tips"`
	CardComm    float64 `json:"card_comm"`
}

// VoidBillRequest carries the name of whoever authorised the void.
type VoidBillRequest struct {
	ClosedBy string `json:"closed_by"`
}
