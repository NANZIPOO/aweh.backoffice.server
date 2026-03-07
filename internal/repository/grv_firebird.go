package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
)

// ─── GrvFirebird ──────────────────────────────────────────────────────────────

// GrvFirebird implements GrvRepository using CREDITORSLEDGER + ORDERITEMS.
type GrvFirebird struct {
	BaseRepository
}

func NewGrvRepository(tm *TenantManager) *GrvFirebird {
	return &GrvFirebird{BaseRepository{TM: tm}}
}

// ─── CreateGrv ────────────────────────────────────────────────────────────────
//
// Mandatory DB Sequence (Project Constitution §4):
//  1. db.BeginTxx(ctx)
//  2. SELECT GEN_ID(ORDERS_GEN, 1) FROM RDB$DATABASE  ← fetch ORDERNO
//  3. Compute totals + per-line VAT in Go
//  4. INSERT INTO CREDITORSLEDGER with fetched ORDERNO
//  5. For each line: INSERT INTO ORDERITEMS
//  6. For each line: UPDATE DMASTER SET PURCHASES=PURCHASES+qty, {day}REC={day}REC+qty
//  7. UPDATE CREDITORSLEDGER SET RECEIVED='Y'
//  8. tx.Commit()
//
// supplier_name is resolved AFTER commit via a SEPARATE single-table SELECT.
// NO JOIN is used in the write transaction (Project Constitution §4 + Appendix A §7).

