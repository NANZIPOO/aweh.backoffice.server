package models

import (
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// RULE (Project Constitution §4): db: tags use the EXACT legacy Firebird column
// names, including all misspellings (CATOGORY, REORDERLEVE). The json: tags use
// correct English spelling. Never "fix" the db: tag — those must match the schema.
// ─────────────────────────────────────────────────────────────────────────────

// ─── InventoryItem maps to the DMASTER table ─────────────────────────────────

type InventoryItem struct {
	// Identity
	ItemPartNo  int64      `db:"ITEMPARTNO"    json:"id"`
	MPartNo     string     `db:"MPARTNO"       json:"supplier_part_no"`
	Barcode     NullString `db:"BARCODE"       json:"barcode"`
	// LinkedSKU   NullString `db:"LINKEDSKU"     json:"linked_sku"`      // TODO: Add migration for this column
	// SupplierSKU NullString `db:"SUPPLIERSKU"   json:"supplier_sku"`   // TODO: Add migration for this column
	ItemImage   NullString `db:"ITEM_IMAGE"    json:"item_image"`

	// Classification
	SupplierNo   string     `db:"SUPPLIERNO"    json:"supplier_no"`
	Description  string     `db:"DESCRIPTION"   json:"description"`
	Brand        NullString `db:"BRAND"         json:"brand"`
	Bin          string     `db:"BIN"           json:"bin"`
	CategoryNo   NullString `db:"CATEGORYNO"    json:"category_no"`
	CostCategory NullString `db:"COSTCATEGORY"  json:"cost_category"`
	CostGroup    NullString `db:"COSTGROUP"     json:"cost_group"`
	Category     NullString `db:"CATOGORY"      json:"category"` // legacy typo preserved in db: tag
	InvForm      NullString `db:"INVFORM"       json:"stock_sheet"`
	Packaging    NullString `db:"PACKAGING"     json:"packaging"`
	ZoneID       NullString `db:"ZONEID"        json:"zone_id"`

	// Costing hierarchy
	Pack     float64 `db:"PACK"     json:"pack_size"`
	PackUnit string  `db:"PACKUNIT" json:"pack_unit"`
	PackCost float64 `db:"PACKCOST" json:"pack_cost"`
	EachCost float64 `db:"EACHCOST" json:"each_cost"`
	Units    float64 `db:"UNITS"    json:"units_per_pack"`
	EachUnit string  `db:"EACHUNIT" json:"each_unit"`
	UnitCost float64 `db:"UNITCOST" json:"unit_cost"`

	// Pricing
	Markup           float64 `db:"MARKUP"           json:"markup"`
	SellingPrice     float64 `db:"SELLINGPRICE"     json:"selling_price"`
	BulkSellingPrice float64 `db:"BULKSELLINGPRICE" json:"bulk_selling_price"`
	TaxRate          float64 `db:"TAXRATE"          json:"tax_rate"`
	Discount         float64 `db:"DISCOUNT"         json:"discount"`
	Bucos            float64 `db:"BUCOS"            json:"bulk_cost_override"`

	// Reorder controls
	MinStockLevel int     `db:"MINSTOCKLEVEL" json:"min_stock_level"`
	MaxStockLevel int     `db:"MAXSTOCKLEVEL" json:"max_stock_level"`
	ReorderLevel  int     `db:"REORDERLEVE"   json:"reorder_level"` // legacy typo preserved
	OnOrder       float64 `db:"ONORDER"       json:"on_order"`
	AutoYield     string  `db:"AUTOYIELD"     json:"-"` // raw Firebird 'T'/'F' — use AutoYieldBool
	Weight        float64 `db:"WEIGHT"        json:"weight"`
	Tare          float64 `db:"TARE"          json:"tare"`

	// Period stock summary (not the 56 day-columns)
	FrontOpeningStock float64 `db:"FRONTOPENINGSTOCK" json:"front_opening_stock"`
	BackOpeningStock  float64 `db:"BACKOPENINGSTOCK"  json:"back_opening_stock"`
	FrontClosingStock float64 `db:"FRONTCLOSINGSTOCK" json:"front_closing_stock"`
	BackClosingStock  float64 `db:"BACKCLOSINGSTOCK"  json:"back_closing_stock"`
	Purchases         float64 `db:"PURCHASES"         json:"purchases"`
	Sales             float64 `db:"SALES"             json:"sales"`

	// Product grouping (NEW)
	GroupID           NullInt64  `db:"GROUP_ID"        json:"group_id"`
	UOM               NullString `db:"UOM"            json:"uom"`
	IsBaseVariant     bool       `db:"IS_BASE_VARIANT" json:"is_base_variant"`
	IsSellable        bool       `db:"IS_SELLABLE"     json:"is_sellable"`
	IsOrderingAllowed bool       `db:"IS_ORDERING_ALLOWED" json:"ordering_allowed"`

	// Go-computed fields — db:"-" means sqlx skips scanning these.
	// Populated by the repository after scanning, before returning to handler.
	GPMarginPct   float64 `db:"-" json:"gp_margin_pct"`  // included in list for grid display
	AutoYieldBool bool    `db:"-" json:"auto_yield"`     // 'T'→true, 'F'→false
	CalculatedQty float64 `db:"-" json:"calculated_qty"` // qty in this variant's UOM after base/variant UNITS conversion
}

// ComputedMargins are calculated by the Go layer — never stored in DB.
// Avoids PACK=0 divide-by-zero in Firebird SQL (Project Constitution Appendix A §4).
type ComputedMargins struct {
	EachCost        float64 `json:"each_cost_calc"`
	UnitCost        float64 `json:"unit_cost_calc"`
	Tax             float64 `json:"tax"`
	NettSelling     float64 `json:"nett_selling"`
	GrossProfit     float64 `json:"gross_profit"` // sellingPrice - eachCost
	NettProfit      float64 `json:"nett_profit"`  // nettSelling - eachCost
	GPMargin        float64 `json:"gp_margin_pct"`
	RecSellingPrice float64 `json:"rec_selling_price"` // eachCost * (1 + markup/100)
	AutoYield       bool    `json:"auto_yield"`        // 'T'→true conversion
}

// InventoryItemDetail embeds base item + full computed margins.
// Only returned by GET /api/v1/inventory/items/:id — list endpoint omits full margins object.
type InventoryItemDetail struct {
	InventoryItem
	Margins ComputedMargins `json:"margins"`
}

// ─── InventorySupplier maps to CREDITORS (slim read model for lookup endpoints) ──

type InventorySupplier struct {
	SupplierNo int64      `db:"SUPPLIERNO" json:"id"`
	Name       string     `db:"SUPPLIER"   json:"name"`
	Address1   NullString `db:"ADDRESS1"   json:"address1"`
	Phone      NullString `db:"PHONE"      json:"phone"`
	Email      NullString `db:"EMAIL"      json:"email"`
}

// ─── GRV structs map to CREDITORSLEDGER (header) + ORDERITEMS (lines) ────────

type GrvHeader struct {
	OrderNo    int64     `db:"ORDERNO"    json:"grv_id"`
	SupplierNo int64     `db:"SUPPLIERNO" json:"supplier_id"`
	InvDate    time.Time `db:"INVDATE"    json:"grv_date"`
	NettTotal  float64   `db:"NETTOTAL"   json:"nett_total"`
	VAT        float64   `db:"VAT"        json:"vat"`
	GrandTotal float64   `db:"GRANDTOTAL" json:"grand_total"`
	Received   string    `db:"RECEIVED"   json:"received"` // 'Y' or 'N'
}

type GrvLine struct {
	OrderNo     int64   `db:"ORDERNO"     json:"grv_id"`
	MPartNo     string  `db:"MPARTNO"     json:"item_supplier_part_no"`
	Description string  `db:"DESCRIPTION" json:"description"`
	Qty         float64 `db:"QTY"         json:"qty"`
	Pack        float64 `db:"PACK"        json:"pack_size"`
	PackCost    float64 `db:"PACKCOST"    json:"pack_cost"`
	EachCost    float64 `db:"EACHCOST"    json:"each_cost"`
	TaxRate     float64 `db:"TAXRATE"     json:"tax_rate"`
	Posted      string  `db:"POSTED"      json:"posted"` // 'T'/'F'
}

// GrvResponse is the handler-composed response for POST /api/v1/inventory/grv.
// supplier_name is resolved via a SEPARATE single-table SELECT after commit — never via JOIN.
type GrvResponse struct {
	GrvID           int64   `json:"grv_id"`
	SupplierID      int64   `json:"supplier_id"`
	SupplierName    string  `json:"supplier_name"`
	GrvDate         string  `json:"grv_date"`
	ReferenceNumber string  `json:"reference_number"`
	NettTotal       float64 `json:"nett_total"`
	VAT             float64 `json:"vat"`
	GrandTotal      float64 `json:"grand_total"`
	LinesAccepted   int     `json:"lines_accepted"`
}

// ─── WastageLine maps to ORDERITEMS (SUPPLIER='Wastage Control', POSTED='F') ──

type WastageLine struct {
	OrderNo      int64      `db:"ORDERNO"      json:"order_no"`
	MPartNo      string     `db:"MPARTNO"      json:"item_supplier_part_no"`
	Description  string     `db:"DESCRIPTION"  json:"description"`
	Qty          float64    `db:"QTY"          json:"qty"`
	EachCost     float64    `db:"EACHCOST"     json:"each_cost"`
	PackCost     float64    `db:"PACKCOST"     json:"pack_cost"`
	Pack         float64    `db:"PACK"         json:"pack_size"`
	TaxRate      float64    `db:"TAXRATE"      json:"tax_rate"`
	Category     NullString `db:"CATOGORY"     json:"category"` // legacy typo preserved
	CostCategory NullString `db:"COSTCATEGORY" json:"cost_category"`
	PackUnit     string     `db:"PACKUNIT"     json:"pack_unit"`
	EachUnit     string     `db:"EACHUNIT"     json:"each_unit"`
	OrderDate    time.Time  `db:"ORDERDATE"    json:"wastage_date"`
	Posted       string     `db:"POSTED"       json:"posted"` // always 'F' on create
}

// WastageResponse is the handler-composed response for POST /api/v1/inventory/wastage.
type WastageResponse struct {
	WastageID   int64   `json:"wastage_id"`
	ItemID      string  `json:"item_id"`
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	EachCost    float64 `json:"each_cost"`
	NettCost    float64 `json:"nett_cost"`
	VAT         float64 `json:"vat"`
	TotalCost   float64 `json:"total_cost"`
	WastageDate string  `json:"wastage_date"`
	Posted      bool    `json:"posted"`
}

// ─── StockTakeDayRow — dynamically aliased per day-of-week ───────────────────
//
// Built with day-specific column aliases (see Appendix B in 07_Implementation_Plan.md).
// Example for Monday:
//
//	SELECT MPARTNO, DESCRIPTION, BIN,
//	       MONFOS  AS opening_stock,
//	       MONREC  AS received,
//	       MONFCS  AS closing_stock,
//	       MONSALES AS sales,
//	       (MONSALES - ((MONFOS + MONREC) - MONFCS)) AS variance
//	FROM DMASTER WHERE INVFORM = ? ORDER BY BIN, DESCRIPTION
type StockTakeDayRow struct {
	MPartNo      string  `db:"MPARTNO"        json:"item_id"`
	Description  string  `db:"DESCRIPTION"    json:"description"`
	Bin          string  `db:"BIN"            json:"bin"`
	OpeningStock float64 `db:"opening_stock"  json:"opening_stock"`
	Received     float64 `db:"received"       json:"received"`
	ClosingStock float64 `db:"closing_stock"  json:"closing_stock"`
	Sales        float64 `db:"sales"          json:"sales"`
	Variance     float64 `db:"variance"       json:"variance"`
}

// ─── Lookup structs ───────────────────────────────────────────────────────────

type LookupItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// InventoryLookups is the compound response for GET /api/v1/inventory/lookups.
// One round trip replaces multiple separate INI file reads from the legacy app.
type InventoryLookups struct {
	StockSheets    []LookupItem        `json:"stock_sheets"`
	Bins           []LookupItem        `json:"bins"`
	Categories     []LookupItem        `json:"categories"`
	CostCategories []LookupItem        `json:"cost_categories"`
	Suppliers      []InventorySupplier `json:"suppliers"`
}

// ─── CRUD request types ───────────────────────────────────────────────────────

// CreateItemRequest is the JSON body for POST /api/v1/inventory/items.
type CreateItemRequest struct {
	Description     string  `json:"description"`
	SupplierNo      string  `json:"supplier_no"`
	Category        string  `json:"category"`
	Bin             string  `json:"bin"`
	StockSheet      string  `json:"stock_sheet"`
	CostCategory    string  `json:"cost_category"`
	PackSize        float64 `json:"pack_size"`
	PackUnit        string  `json:"pack_unit"`
	PackCost        float64 `json:"pack_cost"`
	EachCost        float64 `json:"each_cost"`
	UnitsPerPack    float64 `json:"units_per_pack"`
	EachUnit        string  `json:"each_unit"`
	SellingPrice    float64 `json:"selling_price"`
	TaxRate         float64 `json:"tax_rate"`
	Markup          float64 `json:"markup"`
	MinStockLevel   int     `json:"min_stock_level"`
	MaxStockLevel   int     `json:"max_stock_level"`
	ReorderLevel    int     `json:"reorder_level"`
	AutoYield       bool    `json:"auto_yield"`
	OrderingAllowed *bool   `json:"ordering_allowed,omitempty"`
}

// UpdateItemRequest is the JSON body for PUT /api/v1/inventory/items/:id.
// All fields are pointer types — only non-nil fields are updated.
type UpdateItemRequest struct {
	Description      *string  `json:"description"`
	Barcode          *string  `json:"barcode"`
	SupplierNo       *string  `json:"supplier_no"`
	Category         *string  `json:"category"`
	Bin              *string  `json:"bin"`
	StockSheet       *string  `json:"stock_sheet"`
	CostCategory     *string  `json:"cost_category"`
	ItemImage        *string  `json:"item_image"`
	UOM              *string  `json:"uom"`
	IsSellable       *bool    `json:"is_sellable"`
	OrderingAllowed  *bool    `json:"ordering_allowed"`
	Brand            *string  `json:"brand"`
	Packaging        *string  `json:"packaging"`
	PackSize         *float64 `json:"pack_size"`
	PackUnit         *string  `json:"pack_unit"`
	PackCost         *float64 `json:"pack_cost"`
	EachCost         *float64 `json:"each_cost"`
	UnitsPerPack     *float64 `json:"units_per_pack"`
	EachUnit         *string  `json:"each_unit"`
	SellingPrice     *float64 `json:"selling_price"`
	BulkSellingPrice *float64 `json:"bulk_selling_price"`
	TaxRate          *float64 `json:"tax_rate"`
	Markup           *float64 `json:"markup"`
	Discount         *float64 `json:"discount"`
	MinStockLevel    *int     `json:"min_stock_level"`
	MaxStockLevel    *int     `json:"max_stock_level"`
	ReorderLevel     *int     `json:"reorder_level"`
	AutoYield        *bool    `json:"auto_yield"`
}

// AssignBarcodeRequest is the JSON body for POST /api/v1/inventory/items/:id/barcode.
type AssignBarcodeRequest struct {
	Barcode string `json:"barcode"`
}

// CloneItemRequest is the JSON body for POST /api/v1/inventory/items/:id/clone.
type CloneItemRequest struct {
	NewDescription string `json:"new_description"`
}

// AddVariantRequest is the JSON body for POST /api/v1/inventory/groups/:group_id/add-variant.
// Creates a new DMASTER item and assigns it to the specified group.
type AddVariantRequest struct {
	Description     string  `json:"description"`
	SupplierNo      string  `json:"supplier_no"`
	UOM             string  `json:"uom"`
	PackSize        float64 `json:"pack_size"`
	PackUnit        string  `json:"pack_unit"`
	PackCost        float64 `json:"pack_cost"`
	EachCost        float64 `json:"each_cost"`
	UnitsPerPack    float64 `json:"units_per_pack"`
	EachUnit        string  `json:"each_unit"`
	SellingPrice    float64 `json:"selling_price"`
	TaxRate         float64 `json:"tax_rate"`
	Markup          float64 `json:"markup"`
	IsSellable      bool    `json:"is_sellable"`
	OrderingAllowed *bool   `json:"ordering_allowed,omitempty"`
}

// ItemFilter drives dynamic WHERE construction in ListItems.
type ItemFilter struct {
	Search     string
	StockSheet string
	Category   string
	Bin        string
	SupplierNo string
	Page       int
	Limit      int
	SortBy     string // "description"|"each_cost"|"selling_price"|"gp_margin_pct"
	SortDir    string // "asc"|"desc"
}

// ─── Stock-take request types ─────────────────────────────────────────────────

// UpdateClosingStockRequest is the JSON body for PUT /api/v1/inventory/stock-take/closing.
type UpdateClosingStockRequest struct {
	StockSheet string                   `json:"stock_sheet"`
	DayOfWeek  string                   `json:"day_of_week"` // "monday"|"tuesday"|...|"sunday"
	Items      []UpdateClosingStockItem `json:"items"`
}

type UpdateClosingStockItem struct {
	MPartNo           string  `json:"item_id"`
	FrontClosingStock float64 `json:"front_closing_stock"`
	BackClosingStock  float64 `json:"back_closing_stock"`
}

// FinalizeStockRequest is the JSON body for POST /api/v1/inventory/stock-take/finalize.
type FinalizeStockRequest struct {
	StockSheet string    `json:"stock_sheet"`
	PeriodDate time.Time `json:"period_date"`
	UserID     int64     `json:"-"` // extracted from JWT context, not from request body
}

// ─── GRV request types ────────────────────────────────────────────────────────

// CreateGrvRequest is the JSON body for POST /api/v1/inventory/grv.
type CreateGrvRequest struct {
	SupplierID      int64            `json:"supplier_id"`
	GrvDate         time.Time        `json:"grv_date"`
	ReferenceNumber string           `json:"reference_number"`
	Lines           []GrvLineRequest `json:"lines"`
}

type GrvLineRequest struct {
	MPartNo          string   `json:"item_id"`
	Qty              float64  `json:"qty"`                // qty in EACH units (not packs)
	PackCostOverride *float64 `json:"pack_cost_override"` // nil → use DMASTER.PACKCOST
}

// ─── Wastage request types ────────────────────────────────────────────────────

// RecordWastageRequest is the JSON body for POST /api/v1/inventory/wastage.
type RecordWastageRequest struct {
	MPartNo string  `json:"item_id"`
	Qty     float64 `json:"qty"`
	Notes   string  `json:"notes"`
}

// ─── Report types ────────────────────────────────────────────────────────────

// InventoryValueReport is returned by GET /api/v1/inventory/reports/value.
type InventoryValueReport struct {
	StockSheet string  `json:"stock_sheet"`
	TotalValue float64 `json:"total_value"`
}

// StockVarianceLine represents one item's variance for a reporting period.
type StockVarianceLine struct {
	MPartNo       string  `db:"MPARTNO"       json:"item_id"`
	Description   string  `db:"DESCRIPTION"   json:"description"`
	Bin           string  `db:"BIN"           json:"bin"`
	Opening       float64 `db:"opening"       json:"opening"`
	Received      float64 `db:"received"      json:"received"`
	Closing       float64 `db:"closing"       json:"closing"`
	Sales         float64 `db:"sales"         json:"sales"`
	Variance      float64 `db:"variance"         json:"variance"`
	UnitCost      float64 `db:"UNITCOST"         json:"unit_cost"`
	VarianceValue float64 `db:"-"                json:"variance_value"` // variance * unit_cost, computed in Go
}

// ─── ProductGroup maps to INVENTORY_GROUPS ─────────────────────────────────────

type ProductGroup struct {
	GroupID        int64  `db:"GROUP_ID"         json:"group_id"`
	BaseItemPartNo int64  `db:"BASE_ITEMPARTNO"  json:"base_item_id"`
	GroupName      string `db:"GROUP_NAME"       json:"group_name"`
	BaseUOM        string `db:"BASE_UOM"         json:"base_uom"`
	CreatedAt      string `db:"CREATED_AT"       json:"created_at"`
}

// ─── StockMovement maps to STOCK_MOVEMENTS (audit log) ───────────────────────

type StockMovement struct {
	MovementID        int64      `db:"MOVEMENT_ID"       json:"movement_id"`
	GroupID           int64      `db:"GROUP_ID"          json:"group_id"`
	VariantItemPartNo int64      `db:"VARIANT_ITEMPARTNO" json:"variant_item_id"`
	BaseItemPartNo    int64      `db:"BASE_ITEMPARTNO"   json:"base_item_id"`
	MovementType      string     `db:"MOVEMENT_TYPE"     json:"movement_type"` // GRV, SALE, ADJUSTMENT, etc.
	QtyVariant        float64    `db:"QTY_VARIANT"      json:"qty_variant"`
	QtyBase           float64    `db:"QTY_BASE"         json:"qty_base"`
	Reference         NullString `db:"REFERENCE"    json:"reference"`
	CreatedAt         string     `db:"CREATED_AT"       json:"created_at"`
}

// ─── ItemDetailWithGroupResponse combines item + group info ──────────────────

type ItemDetailWithGroupResponse struct {
	Item      InventoryItemDetail `json:"item"`
	Group     *ProductGroup       `json:"group,omitempty"`
	Variants  []InventoryItem     `json:"variants,omitempty"`
	Movements []StockMovement     `json:"movements,omitempty"`
}
