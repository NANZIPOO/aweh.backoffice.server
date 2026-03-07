package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
	"github.com/shopspring/decimal"
)

// PurchaseOrderRepository handles all DB operations for the PO module.
// Rules: tenant-isolated, no SQL JOINs, mandatory GEN_ID sequence on every insert.
type PurchaseOrderRepository struct {
	BaseRepository
}

func NewPurchaseOrderRepository(tm *TenantManager) *PurchaseOrderRepository {
	return &PurchaseOrderRepository{BaseRepository{TM: tm}}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nullStr extracts a string from sql.NullString, returning "" if not valid.
func nullStr(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

// writeError returns a formatted APIError for consistent JSON payloads in handlers.
func writeError(msg string) models.APIError { return models.APIError{Error: msg} }

// orderStatus maps EXPENSES.RECEIVED into the API status string.
func orderStatus(r models.FBBoolChar) string {
	if r.IsTrue() {
		return "posted"
	}
	return "draft"
}

// computeLine derives the full OrderLineDetail from a PItem DB row.
func computeLine(p *models.PItem) models.OrderLineDetail {
	taxRate := 0
	if p.TaxRate.Valid {
		taxRate = int(p.TaxRate.Int32)
	}
	extCost := p.QTY.Mul(p.PackCost)
	vatAmount := extCost.Mul(decimal.NewFromInt(int64(taxRate))).Div(decimal.NewFromInt(100))
	lineTotal := extCost.Add(vatAmount)

	itemNo := int64(0)
	if p.ItemNo.Valid {
		itemNo = p.ItemNo.Int64
	}

	return models.OrderLineDetail{
		ItemNo:       itemNo,
		MPartNo:      nullStr(p.MPartNo),
		Description:  nullStr(p.Description),
		Qty:          p.QTY.StringFixed(3),
		Pack:         p.Pack.StringFixed(3),
		PackCost:     p.PackCost.StringFixed(3),
		EachCost:     p.EachCost.StringFixed(3),
		TaxRate:      taxRate,
		Discount:     p.Discount.StringFixed(2),
		ExtCost:      extCost.StringFixed(2),
		VatAmount:    vatAmount.StringFixed(2),
		LineTotal:    lineTotal.StringFixed(2),
		CostGroup:    nullStr(p.CostGroup),
		CostCategory: nullStr(p.CostCategory),
		PackUnit:     nullStr(p.PackUnit),
		EachUnit:     nullStr(p.EachUnit),
	}
}

// fetchLines returns all PITEMS rows for an order and computes their line details.
// No JOIN — called separately after fetching the EXPENSES header.
func (r *PurchaseOrderRepository) fetchLines(ctx context.Context, db interface {
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}, orderNo int64) ([]models.OrderLineDetail, error) {
	const q = `
		SELECT ORDERNO, ITEMNO, MPARTNO, DESCRIPTION, QTY, PACK,
		       PACKCOST, EACHCOST, SUPPLIER, CATEGORY, COSTGROUP, COSTCATEGORY,
		       ORDERDATE, PURCHDATE, INVDATE, PURCHASES, POSTED, TAXRATE,
		       DISCOUNT, UNITS, EACHUNIT, PACKUNIT, PACKAGING
		FROM PITEMS WHERE ORDERNO = ? ORDER BY ITEMNO`

	var rows []models.PItem
	if err := db.SelectContext(ctx, &rows, q, orderNo); err != nil {
		return nil, fmt.Errorf("fetch lines: %w", err)
	}

	details := make([]models.OrderLineDetail, 0, len(rows))
	for i := range rows {
		details = append(details, computeLine(&rows[i]))
	}
	return details, nil
}

// expenseToDetail converts an EXPENSES row + pre-computed lines to an OrderDetail.
func expenseToDetail(e *models.Expense, lines []models.OrderLineDetail) models.OrderDetail {
	orderDate := ""
	if e.OrderDate.Valid {
		orderDate = e.OrderDate.Time.Format(time.RFC3339)
	}
	return models.OrderDetail{
		OrderNo:      e.OrderNo,
		SupplierNo:   nullStr(e.SupplierNo),
		SupplierName: nullStr(e.Supplier),
		OrderDate:    orderDate,
		Status:       orderStatus(e.Received),
		NettTotal:    e.NettTotal.StringFixed(2),
		VAT:          e.VAT.StringFixed(2),
		Discount:     e.Discount.StringFixed(2),
		Ullages:      e.Ullages.StringFixed(2),
		GrandTotal:   e.GrandTotal.StringFixed(2),
		Lines:        lines,
	}
}

// ---------------------------------------------------------------------------
// ListOrders — GET /purchase-orders?supplier_no={s}
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) ListOrders(ctx context.Context, supplierNo string) ([]models.OrderSummary, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("list orders: resolve db: %w", err)
	}

	var (
		rows  []models.Expense
		query string
		args  []interface{}
	)

	if supplierNo != "" {
		query = `SELECT ORDERNO, SUPPLIER, SUPPLIERNO, ORDERDATE, RECEIVED, NETTOTAL, VAT, DISCOUNT, ULLAGES, GRANDTOTAL, CATEGORY, PLACEDORDER, INVOICENO, PAYMETHOD, PAYREF, INVDATE, DISCOUNTDATE, PURCHDATE, DUEDATE FROM EXPENSES WHERE SUPPLIERNO = ? ORDER BY ORDERNO DESC`
		args = []interface{}{supplierNo}
	} else {
		query = `SELECT ORDERNO, SUPPLIER, SUPPLIERNO, ORDERDATE, RECEIVED, NETTOTAL, VAT, DISCOUNT, ULLAGES, GRANDTOTAL, CATEGORY, PLACEDORDER, INVOICENO, PAYMETHOD, PAYREF, INVDATE, DISCOUNTDATE, PURCHDATE, DUEDATE FROM EXPENSES ORDER BY ORDERNO DESC`
	}

	if err := db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("list orders: query: %w", err)
	}

	summaries := make([]models.OrderSummary, 0, len(rows))
	for _, e := range rows {
		od := ""
		if e.OrderDate.Valid {
			od = e.OrderDate.Time.Format(time.RFC3339)
		}
		summaries = append(summaries, models.OrderSummary{
			OrderNo:      e.OrderNo,
			SupplierNo:   nullStr(e.SupplierNo),
			SupplierName: nullStr(e.Supplier),
			OrderDate:    od,
			Status:       orderStatus(e.Received),
			GrandTotal:   e.GrandTotal.StringFixed(2),
		})
	}
	return summaries, nil
}

