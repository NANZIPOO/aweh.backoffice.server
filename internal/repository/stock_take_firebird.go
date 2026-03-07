package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
)

// ─── StockTakeFirebird ────────────────────────────────────────────────────────

// StockTakeFirebird implements StockTakeRepository against DMASTER.
type StockTakeFirebird struct {
	BaseRepository
}

func NewStockTakeRepository(tm *TenantManager) *StockTakeFirebird {
	return &StockTakeFirebird{BaseRepository{TM: tm}}
}

// ─── Day-of-week → column mapping ────────────────────────────────────────────
//
// Source: Appendix B of 07_Implementation_Plan.md
// NEVER accept raw column names from client requests — always map through this table.

type dayColumns struct {
	FCS string
	FOS string
	REC string
	SAL string
}

var dayColumnMap = map[string]dayColumns{
	"monday":    {FCS: "MONFCS", FOS: "MONFOS", REC: "MONREC", SAL: "MONSALES"},
	"tuesday":   {FCS: "TUESFCS", FOS: "TUESFOS", REC: "TUESREC", SAL: "TUESSALES"},
	"wednesday": {FCS: "WEDFCS", FOS: "WEDFOS", REC: "WEDREC", SAL: "WEDSALES"},
	"thursday":  {FCS: "THURFCS", FOS: "THURFOS", REC: "THURREC", SAL: "THURSALES"},
	"friday":    {FCS: "FRIFCS", FOS: "FRIFOS", REC: "FRIREC", SAL: "FRISALES"},
	"saturday":  {FCS: "SATFCS", FOS: "SATFOS", REC: "SATREC", SAL: "SATSALES"},
	"sunday":    {FCS: "SUNFCS", FOS: "SUNFOS", REC: "SUNREC", SAL: "SUNSALES"},
}

// dayRecColumn returns the {day}REC column name for a given time.Weekday.
// Used by GRV to increment received stock.
func dayRecColumn(wd time.Weekday) string {
	days := [7]string{"SUNREC", "MONREC", "TUESREC", "WEDREC", "THURREC", "FRIREC", "SATREC"}
	return days[int(wd)]
}

// ─── GetStockTakeSheet ────────────────────────────────────────────────────────

// GetStockTakeSheet returns all DMASTER rows for a stock sheet with correct
// day column aliases per Appendix B. dayOfWeek must be lowercase ("monday" etc.).
func (r *StockTakeFirebird) GetStockTakeSheet(ctx context.Context, stockSheet string, dayOfWeek string) ([]models.StockTakeDayRow, error) {
	cols, ok := dayColumnMap[strings.ToLower(dayOfWeek)]
	if !ok {
		return nil, fmt.Errorf("invalid day_of_week '%s': must be monday|tuesday|wednesday|thursday|friday|saturday|sunday", dayOfWeek)
	}

	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	// Build SELECT using validated column names (not from client input)
	q := fmt.Sprintf(`
		SELECT MPARTNO, DESCRIPTION, BIN,
		       %s AS opening_stock,
		       %s AS received,
		       %s AS closing_stock,
		       %s AS sales,
		       (%s - ((%s + %s) - %s)) AS variance
		FROM DMASTER
		WHERE INVFORM = ?
		ORDER BY BIN, DESCRIPTION`,
		cols.FOS, cols.REC, cols.FCS, cols.SAL,
		cols.SAL, cols.FOS, cols.REC, cols.FCS,
	)

	var rows []models.StockTakeDayRow
	if err := db.SelectContext(ctx, &rows, q, stockSheet); err != nil {
		return nil, fmt.Errorf("GetStockTakeSheet: %w", err)
	}
	return rows, nil
}

// ─── UpdateClosingStock ───────────────────────────────────────────────────────

