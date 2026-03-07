package models

import "time"

// DashboardSummary is the root response model containing all dashboard metrics
type DashboardSummary struct {
	Period      string              `json:"period"`
	From        time.Time           `json:"from"`
	To          time.Time           `json:"to"`
	Metrics     SalesMetrics        `json:"metrics"`
	Payment     PaymentBreakdown    `json:"payment_breakdown"`
	Operational OperationalStatus   `json:"operational"`
	Comparison  *ComparisonMetrics  `json:"comparison,omitempty"`
}

// SalesMetrics represents core revenue and transaction data
type SalesMetrics struct {
	GrossSales       float64 `json:"gross_sales" db:"GROSSSALES"`
	NetSales         float64 `json:"net_sales" db:"NETSALES"`
	Tax              float64 `json:"tax" db:"TAX"`
	Discounts        float64 `json:"discounts" db:"DISCOUNT"`
	Voids            float64 `json:"voids" db:"VOIDS"`
	TransactionCount int     `json:"transaction_count" db:"TRANSACTION_COUNT"`
	GuestCount       int     `json:"guest_count" db:"GUEST_COUNT"`
	AvgTransaction   float64 `json:"avg_transaction"`   // Computed: GrossSales / TransactionCount
	AvgPerHead       float64 `json:"avg_per_head"`      // Computed: GrossSales / GuestCount
}

// PaymentBreakdown shows payment method distribution
type PaymentBreakdown struct {
	Cash        float64 `json:"cash" db:"CASH"`
	CashPct     float64 `json:"cash_pct"`
	Card        float64 `json:"card" db:"CARD"`
	CardPct     float64 `json:"card_pct"`
	Account     float64 `json:"account" db:"ACCOUNT"`
	AccountPct  float64 `json:"account_pct"`
	Voucher     float64 `json:"voucher" db:"VOUCHER"`
	VoucherPct  float64 `json:"voucher_pct"`
	Total       float64 `json:"total"`
}

// OperationalStatus shows real-time operational metrics
type OperationalStatus struct {
	OpenChecksCount int `json:"open_checks_count" db:"OPEN_CHECKS_COUNT"`
	ActiveStaffCount int `json:"active_staff_count" db:"ACTIVE_STAFF_COUNT"`
}

// ComparisonMetrics shows period-over-period variance
type ComparisonMetrics struct {
	Period           string    `json:"period"`             // e.g., "yesterday", "last_week"
	From             time.Time `json:"from"`
	To               time.Time `json:"to"`
	GrossSales       float64   `json:"gross_sales"`
	TransactionCount int       `json:"transaction_count"`
	VariancePct      float64   `json:"variance_pct"`       // Percentage change
	VarianceAmount   float64   `json:"variance_amount"`    // Absolute change
}

// RawSalesData is used for sqlx scanning from BILLSHISTORY/BILLS
type RawSalesData struct {
	GrossSales       float64 `db:"GROSSSALES"`
	NetSales         float64 `db:"NETSALES"`
	Tax              float64 `db:"TAX"`
	Discount         float64 `db:"DISCOUNT"`
	Voids            float64 `db:"VOIDS"`
	TransactionCount int     `db:"TRANSACTION_COUNT"`
	GuestCount       int     `db:"GUEST_COUNT"`
	Cash             float64 `db:"CASH"`
	Card             float64 `db:"CARD"`
	Account          float64 `db:"ACCOUNT"`
	Voucher          float64 `db:"VOUCHER"`
}

// RawOperationalData is used for sqlx scanning from BILLS (live checks)
type RawOperationalData struct {
	OpenChecksCount  int `db:"OPEN_CHECKS_COUNT"`
	ActiveStaffCount int `db:"ACTIVE_STAFF_COUNT"`
}