// ---------------------------------------------------------------------------
// GetOrder — GET /purchase-orders/{order_no}
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) GetOrder(ctx context.Context, orderNo int64) (*models.OrderDetail, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("get order: resolve db: %w", err)
	}

	const q = `SELECT ORDERNO, SUPPLIER, SUPPLIERNO, ORDERDATE, RECEIVED, NETTOTAL, VAT, DISCOUNT, ULLAGES, GRANDTOTAL, CATEGORY, PLACEDORDER, INVOICENO, PAYMETHOD, PAYREF, INVDATE, DISCOUNTDATE, PURCHDATE, DUEDATE FROM EXPENSES WHERE ORDERNO = ?`

	var e models.Expense
	if err := db.GetContext(ctx, &e, q, orderNo); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order %d not found", orderNo)
		}
		return nil, fmt.Errorf("get order: query: %w", err)
	}

	lines, err := r.fetchLines(ctx, db, orderNo)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}

	detail := expenseToDetail(&e, lines)
	return &detail, nil
}

// ---------------------------------------------------------------------------
// CreateOrder — POST /purchase-orders
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) CreateOrder(ctx context.Context, req *models.CreateOrderRequest) (*models.OrderDetail, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order: resolve db: %w", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("create order: begin tx: %w", err)
	}
	defer tx.Rollback()

	var newOrderNo int64
	if err := tx.QueryRowContext(ctx, models.NextIDQuery(models.GenOrders)).Scan(&newOrderNo); err != nil {
		return nil, fmt.Errorf("create order: gen_id(ORDERS_GEN): %w", err)
	}

	const insertSQL = `
		INSERT INTO EXPENSES (
			ORDERNO, SUPPLIER, SUPPLIERNO, ORDERDATE,
			RECEIVED, NETTOTAL, VAT, DISCOUNT, ULLAGES, GRANDTOTAL,
			CATEGORY, PLACEDORDER
		) VALUES (?, ?, ?, ?, 'F', 0, 0, 0, 0, 0, 'PURCHASES', 'N')`

	_, err = tx.ExecContext(ctx, insertSQL,
		newOrderNo, req.SupplierName, req.SupplierNo, time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("create order: exec insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create order: commit: %w", err)
	}

	return r.GetOrder(ctx, newOrderNo)
}