func (r *GrvFirebird) CreateGrv(ctx context.Context, req models.CreateGrvRequest) (*models.GrvHeader, error) {
	if len(req.Lines) == 0 {
		return nil, fmt.Errorf("ERR_VALIDATION: GRV must have at least one line")
	}

	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	// Determine day-of-week REC column from GrvDate (computed before tx)
	recCol := dayRecColumn(req.GrvDate.Weekday())

	// ── Step 1: Begin transaction ────────────────────────────────────────────
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateGrv BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// ── Step 2: Fetch next ORDERNO inside tx ─────────────────────────────────
	var orderNo int64
	if err := tx.GetContext(ctx, &orderNo, "SELECT GEN_ID(ORDERS_GEN, 1) FROM RDB$DATABASE"); err != nil {
		return nil, fmt.Errorf("CreateGrv GEN_ID: %w", err)
	}

	// ── Step 3: Compute totals in Go ──────────────────────────────────────────
	type lineData struct {
		mPartNo     string
		description string
		qty         float64
		pack        float64
		packCost    float64
		eachCost    float64
		taxRate     float64
		lineNett    float64
		lineVAT     float64
	}

	lines := make([]lineData, 0, len(req.Lines))
	var nettTotal, vatTotal float64

	for _, reqLine := range req.Lines {
		// Fetch DMASTER record for this item — single-table SELECT, no JOIN
		var item struct {
			Description string  `db:"DESCRIPTION"`
			Pack        float64 `db:"PACK"`
			PackCost    float64 `db:"PACKCOST"`
			TaxRate     float64 `db:"TAXRATE"`
		}
		if err := db.GetContext(ctx, &item,
			"SELECT DESCRIPTION, PACK, PACKCOST, TAXRATE FROM DMASTER WHERE MPARTNO = ?",
			reqLine.MPartNo); err != nil {
			return nil, fmt.Errorf("CreateGrv: item '%s' not found in DMASTER: %w", reqLine.MPartNo, err)
		}

		pack := item.Pack
		if pack <= 0 {
			pack = 1
		}
		packCost := item.PackCost
		if reqLine.PackCostOverride != nil {
			packCost = *reqLine.PackCostOverride
		}
		eachCost := packCost / pack
		lineNett := eachCost * reqLine.Qty
		lineVAT := lineNett * (item.TaxRate / 100)

		nettTotal += lineNett
		vatTotal += lineVAT

		lines = append(lines, lineData{
			mPartNo:     reqLine.MPartNo,
			description: item.Description,
			qty:         reqLine.Qty,
			pack:        pack,
			packCost:    packCost,
			eachCost:    eachCost,
			taxRate:     item.TaxRate,
			lineNett:    lineNett,
			lineVAT:     lineVAT,
		})
	}

	grandTotal := nettTotal + vatTotal

	// ── Step 4: INSERT CREDITORSLEDGER header ─────────────────────────────────
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO CREDITORSLEDGER (ORDERNO, SUPPLIERNO, INVDATE, NETTOTAL, VAT, GRANDTOTAL, RECEIVED)
		VALUES (?, ?, ?, ?, ?, ?, 'N')`,
		orderNo, req.SupplierID, req.GrvDate, round2dp(nettTotal), round2dp(vatTotal), round2dp(grandTotal),
	); err != nil {
		return nil, fmt.Errorf("CreateGrv INSERT CREDITORSLEDGER: %w", err)
	}

	// ── Step 5: INSERT ORDERITEMS per line ────────────────────────────────────
	// Note: ORDERITEMS may have a Firebird BEFORE INSERT trigger that assigns ITEMNO.
	// If not, add GEN_ID call here for whatever generator covers ORDERITEMS.ITEMNO.
	for _, line := range lines {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ORDERITEMS (ORDERNO, MPARTNO, DESCRIPTION, QTY, PACK, PACKCOST, EACHCOST, TAXRATE, POSTED, ORDERDATE, SUPPLIER)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'F', ?, ?)`,
			orderNo, line.mPartNo, line.description, line.qty, line.pack,
			round2dp(line.packCost), round2dp(line.eachCost), line.taxRate,
			req.GrvDate, fmt.Sprintf("%d", req.SupplierID),
		); err != nil {
			return nil, fmt.Errorf("CreateGrv INSERT ORDERITEMS for %s: %w", line.mPartNo, err)
		}
	}

	// ── Step 6: UPDATE DMASTER PURCHASES + {day}REC per line ─────────────────
	// Decision D-04: additive patching only — no trigger replacement.
	// recCol is validated (from dayColumnMap), safe to interpolate.
	updateQ := fmt.Sprintf(
		"UPDATE DMASTER SET PURCHASES = PURCHASES + ?, %s = %s + ? WHERE MPARTNO = ?",
		recCol, recCol)

	for _, line := range lines {
		if _, err := tx.ExecContext(ctx, updateQ, line.qty, line.qty, line.mPartNo); err != nil {
			return nil, fmt.Errorf("CreateGrv UPDATE DMASTER for %s: %w", line.mPartNo, err)
		}
	}

	// ── Step 7: Mark header as received ──────────────────────────────────────
	if _, err := tx.ExecContext(ctx, "UPDATE CREDITORSLEDGER SET RECEIVED = 'Y' WHERE ORDERNO = ?", orderNo); err != nil {
		return nil, fmt.Errorf("CreateGrv UPDATE RECEIVED: %w", err)
	}

	// ── Step 8: Commit ────────────────────────────────────────────────────────
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("CreateGrv Commit: %w", err)
	}

	// ── Post-commit: return header (no JOIN used) ─────────────────────────────
	header := &models.GrvHeader{
		OrderNo:    orderNo,
		SupplierNo: req.SupplierID,
		InvDate:    req.GrvDate,
		NettTotal:  round2dp(nettTotal),
		VAT:        round2dp(vatTotal),
		GrandTotal: round2dp(grandTotal),
		Received:   "Y",
	}
	return header, nil
}

// resolveSupplierName performs a separate single-table SELECT to get the supplier name
// for use in GrvResponse. Called by the handler AFTER the repository returns.
// This is NOT inside the write transaction — per Project Constitution §4 + Appendix A §7.
func ResolveSupplierName(ctx context.Context, tm *TenantManager, supplierID int64) string {
	db, err := tm.GetDB(ctx)
	if err != nil {
		return fmt.Sprintf("%d", supplierID)
	}
	var name string
	if err := db.GetContext(ctx, &name, "SELECT SUPPLIER FROM CREDITORS WHERE SUPPLIERNO = ?", supplierID); err != nil {
		return fmt.Sprintf("%d", supplierID)
	}
	return name
}

// ensure time package is used (for GrvDate.Weekday())
var _ = time.Now
