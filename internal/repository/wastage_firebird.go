package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
)

// ─── WastageFirebird ──────────────────────────────────────────────────────────

// WastageFirebird implements WastageRepository using ORDERITEMS.
type WastageFirebird struct {
	BaseRepository
}

func NewWastageRepository(tm *TenantManager) *WastageFirebird {
	return &WastageFirebird{BaseRepository{TM: tm}}
}

// ─── RecordWastage ────────────────────────────────────────────────────────────
//
// Inserts a single ORDERITEMS row with SUPPLIER='Wastage Control', POSTED='F'.
// Does NOT update DMASTER (Decision D-03 in implementation plan).
// Use PostPendingWastage to batch-post wastage into DMASTER.PURCHASES.

func (r *WastageFirebird) RecordWastage(ctx context.Context, req models.RecordWastageRequest) (*models.WastageLine, error) {
	if req.MPartNo == "" {
		return nil, fmt.Errorf("ERR_VALIDATION: item_id is required")
	}
	if req.Qty <= 0 {
		return nil, fmt.Errorf("ERR_VALIDATION: qty must be greater than 0")
	}

	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch item costing data — single-table SELECT, no JOIN
	var item struct {
		Description  string  `db:"DESCRIPTION"`
		Pack         float64 `db:"PACK"`
		PackCost     float64 `db:"PACKCOST"`
		EachCost     float64 `db:"EACHCOST"`
		TaxRate      float64 `db:"TAXRATE"`
		PackUnit     string  `db:"PACKUNIT"`
		EachUnit     string  `db:"EACHUNIT"`
		Category     *string `db:"CATOGORY"`
		CostCategory *string `db:"COSTCATEGORY"`
	}
	if err := db.GetContext(ctx, &item,
		"SELECT DESCRIPTION, PACK, PACKCOST, EACHCOST, TAXRATE, PACKUNIT, EACHUNIT, CATOGORY, COSTCATEGORY FROM DMASTER WHERE MPARTNO = ?",
		req.MPartNo); err != nil {
		return nil, fmt.Errorf("RecordWastage: item '%s' not found: %w", req.MPartNo, err)
	}

	// ── Mandatory DB Sequence ─────────────────────────────────────────────────
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("RecordWastage BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Fetch ORDERNO from generator (each wastage entry is its own "order" in ORDERITEMS)
	var orderNo int64
	if err := tx.GetContext(ctx, &orderNo, "SELECT GEN_ID(ORDERS_GEN, 1) FROM RDB$DATABASE"); err != nil {
		return nil, fmt.Errorf("RecordWastage GEN_ID: %w", err)
	}

	orderDate := time.Now()
	pack := item.Pack
	if pack <= 0 {
		pack = 1
	}

	catVal := ""
	if item.Category != nil {
		catVal = *item.Category
	}
	costCatVal := ""
	if item.CostCategory != nil {
		costCatVal = *item.CostCategory
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ORDERITEMS (
			ORDERNO, MPARTNO, DESCRIPTION, QTY, PACK, PACKCOST, EACHCOST,
			TAXRATE, CATOGORY, COSTCATEGORY, PACKUNIT, EACHUNIT, ORDERDATE, POSTED, SUPPLIER
		) VALUES (
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, 'F', 'Wastage Control'
		)`,
		orderNo, req.MPartNo, item.Description, req.Qty, pack, round2dp(item.PackCost), round2dp(item.EachCost),
		item.TaxRate, catVal, costCatVal, item.PackUnit, item.EachUnit, orderDate,
	); err != nil {
		return nil, fmt.Errorf("RecordWastage INSERT: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("RecordWastage Commit: %w", err)
	}

	// Return the wastage line as-inserted
	wastageLine := &models.WastageLine{
		OrderNo:     orderNo,
		MPartNo:     req.MPartNo,
		Description: item.Description,
		Qty:         req.Qty,
		EachCost:    round2dp(item.EachCost),
		PackCost:    round2dp(item.PackCost),
		Pack:        pack,
		TaxRate:     item.TaxRate,
		PackUnit:    item.PackUnit,
		EachUnit:    item.EachUnit,
		OrderDate:   orderDate,
		Posted:      "F",
	}
	return wastageLine, nil
}

// ─── PostPendingWastage ───────────────────────────────────────────────────────
//
// Admin-only (access_level >= 5 enforced in handler).
// Batch-posts ORDERITEMS rows where SUPPLIER='Wastage Control' AND POSTED='F'
// into DMASTER.PURCHASES (subtraction) and marks them POSTED='T'.
//
// If stockSheet is provided, only items whose DMASTER.INVFORM matches are processed.
// Per Project Constitution §4 (no complex JOINs): filtering by stock sheet requires
// two sequential single-table SELECTs rather than a subquery or JOIN.

func (r *WastageFirebird) PostPendingWastage(ctx context.Context, stockSheet string) (int, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, err
	}

	// Fetch all pending wastage lines
	var pendingLines []struct {
		OrderNo int64   `db:"ORDERNO"`
		MPartNo string  `db:"MPARTNO"`
		Qty     float64 `db:"QTY"`
	}
	if err := db.SelectContext(ctx, &pendingLines,
		"SELECT ORDERNO, MPARTNO, QTY FROM ORDERITEMS WHERE SUPPLIER = 'Wastage Control' AND POSTED = 'F'",
	); err != nil {
		return 0, fmt.Errorf("PostPendingWastage SELECT: %w", err)
	}

	if len(pendingLines) == 0 {
		return 0, nil
	}

	// If stockSheet filter: fetch MPARTNO→INVFORM mapping (single-table SELECT per the constitution)
	invFormByMPartNo := map[string]string{}
	if stockSheet != "" {
		var items []struct {
			MPartNo string `db:"MPARTNO"`
			InvForm string `db:"INVFORM"`
		}
		if err := db.SelectContext(ctx, &items,
			"SELECT MPARTNO, INVFORM FROM DMASTER WHERE INVFORM = ?", stockSheet,
		); err != nil {
			return 0, fmt.Errorf("PostPendingWastage filter SELECT: %w", err)
		}
		for _, row := range items {
			invFormByMPartNo[row.MPartNo] = row.InvForm
		}
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("PostPendingWastage BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	posted := 0
	for _, line := range pendingLines {
		// Apply stockSheet filter if provided
		if stockSheet != "" {
			if invFormByMPartNo[line.MPartNo] != stockSheet {
				continue
			}
		}

		// Deduct from DMASTER.PURCHASES (wastage reduces stock)
		if _, err := tx.ExecContext(ctx,
			"UPDATE DMASTER SET PURCHASES = PURCHASES - ? WHERE MPARTNO = ?",
			line.Qty, line.MPartNo); err != nil {
			return 0, fmt.Errorf("PostPendingWastage UPDATE DMASTER mpartno %s: %w", line.MPartNo, err)
		}

		// Mark wastage line as posted
		if _, err := tx.ExecContext(ctx,
			"UPDATE ORDERITEMS SET POSTED = 'T' WHERE ORDERNO = ?",
			line.OrderNo); err != nil {
			return 0, fmt.Errorf("PostPendingWastage UPDATE ORDERITEMS orderno %d: %w", line.OrderNo, err)
		}
		posted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("PostPendingWastage Commit: %w", err)
	}
	return posted, nil
}