// ---------------------------------------------------------------------------
// DeleteOrder — DELETE /purchase-orders/{order_no}  (draft only)
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) DeleteOrder(ctx context.Context, orderNo int64) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("delete order: resolve db: %w", err)
	}

	// Guard: only delete drafts
	var received string
	if err := db.QueryRowContext(ctx, `SELECT RECEIVED FROM EXPENSES WHERE ORDERNO = ?`, orderNo).Scan(&received); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("order %d not found", orderNo)
		}
		return fmt.Errorf("delete order: read status: %w", err)
	}
	if received == "T" || received == "t" {
		return fmt.Errorf("order %d is already posted — cannot delete", orderNo)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete order: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `DELETE FROM PITEMS WHERE ORDERNO = ?`, orderNo); err != nil {
		return fmt.Errorf("delete order: delete lines: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM EXPENSES WHERE ORDERNO = ?`, orderNo); err != nil {
		return fmt.Errorf("delete order: delete header: %w", err)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// AddLineItem — POST /purchase-orders/{order_no}/lines
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) AddLineItem(ctx context.Context, orderNo int64, req *models.AddLineItemRequest) (*models.OrderLineDetail, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("add line: resolve db: %w", err)
	}

	// Guard: order must be draft.
	var received string
	if err := db.QueryRowContext(ctx, `SELECT RECEIVED FROM EXPENSES WHERE ORDERNO = ?`, orderNo).Scan(&received); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order %d not found", orderNo)
		}
		return nil, fmt.Errorf("add line: read order status: %w", err)
	}
	if received == "T" || received == "t" {
		return nil, fmt.Errorf("order %d is already posted", orderNo)
	}

	// Lookup item in DMASTER (no JOIN — separate query).
	const dmSQL = `
		SELECT MPARTNO, DESCRIPTION, SUPPLIERNO, PACKCOST, EACHCOST, UNITCOST,
		       PACK, UNITS, ONORDER, TAXRATE, COSTGROUP, COSTCATEGORY,
		       PACKUNIT, EACHUNIT, PACKAGING, PURCHASES, WPURCHASES
		FROM DMASTER WHERE MPARTNO = ?`

	var dm models.DmasterItem
	if err := db.GetContext(ctx, &dm, dmSQL, req.MPartNo); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("item %s not found in inventory", req.MPartNo)
		}
		return nil, fmt.Errorf("add line: lookup dmaster: %w", err)
	}

	// Guard: pack > 0 to avoid divide-by-zero when computing eachCost.
	if dm.Pack.IsZero() {
		return nil, fmt.Errorf("item %s has zero pack size — cannot calculate unit cost", req.MPartNo)
	}

	// Parse qty; if 0 or blank, apply legacy rule: default to 1 when ONORDER==0.
	qty, parseErr := decimal.NewFromString(req.Qty)
	if parseErr != nil || qty.IsZero() {
		if dm.OnOrder.IsZero() {
			qty = decimal.NewFromInt(1)
		} else {
			qty = dm.OnOrder
		}
	}

	// Apply VAT stripping if caller says price is VAT-inclusive.
	packCost := dm.PackCost
	if req.VatInclusive && dm.TaxRate > 0 {
		divisor := decimal.NewFromInt(int64(dm.TaxRate)).Div(decimal.NewFromInt(100)).Add(decimal.NewFromInt(1))
		packCost = packCost.Div(divisor).RoundBank(4)
	}
	eachCost := packCost.Div(dm.Pack).RoundBank(4)

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("add line: begin tx: %w", err)
	}
	defer tx.Rollback()

	var newItemNo int64
	if err := tx.QueryRowContext(ctx, models.NextIDQuery(models.GenOrderItems)).Scan(&newItemNo); err != nil {
		return nil, fmt.Errorf("add line: gen_id(LINE_ORDERNO_GEN): %w", err)
	}

	const insertSQL = `
		INSERT INTO PITEMS (
			ORDERNO, ITEMNO, MPARTNO, DESCRIPTION, QTY, PACK,
			PACKCOST, EACHCOST, SUPPLIER, CATEGORY, COSTGROUP, COSTCATEGORY,
			ORDERDATE, TAXRATE, DISCOUNT, UNITS, EACHUNIT, PACKUNIT, PACKAGING,
			POSTED
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, 0, ?, ?, ?, ?,
			'F'
		)`

	_, err = tx.ExecContext(ctx, insertSQL,
		orderNo, newItemNo, dm.MPartNo,
		nullStr(dm.Description),
		qty, dm.Pack,
		packCost, eachCost,
		nullStr(dm.SupplierNo), // SUPPLIER column holds supplier no in PITEMS
		"PURCHASES",
		nullStr(dm.CostGroup), nullStr(dm.CostCategory),
		time.Now(),
		dm.TaxRate,
		dm.Units,
		nullStr(dm.EachUnit), nullStr(dm.PackUnit), nullStr(dm.Packaging),
	)
	if err != nil {
		return nil, fmt.Errorf("add line: exec insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("add line: commit: %w", err)
	}

	// Build the response from what we know — no second SELECT needed.
	p := models.PItem{
		ItemNo:       sql.NullInt64{Int64: newItemNo, Valid: true},
		MPartNo:      sql.NullString{String: dm.MPartNo, Valid: true},
		Description:  dm.Description,
		QTY:          qty,
		Pack:         dm.Pack,
		PackCost:     packCost,
		EachCost:     eachCost,
		CostGroup:    dm.CostGroup,
		CostCategory: dm.CostCategory,
		PackUnit:     dm.PackUnit,
		EachUnit:     dm.EachUnit,
		TaxRate:      sql.NullInt32{Int32: int32(dm.TaxRate), Valid: true},
		Discount:     decimal.Zero,
		Units:        decimal.NewFromInt(dm.Units),
	}
	ld := computeLine(&p)
	return &ld, nil
}