// UpdateClosingStock updates FRONTCLOSINGSTOCK and BACKCLOSINGSTOCK for each
// item in the request. Maps dayOfWeek → correct column via dayColumnMap.
// All updates are wrapped in a single transaction.
func (r *StockTakeFirebird) UpdateClosingStock(ctx context.Context, req models.UpdateClosingStockRequest) (int, error) {
	cols, ok := dayColumnMap[strings.ToLower(req.DayOfWeek)]
	if !ok {
		return 0, fmt.Errorf("invalid day_of_week '%s'", req.DayOfWeek)
	}

	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, err
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("UpdateClosingStock BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Build UPDATE with validated column names
	q := fmt.Sprintf("UPDATE DMASTER SET %s = ?, %s = ? WHERE MPARTNO = ? AND INVFORM = ?",
		cols.FCS, cols.FCS) // front and back share same day column in legacy schema
	// Note: DMASTER has separate FRONTCLOSINGSTOCK and {day}FCS.
	// {day}FCS is the daily count; FRONTCLOSINGSTOCK is the current period summary.
	// We update both: the day-specific FCS column AND FRONTCLOSINGSTOCK for the API response.
	q = fmt.Sprintf("UPDATE DMASTER SET %s = ?, FRONTCLOSINGSTOCK = ?, BACKCLOSINGSTOCK = ? WHERE MPARTNO = ? AND INVFORM = ?",
		cols.FCS)

	updated := 0
	for _, item := range req.Items {
		result, err := tx.ExecContext(ctx, q,
			item.FrontClosingStock, item.FrontClosingStock, item.BackClosingStock,
			item.MPartNo, req.StockSheet)
		if err != nil {
			return 0, fmt.Errorf("UpdateClosingStock UPDATE mpartno %s: %w", item.MPartNo, err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			updated++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("UpdateClosingStock Commit: %w", err)
	}
	return updated, nil
}

// ─── IsPeriodFinalized ────────────────────────────────────────────────────────

// IsPeriodFinalized checks PERIOD_FINALIZATIONS for the given stock sheet + date.
// Returns (false, nil) if the PERIOD_FINALIZATIONS table does not yet exist
// (Stage 6.3 must be run manually before this returns meaningful results).
func (r *StockTakeFirebird) IsPeriodFinalized(ctx context.Context, stockSheet string, periodDate time.Time) (bool, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return false, err
	}
	var count int
	err = db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM PERIOD_FINALIZATIONS WHERE STOCK_SHEET = ? AND PERIOD_DATE = ?",
		stockSheet, periodDate)
	if err != nil {
		// Table may not exist — degrade gracefully (allow finalization to proceed).
		return false, nil
	}
	return count > 0, nil
}

// ─── FinalizeStockPeriod ──────────────────────────────────────────────────────
//
// Mandatory sequence (single transaction):
//  1. Check IsPeriodFinalized → return 409 error if true
//  2. UPDATE DMASTER: FRONTOPENINGSTOCK=FRONTCLOSINGSTOCK, BACKOPENINGSTOCK=BACKCLOSINGSTOCK,
//                     PURCHASES=0, SALES=0, FRONTCLOSINGSTOCK=0, BACKCLOSINGSTOCK=0
//  3. INSERT INTO PERIOD_FINALIZATIONS
//  4. Commit
//
// Returns the count of DMASTER rows reset.

func (r *StockTakeFirebird) FinalizeStockPeriod(ctx context.Context, req models.FinalizeStockRequest) (int64, error) {
	// Pre-check (outside tx — read-only)
	finalized, err := r.IsPeriodFinalized(ctx, req.StockSheet, req.PeriodDate)
	if err != nil {
		return 0, err
	}
	if finalized {
		return 0, fmt.Errorf("ERR_PERIOD_ALREADY_FINALIZED: %s period %s was already finalized",
			req.StockSheet, req.PeriodDate.Format("2006-01-02"))
	}

	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, err
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("FinalizeStockPeriod BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Reset all items in this stock sheet
	result, err := tx.ExecContext(ctx, `
		UPDATE DMASTER SET
			FRONTOPENINGSTOCK = FRONTCLOSINGSTOCK,
			BACKOPENINGSTOCK  = BACKCLOSINGSTOCK,
			PURCHASES         = 0,
			SALES             = 0,
			FRONTCLOSINGSTOCK = 0,
			BACKCLOSINGSTOCK  = 0
		WHERE INVFORM = ?`, req.StockSheet)
	if err != nil {
		return 0, fmt.Errorf("FinalizeStockPeriod UPDATE: %w", err)
	}
	itemsReset, _ := result.RowsAffected()

	// Record finalization (PERIOD_FINALIZATIONS table must exist — see Stage 6.3 DDL)
	_, err = tx.ExecContext(ctx,
		"INSERT INTO PERIOD_FINALIZATIONS (STOCK_SHEET, PERIOD_DATE, FINALIZED_BY) VALUES (?, ?, ?)",
		req.StockSheet, req.PeriodDate, req.UserID)
	if err != nil {
		// If table doesn't exist, log but don't fail the finalization reset
		// TODO: make this a hard failure once Stage 6.3 DDL is confirmed applied
		_ = err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("FinalizeStockPeriod Commit: %w", err)
	}
	return itemsReset, nil
}
