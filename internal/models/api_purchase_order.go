package models

// ---------------------------------------------------------------------------
// Request bodies (Flutter → Go)
// All monetary fields carry string to avoid IEEE-754 float rounding.
// ---------------------------------------------------------------------------

type CreateOrderRequest struct {
	SupplierNo   string `json:"supplier_no"`
	SupplierName string `json:"supplier_name"`
}

type AddLineItemRequest struct {
	MPartNo      string `json:"mpart_no"`
	Qty          string `json:"qty"`           // decimal as string; "0" triggers ONORDER default logic
	VatInclusive bool   `json:"vat_inclusive"` // if true, strip VAT from pack_cost before saving
}

type UpdateLineItemRequest struct {
	Qty      string `json:"qty"`
	PackCost string `json:"pack_cost"`
}

type CaptureInvoiceRequest struct {
	InvoicedAmount string `json:"invoiced_amount"`
	Discount       string `json:"discount"`
	Ullages        string `json:"ullages"`
}

type PostInvoiceRequest struct {
	InvoiceNumber string  `json:"invoice_number"`
	InvoiceDate   string  `json:"invoice_date"`  // RFC3339
	ReceivedDate  string  `json:"received_date"` // RFC3339
	PayMethod     string  `json:"pay_method"`    // CASH|EFT|CHEQUE|CR.CARD|NOT PAID
	PayReference  string  `json:"pay_reference"` // required when pay_method=CHEQUE
	DueDate       *string `json:"due_date"`      // omit / null when pay_method=NOT PAID
}

type UpdateCostLine struct {
	MPartNo  string `json:"mpart_no"`
	PackCost string `json:"pack_cost"`
	EachCost string `json:"each_cost"`
	UnitCost string `json:"unit_cost"`
}

type UpdateCostsRequest struct {
	Lines []UpdateCostLine `json:"lines"`
}

// ---------------------------------------------------------------------------
// Response bodies (Go → Flutter)
// ---------------------------------------------------------------------------

type OrderSummary struct {
	OrderNo      int64  `json:"order_no"`
	SupplierNo   string `json:"supplier_no"`
	SupplierName string `json:"supplier_name"`
	OrderDate    string `json:"order_date"`
	Status       string `json:"status"` // "draft" | "posted"
	GrandTotal   string `json:"grand_total"`
}

type OrderDetail struct {
	OrderNo      int64             `json:"order_no"`
	SupplierNo   string            `json:"supplier_no"`
	SupplierName string            `json:"supplier_name"`
	OrderDate    string            `json:"order_date"`
	Status       string            `json:"status"`
	NettTotal    string            `json:"nett_total"`
	VAT          string            `json:"vat"`
	Discount     string            `json:"discount"`
	Ullages      string            `json:"ullages"`
	GrandTotal   string            `json:"grand_total"`
	Lines        []OrderLineDetail `json:"lines"`
}

type OrderLineDetail struct {
	ItemNo       int64  `json:"item_no"`
	MPartNo      string `json:"mpart_no"`
	Description  string `json:"description"`
	Qty          string `json:"qty"`
	Pack         string `json:"pack"`
	PackCost     string `json:"pack_cost"`
	EachCost     string `json:"each_cost"`
	TaxRate      int    `json:"tax_rate"`
	Discount     string `json:"discount"`
	ExtCost      string `json:"ext_cost"`   // qty × pack_cost
	VatAmount    string `json:"vat_amount"` // ext_cost × (tax_rate/100)
	LineTotal    string `json:"line_total"` // ext_cost + vat_amount
	CostGroup    string `json:"cost_group"`
	CostCategory string `json:"cost_category"`
	PackUnit     string `json:"pack_unit"`
	EachUnit     string `json:"each_unit"`
}

type OrderTotals struct {
	NettTotal  string  `json:"nett_total"`
	VAT        string  `json:"vat"`
	GrandTotal string  `json:"grand_total"`
	Difference string  `json:"difference"`
	Warning    *string `json:"warning,omitempty"` // present when 0 < |diff| ≤ 1.00
}

type PostInvoiceResponse struct {
	OrderNo       int64  `json:"order_no"`
	Status        string `json:"status"`
	InvoiceNumber string `json:"invoice_number"`
	GrandTotal    string `json:"grand_total"`
	PostedAt      string `json:"posted_at"`
}

type UpdateCostsResponse struct {
	UpdatedCount int `json:"updated_count"`
	SkippedCount int `json:"skipped_count"`
}

type SupplierItem struct {
	MPartNo      string `json:"mpart_no"`
	Description  string `json:"description"`
	PackCost     string `json:"pack_cost"`
	EachCost     string `json:"each_cost"`
	Pack         string `json:"pack"`
	Units        int64  `json:"units"`
	OnOrder      string `json:"on_order"`
	TaxRate      int    `json:"tax_rate"`
	CostGroup    string `json:"cost_group"`
	CostCategory string `json:"cost_category"`
	PackUnit     string `json:"pack_unit"`
	EachUnit     string `json:"each_unit"`
	Packaging    string `json:"packaging"`
}

type SupplierResponse struct {
	ItemNo       int64  `json:"item_no"`
	SupplierNo   string `json:"supplier_no"`
	SupplierName string `json:"supplier_name"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Contact      string `json:"contact"`
	Address      string `json:"address"`
}

// APIError is the envelope for all error responses.
type APIError struct {
	Error string `json:"error"`
}