// ---------------------------------------------------------------------------
// UpdateLineItem — PUT /purchase-orders/{order_no}/lines/{item_no}
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) UpdateLineItem(ctx context.Context, orderNo, itemNo int64, req *models.UpdateLineItemRequest) (*models.OrderLineDetail, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("update line: resolve db: %w", err)
	}

	// Guard: order must be draft.
	var received string
	if err := db.QueryRowContext(ctx, `SELECT RECEIVED FROM EXPENSES WHERE ORDERNO = ?`, orderNo).Scan(&received); err != nil {
		return nil, fmt.Errorf("update line: read order status: %w", err)
	}
	if received == "T" || received == "t" {
		return nil, fmt.Errorf("order %d is already posted", orderNo)
	}

	// Read the existing line to get PACK (unchanged), taxRate, costGroup etc.
	const selectSQL = `
		SELECT ORDERNO, ITEMNO, MPARTNO, DESCRIPTION, QTY, PACK,
		       PACKCOST, EACHCOST, SUPPLIER, CATEGORY, COSTGROUP, COSTCATEGORY,
		       ORDERDATE, PURCHDATE, INVDATE, PURCHASES, POSTED, TAXRATE,
		       DISCOUNT, UNITS, EACHUNIT, PACKUNIT, PACKAGING
		FROM PITEMS WHERE ORDERNO = ? AND ITEMNO = ?`

	var existing models.PItem
	if err := db.GetContext(ctx, &existing, selectSQL, orderNo, itemNo); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("line %d not found on order %d", itemNo, orderNo)
		}
		return nil, fmt.Errorf("update line: read existing: %w", err)
	}

	// Parse the new values.
	newQty, err := decimal.NewFromString(req.Qty)
	if err != nil {
		return nil, fmt.Errorf("update line: invalid qty %q: %w", req.Qty, err)
	}
	newPackCost, err := decimal.NewFromString(req.PackCost)
	if err != nil {
		return nil, fmt.Errorf("update line: invalid pack_cost %q: %w", req.PackCost, err)
	}

	newEachCost := decimal.Zero
	if !existing.Pack.IsZero() {
		newEachCost = newPackCost.Div(existing.Pack).RoundBank(4)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("update line: begin tx: %w", err)
	}
	defer tx.Rollback()

	const updateSQL = `UPDATE PITEMS SET QTY = ?, PACKCOST = ?, EACHCOST = ? WHERE ORDERNO = ? AND ITEMNO = ?`
	if _, err := tx.ExecContext(ctx, updateSQL, newQty, newPackCost, newEachCost, orderNo, itemNo); err != nil {
		return nil, fmt.Errorf("update line: exec update: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update line: commit: %w", err)
	}

	existing.QTY = newQty
	existing.PackCost = newPackCost
	existing.EachCost = newEachCost
	ld := computeLine(&existing)
	return &ld, nil
}

// ---------------------------------------------------------------------------
// DeleteLineItem — DELETE /purchase-orders/{order_no}/lines/{item_no}
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) DeleteLineItem(ctx context.Context, orderNo, itemNo int64) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("delete line: resolve db: %w", err)
	}

	var received string
	if err := db.QueryRowContext(ctx, `SELECT RECEIVED FROM EXPENSES WHERE ORDERNO = ?`, orderNo).Scan(&received); err != nil {
		return fmt.Errorf("delete line: read order status: %w", err)
	}
	if received == "T" || received == "t" {
		return fmt.Errorf("order %d is already posted", orderNo)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete line: begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM PITEMS WHERE ORDERNO = ? AND ITEMNO = ?`, orderNo, itemNo)
	if err != nil {
		return fmt.Errorf("delete line: exec delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("line %d not found on order %d", itemNo, orderNo)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// GetOrderTotals — GET /purchase-orders/{order_no}/totals
// Recalculates from PITEMS, updates EXPENSES, returns computed totals.
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) GetOrderTotals(ctx context.Context, orderNo int64) (*models.OrderTotals, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("get totals: resolve db: %w", err)
	}

	// Read existing discount / ullages from the header.
	var discount, ullages decimal.Decimal
	if err := db.QueryRowContext(ctx,
		`SELECT DISCOUNT, ULLAGES FROM EXPENSES WHERE ORDERNO = ?`, orderNo,
	).Scan(&discount, &ullages); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order %d not found", orderNo)
		}
		return nil, fmt.Errorf("get totals: read header: %w", err)
	}

	nett, vat, grandTotal := r.computeTotals(ctx, db, orderNo, discount, ullages)

	// Persist back to EXPENSES.
	if _, err := db.ExecContext(ctx,
		`UPDATE EXPENSES SET NETTOTAL=?, VAT=?, GRANDTOTAL=? WHERE ORDERNO=?`,
		nett, vat, grandTotal, orderNo,
	); err != nil {
		return nil, fmt.Errorf("get totals: persist: %w", err)
	}

	return &models.OrderTotals{
		NettTotal:  nett.StringFixed(2),
		VAT:        vat.StringFixed(2),
		GrandTotal: grandTotal.StringFixed(2),
		Difference: "0.00",
	}, nil
}

// computeTotals loops PITEMS to derive nett / vat / grand for an order.
// discount and ullages come from EXPENSES and are folded into grandTotal.
func (r *PurchaseOrderRepository) computeTotals(ctx context.Context, db interface {
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}, orderNo int64, discount, ullages decimal.Decimal) (nett, vat, grandTotal decimal.Decimal) {
	const q = `SELECT QTY, PACKCOST, TAXRATE FROM PITEMS WHERE ORDERNO = ?`

	type lineRow struct {
		QTY      decimal.Decimal `db:"QTY"`
		PackCost decimal.Decimal `db:"PACKCOST"`
		TaxRate  sql.NullInt32   `db:"TAXRATE"`
	}
	var rows []lineRow
	_ = db.SelectContext(ctx, &rows, q, orderNo) // best-effort; returns zeros on error

	for _, row := range rows {
		ext := row.QTY.Mul(row.PackCost)
		nett = nett.Add(ext)
		if row.TaxRate.Valid && row.TaxRate.Int32 > 0 {
			v := ext.Mul(decimal.NewFromInt(int64(row.TaxRate.Int32))).Div(decimal.NewFromInt(100))
			vat = vat.Add(v)
		}
	}
	grandTotal = nett.Add(vat).Sub(discount).Add(ullages)
	return
}

// ---------------------------------------------------------------------------
// CaptureInvoice — POST /purchase-orders/{order_no}/capture-invoice
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) CaptureInvoice(ctx context.Context, orderNo int64, req *models.CaptureInvoiceRequest) (*models.OrderTotals, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("capture invoice: resolve db: %w", err)
	}

	invoicedAmt, err := decimal.NewFromString(req.InvoicedAmount)
	if err != nil {
		return nil, fmt.Errorf("capture invoice: invalid invoiced_amount: %w", err)
	}
	discount, err := decimal.NewFromString(req.Discount)
	if err != nil {
		return nil, fmt.Errorf("capture invoice: invalid discount: %w", err)
	}
	ullages, err := decimal.NewFromString(req.Ullages)
	if err != nil {
		return nil, fmt.Errorf("capture invoice: invalid ullages: %w", err)
	}

	// Guard: draft only.
	var received string
	if err := db.QueryRowContext(ctx, `SELECT RECEIVED FROM EXPENSES WHERE ORDERNO = ?`, orderNo).Scan(&received); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order %d not found", orderNo)
		}
		return nil, fmt.Errorf("capture invoice: read status: %w", err)
	}
	if received == "T" || received == "t" {
		return nil, fmt.Errorf("order %d is already posted", orderNo)
	}

	// Persist discount/ullages.
	if _, err := db.ExecContext(ctx,
		`UPDATE EXPENSES SET DISCOUNT=?, ULLAGES=? WHERE ORDERNO=?`,
		discount, ullages, orderNo,
	); err != nil {
		return nil, fmt.Errorf("capture invoice: update header: %w", err)
	}

	nett, vat, grandTotal := r.computeTotals(ctx, db, orderNo, discount, ullages)

	// Persist computed totals.
	if _, err := db.ExecContext(ctx,
		`UPDATE EXPENSES SET NETTOTAL=?, VAT=?, GRANDTOTAL=? WHERE ORDERNO=?`,
		nett, vat, grandTotal, orderNo,
	); err != nil {
		return nil, fmt.Errorf("capture invoice: persist totals: %w", err)
	}

	diff := invoicedAmt.Sub(grandTotal)
	absDiff := diff.Abs()

	// Legacy tolerance rule: >1.00 is a blocking error, ≤1.00 is a warning.
	if absDiff.GreaterThan(decimal.NewFromInt(1)) {
		return nil, fmt.Errorf("invoice total (%s) differs from computed grand total (%s) by %s — exceeds R1.00 tolerance",
			invoicedAmt.StringFixed(2), grandTotal.StringFixed(2), absDiff.StringFixed(2))
	}

	totals := &models.OrderTotals{
		NettTotal:  nett.StringFixed(2),
		VAT:        vat.StringFixed(2),
		GrandTotal: grandTotal.StringFixed(2),
		Difference: diff.StringFixed(2),
	}

	if absDiff.GreaterThan(decimal.Zero) {
		msg := fmt.Sprintf("Invoice total differs from computed total by R%s. Rounding may be required.", absDiff.StringFixed(2))
		totals.Warning = &msg
	}

	return totals, nil
}

// ---------------------------------------------------------------------------
// IsDuplicateInvoice — used internally by PostInvoice
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) IsDuplicateInvoice(ctx context.Context, db interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}, supplierNo, invoiceNo string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM CREDITORSLEDGER WHERE SUPPLIERNO = ? AND INVOICENO = ?`,
		supplierNo, invoiceNo,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("duplicate check: %w", err)
	}
	return count > 0, nil
}

// ---------------------------------------------------------------------------
// PostInvoice — POST /purchase-orders/{order_no}/post
// 12-step atomic transaction. Any failure rolls back everything.
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) PostInvoice(ctx context.Context, orderNo int64, req *models.PostInvoiceRequest) (*models.PostInvoiceResponse, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("post invoice: resolve db: %w", err)
	}

	// --- Pre-tx validation (read-only guards) ---

	// Step 1: Load the order header.
	const expSQL = `SELECT ORDERNO, SUPPLIER, SUPPLIERNO, ORDERDATE, RECEIVED, NETTOTAL, VAT, DISCOUNT, ULLAGES, GRANDTOTAL, CATEGORY, PLACEDORDER, INVOICENO, PAYMETHOD, PAYREF, INVDATE, DISCOUNTDATE, PURCHDATE, DUEDATE FROM EXPENSES WHERE ORDERNO = ?`
	var e models.Expense
	if err := db.GetContext(ctx, &e, expSQL, orderNo); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order %d not found", orderNo)
		}
		return nil, fmt.Errorf("post invoice: load order: %w", err)
	}
	if e.Received.IsTrue() {
		return nil, fmt.Errorf("order %d is already posted", orderNo)
	}

	// Step 2: Duplicate invoice guard.
	dup, err := r.IsDuplicateInvoice(ctx, db, nullStr(e.SupplierNo), req.InvoiceNumber)
	if err != nil {
		return nil, fmt.Errorf("post invoice: %w", err)
	}
	if dup {
		return nil, fmt.Errorf("invoice %s already exists for supplier %s", req.InvoiceNumber, nullStr(e.SupplierNo))
	}

	// Step 3: CHEQUE requires a reference.
	if req.PayMethod == "CHEQUE" && req.PayReference == "" {
		return nil, fmt.Errorf("pay reference required for CHEQUE payment method")
	}

	// Parse dates.
	invDate, err := time.Parse(time.RFC3339, req.InvoiceDate)
	if err != nil {
		return nil, fmt.Errorf("post invoice: invalid invoice_date: %w", err)
	}
	purchDate, err := time.Parse(time.RFC3339, req.ReceivedDate)
	if err != nil {
		return nil, fmt.Errorf("post invoice: invalid received_date: %w", err)
	}
	var dueDate *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		t, err := time.Parse(time.RFC3339, *req.DueDate)
		if err != nil {
			return nil, fmt.Errorf("post invoice: invalid due_date: %w", err)
		}
		dueDate = &t
	}

	now := time.Now()

	// --- Begin the atomic transaction ---
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("post invoice: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Step 4: Update EXPENSES to mark as posted.
	const updateExpSQL = `
		UPDATE EXPENSES SET
			INVOICENO = ?, PAYMETHOD = ?, PAYREF = ?,
			PURCHDATE = ?, INVDATE = ?,
			RECEIVED = 'T', CATEGORY = 'PURCHASES',
			DUEDATE = ?
		WHERE ORDERNO = ?`

	if _, err := tx.ExecContext(ctx, updateExpSQL,
		req.InvoiceNumber, req.PayMethod, req.PayReference,
		purchDate, invDate,
		dueDate,
		orderNo,
	); err != nil {
		return nil, fmt.Errorf("post invoice: update expenses: %w", err)
	}

	// Step 5: INSERT invoice row into CREDITORSLEDGER (shares ORDERNO with EXPENSES).
	const insCredSQL = `
		INSERT INTO CREDITORSLEDGER (
			ORDERNO, SUPPLIER, SUPPLIERNO, INVOICENO, PAYMETHOD, PAYREF,
			INVDATE, DISCOUNTDATE, PURCHDATE, ORDERDATE, DUEDATE,
			RECEIVED, NETTOTAL, VAT, DISCOUNT, ULLAGES, GRANDTOTAL,
			CATEGORY, PLACEDORDER
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, NULL, ?, ?, ?,
			'T', ?, ?, ?, ?, ?,
			'PURCHASES', 'N'
		)`

	if _, err := tx.ExecContext(ctx, insCredSQL,
		e.OrderNo, nullStr(e.Supplier), nullStr(e.SupplierNo),
		req.InvoiceNumber, req.PayMethod, req.PayReference,
		invDate, purchDate,
		e.OrderDate, // re-use original order date
		dueDate,
		e.NettTotal, e.VAT, e.Discount, e.Ullages, e.GrandTotal,
	); err != nil {
		return nil, fmt.Errorf("post invoice: insert creditorsledger: %w", err)
	}

	// Step 6: INSERT payment credit row (only when paymethod is not NOT PAID).
	if req.PayMethod != "NOT PAID" {
		var paymentOrderNo int64
		if err := tx.QueryRowContext(ctx, models.NextIDQuery(models.GenOrders)).Scan(&paymentOrderNo); err != nil {
			return nil, fmt.Errorf("post invoice: gen_id for payment row: %w", err)
		}

		negativeTotal := e.GrandTotal.Neg()
		const insPaySQL = `
			INSERT INTO CREDITORSLEDGER (
				ORDERNO, SUPPLIER, SUPPLIERNO, INVOICENO, PAYMETHOD, PAYREF,
				INVDATE, PURCHDATE, ORDERDATE,
				RECEIVED, NETTOTAL, VAT, DISCOUNT, ULLAGES, GRANDTOTAL,
				CATEGORY, PLACEDORDER
			) VALUES (
				?, ?, ?, 'Payment Thank You', ?, ?,
				?, ?, ?,
				'T', 0, 0, 0, 0, ?,
				'PURCHASES', 'N'
			)`

		if _, err := tx.ExecContext(ctx, insPaySQL,
			paymentOrderNo, nullStr(e.Supplier), nullStr(e.SupplierNo),
			req.PayMethod, req.PayReference,
			invDate, purchDate, e.OrderDate,
			negativeTotal,
		); err != nil {
			return nil, fmt.Errorf("post invoice: insert payment credit row: %w", err)
		}
	}

	// Step 7: Update DMASTER — reset ONORDER to 0 and add PURCHASES for each item.
	const updPurchSQL = `
		UPDATE DMASTER SET
			ONORDER = 0,
			PURCHASES = PURCHASES + (
				SELECT COALESCE(SUM(QTY * PACK), 0) FROM PITEMS
				WHERE ORDERNO = ? AND MPARTNO = DMASTER.MPARTNO
			)
		WHERE MPARTNO IN (SELECT MPARTNO FROM PITEMS WHERE ORDERNO = ?)`

	if _, err := tx.ExecContext(ctx, updPurchSQL, orderNo, orderNo); err != nil {
		return nil, fmt.Errorf("post invoice: update dmaster purchases: %w", err)
	}

	// Step 8: Update DMASTER — add to WPURCHASES (weekly/period counter).
	const updWPurchSQL = `
		UPDATE DMASTER SET
			WPURCHASES = WPURCHASES + (
				SELECT COALESCE(SUM(QTY * PACK), 0) FROM PITEMS
				WHERE ORDERNO = ? AND MPARTNO = DMASTER.MPARTNO
			)
		WHERE MPARTNO IN (SELECT MPARTNO FROM PITEMS WHERE ORDERNO = ?)`

	if _, err := tx.ExecContext(ctx, updWPurchSQL, orderNo, orderNo); err != nil {
		return nil, fmt.Errorf("post invoice: update dmaster wpurchases: %w", err)
	}

	// Step 9: Stamp PITEMS as posted with invoice dates.
	if _, err := tx.ExecContext(ctx,
		`UPDATE PITEMS SET POSTED = 'T', PURCHDATE = ?, INVDATE = ? WHERE ORDERNO = ?`,
		purchDate, invDate, orderNo,
	); err != nil {
		return nil, fmt.Errorf("post invoice: stamp pitems posted: %w", err)
	}

	// Step 10: Archive to PITEMSHIS.
	// PITEMS.PACKCOST is NUMERIC(18,3); PITEMSHIS.PACKCOST is NUMERIC(18,2) — explicit CAST required.
	const insPItemsHisSQL = `
		INSERT INTO PITEMSHIS (
			ORDERNO, ITEMNO, MPARTNO, DESCRIPTION, QTY, PACK,
			PACKCOST, EACHCOST, SUPPLIER, CATEGORY, COSTGROUP, COSTCATEGORY,
			ORDERDATE, PURCHDATE, INVDATE, PURCHASES, POSTED, TAXRATE,
			DISCOUNT, UNITS, EACHUNIT, PACKUNIT, PACKAGING
		)
		SELECT
			ORDERNO, ITEMNO, MPARTNO, DESCRIPTION, QTY, PACK,
			CAST(PACKCOST AS NUMERIC(18,2)), CAST(EACHCOST AS NUMERIC(18,2)),
			SUPPLIER, CATEGORY, COSTGROUP, COSTCATEGORY,
			ORDERDATE, PURCHDATE, INVDATE, PURCHASES, POSTED, TAXRATE,
			DISCOUNT, UNITS, EACHUNIT, PACKUNIT, PACKAGING
		FROM PITEMS WHERE ORDERNO = ?`

	if _, err := tx.ExecContext(ctx, insPItemsHisSQL, orderNo); err != nil {
		return nil, fmt.Errorf("post invoice: archive to pitemshis: %w", err)
	}

	// Step 11: Delete working lines.
	if _, err := tx.ExecContext(ctx, `DELETE FROM PITEMS WHERE ORDERNO = ?`, orderNo); err != nil {
		return nil, fmt.Errorf("post invoice: delete pitems: %w", err)
	}

	// Step 12: Reset ONORDER for all remaining items of this supplier.
	if _, err := tx.ExecContext(ctx,
		`UPDATE DMASTER SET ONORDER = 0 WHERE SUPPLIERNO = ?`,
		nullStr(e.SupplierNo),
	); err != nil {
		return nil, fmt.Errorf("post invoice: reset supplier onorder: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("post invoice: commit: %w", err)
	}

	return &models.PostInvoiceResponse{
		OrderNo:       orderNo,
		Status:        "posted",
		InvoiceNumber: req.InvoiceNumber,
		GrandTotal:    e.GrandTotal.StringFixed(2),
		PostedAt:      now.Format(time.RFC3339),
	}, nil
}

// ---------------------------------------------------------------------------
// UpdateInventoryCosts — PUT /purchase-orders/{order_no}/update-costs
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) UpdateInventoryCosts(ctx context.Context, req *models.UpdateCostsRequest) (*models.UpdateCostsResponse, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("update costs: resolve db: %w", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("update costs: begin tx: %w", err)
	}
	defer tx.Rollback()

	updated, skipped := 0, 0

	for _, line := range req.Lines {
		pc, err1 := decimal.NewFromString(line.PackCost)
		ec, err2 := decimal.NewFromString(line.EachCost)
		uc, err3 := decimal.NewFromString(line.UnitCost)

		if err1 != nil || err2 != nil || err3 != nil {
			skipped++
			continue
		}

		res, err := tx.ExecContext(ctx,
			`UPDATE DMASTER SET PACKCOST=?, EACHCOST=?, UNITCOST=? WHERE MPARTNO=?`,
			pc, ec, uc, line.MPartNo,
		)
		if err != nil {
			skipped++
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			skipped++
			continue
		}

		// Mirror cost price to SALESMENU.
		if _, err := tx.ExecContext(ctx,
			`UPDATE SALESMENU SET COSTPRICE=? WHERE MPARTNO=?`,
			ec, line.MPartNo,
		); err != nil {
			// Non-fatal: SALESMENU may not have this item.
			// No rollback — DMASTER update is still committed.
		}

		updated++
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update costs: commit: %w", err)
	}

	return &models.UpdateCostsResponse{
		UpdatedCount: updated,
		SkippedCount: skipped,
	}, nil
}

// ---------------------------------------------------------------------------
// ListSuppliers — GET /suppliers
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) ListSuppliers(ctx context.Context) ([]models.SupplierResponse, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("list suppliers: resolve db: %w", err)
	}

	const q = `
		SELECT ITEMNO, SUPPLIERNO, SUPPLIER, PHONE, EMAIL, CONTACT, ADDRESS1
		FROM SUPPLIERS ORDER BY SUPPLIERNO`

	var rows []models.Supplier
	if err := db.SelectContext(ctx, &rows, q); err != nil {
		return nil, fmt.Errorf("list suppliers: query: %w", err)
	}

	out := make([]models.SupplierResponse, 0, len(rows))
	for _, s := range rows {
		out = append(out, models.SupplierResponse{
			ItemNo:       s.ItemNo,
			SupplierNo:   s.SupplierNo,
			SupplierName: nullStr(s.Supplier),
			Phone:        nullStr(s.Phone),
			Email:        nullStr(s.Email),
			Contact:      nullStr(s.Contact),
			Address:      nullStr(s.Address1),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// GetSupplierItems — GET /suppliers/{supplier_no}/items
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) GetSupplierItems(ctx context.Context, supplierNo string) ([]models.SupplierItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("supplier items: resolve db: %w", err)
	}

	const q = `
		SELECT MPARTNO, DESCRIPTION, SUPPLIERNO, PACKCOST, EACHCOST, UNITCOST,
		       PACK, UNITS, ONORDER, TAXRATE, COSTGROUP, COSTCATEGORY,
		       PACKUNIT, EACHUNIT, PACKAGING, PURCHASES, WPURCHASES
		FROM DMASTER WHERE SUPPLIERNO = ? ORDER BY MPARTNO`

	var rows []models.DmasterItem
	if err := db.SelectContext(ctx, &rows, q, supplierNo); err != nil {
		return nil, fmt.Errorf("supplier items: query: %w", err)
	}

	out := make([]models.SupplierItem, 0, len(rows))
	for _, d := range rows {
		out = append(out, dmasterToSupplierItem(&d))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// SearchInventoryItem — GET /inventory/search?mpart_no={p}
// ---------------------------------------------------------------------------

func (r *PurchaseOrderRepository) SearchInventoryItem(ctx context.Context, mpartNo string) (*models.SupplierItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("search item: resolve db: %w", err)
	}

	const q = `
		SELECT MPARTNO, DESCRIPTION, SUPPLIERNO, PACKCOST, EACHCOST, UNITCOST,
		       PACK, UNITS, ONORDER, TAXRATE, COSTGROUP, COSTCATEGORY,
		       PACKUNIT, EACHUNIT, PACKAGING, PURCHASES, WPURCHASES
		FROM DMASTER WHERE MPARTNO = ?`

	var d models.DmasterItem
	if err := db.GetContext(ctx, &d, q, mpartNo); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("item %s not found in inventory", mpartNo)
		}
		return nil, fmt.Errorf("search item: query: %w", err)
	}

	si := dmasterToSupplierItem(&d)
	return &si, nil
}

// ---------------------------------------------------------------------------
// Private conversion helper
// ---------------------------------------------------------------------------

func dmasterToSupplierItem(d *models.DmasterItem) models.SupplierItem {
	return models.SupplierItem{
		MPartNo:      d.MPartNo,
		Description:  nullStr(d.Description),
		PackCost:     d.PackCost.StringFixed(3),
		EachCost:     d.EachCost.StringFixed(3),
		Pack:         d.Pack.StringFixed(3),
		Units:        d.Units,
		OnOrder:      d.OnOrder.StringFixed(3),
		TaxRate:      d.TaxRate,
		CostGroup:    nullStr(d.CostGroup),
		CostCategory: nullStr(d.CostCategory),
		PackUnit:     nullStr(d.PackUnit),
		EachUnit:     nullStr(d.EachUnit),
		Packaging:    nullStr(d.Packaging),
	}
}
