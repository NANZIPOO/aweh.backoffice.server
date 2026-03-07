package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
)

// BillRepository handles all DB operations for the BILLS table.
// Rules: tenant-isolated via TM.GetDB, no JOINs, mandatory GEN_ID sequence on insert.
type BillRepository struct {
	BaseRepository
}

func NewBillRepository(tm *TenantManager) *BillRepository {
	return &BillRepository{BaseRepository{TM: tm}}
}

// column list is a single source of truth shared by both SELECT statements.
const billColumns = `
	CHECKNO, TILLNO, TABLENO, TABNAME,
	BILLOPEN, PRINTED, INUSE, CASHEDUP,
	USERNO, CASHIER, PAX,
	NETAMOUNT, CASH, CREDITCARD, VOUCHER, CHECKS, PAIDOUT,
	PROMOS, DISCOUNT, VOIDS, STAFF, ACCOUNT, SURCHARGE,
	SALESTAX, TIP, GRANDTOTAL, PAYTYPE, ACCRECEIVED,
	BREAKAGES, POOLTIPS, CARDCOMM,
	OUTLETNO, TDATE, OPENTIME, CLOSEDTIME, CLOSEDBY,
	BUSINESSDAY, OUTLETNAME, SETCALC,
	ORDERMEMO, SALESCATEGORY, FOOTERMEMO, BILLEDTIME`

// GetBill retrieves a single bill by its primary key (CHECKNO).
// No JOIN — any related data (e.g. line items) must be fetched via a separate repo call.
func (r *BillRepository) GetBill(ctx context.Context, checkNo int32) (*models.Bill, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("bill get: resolve tenant db: %w", err)
	}

	const query = `SELECT` + billColumns + `FROM BILLS WHERE CHECKNO = ?`

	var b models.Bill
	if err := db.GetContext(ctx, &b, query, checkNo); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("bill %d not found", checkNo)
		}
		return nil, fmt.Errorf("bill get: query: %w", err)
	}

	return &b, nil
}

// ListBillsByBusinessDay returns all bills whose BUSINESSDAY falls on the given calendar day.
// Uses a date-cast comparison so time-of-day on the stored TIMESTAMP is ignored.
func (r *BillRepository) ListBillsByBusinessDay(ctx context.Context, day time.Time) ([]*models.Bill, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("bill list: resolve tenant db: %w", err)
	}

	const query = `SELECT` + billColumns + `
		FROM BILLS
		WHERE CAST(BUSINESSDAY AS DATE) = CAST(? AS DATE)
		ORDER BY CHECKNO`

	rows, err := db.QueryxContext(ctx, query, day)
	if err != nil {
		return nil, fmt.Errorf("bill list: query: %w", err)
	}
	defer rows.Close()

	var bills []*models.Bill
	for rows.Next() {
		var b models.Bill
		if err := rows.StructScan(&b); err != nil {
			return nil, fmt.Errorf("bill list: scan row: %w", err)
		}
		bills = append(bills, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("bill list: rows iteration: %w", err)
	}

	return bills, nil
}

// InsertBill opens a new bill in the BILLS table.
//
// Write sequence (mandated by architecture):
//  1. Begin explicit transaction.
//  2. Fetch next CHECKNO from bills_gen via GEN_ID.
//  3. INSERT the row with all system-assigned defaults (open flags, zero totals).
//  4. Commit.
func (r *BillRepository) InsertBill(ctx context.Context, req *models.CreateBillRequest) (*models.Bill, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("bill insert: resolve tenant db: %w", err)
	}

	// --- Step 1: Begin transaction ---
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("bill insert: begin tx: %w", err)
	}
	defer tx.Rollback() // no-op after Commit; guards against early returns

	// --- Step 2: Fetch next PK from generator ---
	var newCheckNo int32
	if err := tx.QueryRowContext(ctx, models.NextIDQuery(models.GenBills)).Scan(&newCheckNo); err != nil {
		return nil, fmt.Errorf("bill insert: fetch gen_id(bills_gen): %w", err)
	}

	// --- Step 3: Build nullable fields from the request ---
	now := time.Now()

	tillNo := toNullInt32(req.TillNo)
	tableNo := toNullString(req.TableNo)
	tabName := toNullString(req.TabName)
	userNo := toNullInt32(req.UserNo)
	cashier := toNullString(req.Cashier)
	pax := toNullInt32(req.Pax)
	outletNo := toNullInt32(req.OutletNo)
	outletName := toNullString(req.OutletName)
	salesCat := toNullString(req.SalesCategory)
	bizDay := toNullTime(req.BusinessDay)
	orderMemo := toNullString(req.OrderMemo)

	const insertSQL = `
		INSERT INTO BILLS (
			CHECKNO, TILLNO, TABLENO, TABNAME,
			BILLOPEN, PRINTED, INUSE, CASHEDUP,
			USERNO, CASHIER, PAX,
			NETAMOUNT, CASH, CREDITCARD, VOUCHER, CHECKS, PAIDOUT,
			PROMOS, DISCOUNT, VOIDS, STAFF, ACCOUNT, SURCHARGE,
			SALESTAX, TIP, GRANDTOTAL, ACCRECEIVED,
			BREAKAGES, POOLTIPS, CARDCOMM,
			OUTLETNO, TDATE, OPENTIME, BUSINESSDAY, OUTLETNAME,
			SETCALC, ORDERMEMO, SALESCATEGORY
		) VALUES (
			?, ?, ?, ?,
			'T', 'F', 'T', 'F',
			?, ?, ?,
			0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0,
			?, ?, ?, ?, ?,
			'F', ?, ?
		)`

	_, err = tx.ExecContext(ctx, insertSQL,
		newCheckNo, tillNo, tableNo, tabName,
		userNo, cashier, pax,
		outletNo, now, now, bizDay, outletName,
		orderMemo, salesCat,
	)
	if err != nil {
		return nil, fmt.Errorf("bill insert: exec insert: %w", err)
	}

	// --- Step 4: Commit ---
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("bill insert: commit: %w", err)
	}

	// Return the freshly-created bill by doing a second lookup (no dirty field in tx).
	// This is the "no guessing" approach — we read exactly what was committed.
	return r.GetBill(ctx, newCheckNo)
}

// CloseBill settles an open bill. This is the core POS money path.
//
// Write sequence:
//  1. Begin explicit transaction.
//  2. Lock the BILLS row and read BILLOPEN to guard against double-close.
//  3. Reject if BILLOPEN != 'T' (already closed or voided).
//  4. UPDATE all payment totals + status flags in a single statement.
//  5. Commit.
//  6. Return the committed row via a clean second SELECT (no stale tx state).
func (r *BillRepository) CloseBill(ctx context.Context, checkNo int32, req *models.CloseBillRequest) (*models.Bill, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("bill close: resolve tenant db: %w", err)
	}

	// --- Step 1: Begin transaction ---
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("bill close: begin tx: %w", err)
	}
	defer tx.Rollback()

	// --- Step 2: Lock row + guard against double-close ---
	// Firebird: SELECT ... FOR UPDATE WITH LOCK acquires a pessimistic row lock
	// so two cashiers can't close the same bill simultaneously.
	var billOpen string
	const lockSQL = `SELECT BILLOPEN FROM BILLS WHERE CHECKNO = ? FOR UPDATE WITH LOCK`
	if err := tx.QueryRowContext(ctx, lockSQL, checkNo).Scan(&billOpen); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, fmt.Errorf("bill %d not found", checkNo)
		}
		return nil, fmt.Errorf("bill close: lock row: %w", err)
	}

	// --- Step 3: Guard ---
	if billOpen != "T" && billOpen != "t" {
		return nil, fmt.Errorf("bill %d is not open (BILLOPEN=%q)", checkNo, billOpen)
	}

	// --- Step 4: UPDATE all payment fields and flip status flags ---
	const updateSQL = `
		UPDATE BILLS SET
			BILLOPEN    = 'F',
			INUSE       = 'F',
			PRINTED     = 'T',
			CLOSEDTIME  = ?,
			CLOSEDBY    = ?,
			PAYTYPE     = ?,
			NETAMOUNT   = ?,
			GRANDTOTAL  = ?,
			CASH        = ?,
			CREDITCARD  = ?,
			VOUCHER     = ?,
			CHECKS      = ?,
			PAIDOUT     = ?,
			PROMOS      = ?,
			DISCOUNT    = ?,
			VOIDS       = ?,
			STAFF       = ?,
			ACCOUNT     = ?,
			SURCHARGE   = ?,
			SALESTAX    = ?,
			TIP         = ?,
			ACCRECEIVED = ?,
			BREAKAGES   = ?,
			POOLTIPS    = ?,
			CARDCOMM    = ?
		WHERE CHECKNO = ?`

	now := time.Now()
	_, err = tx.ExecContext(ctx, updateSQL,
		now, req.ClosedBy, req.PayType,
		req.NetAmount, req.GrandTotal,
		req.Cash, req.CreditCard, req.Voucher, req.Checks,
		req.PaidOut, req.Promos, req.Discount, req.Voids,
		req.Staff, req.Account, req.Surcharge, req.SalesTax, req.Tip,
		req.AccReceived, req.Breakages, req.PoolTips, req.CardComm,
		checkNo,
	)
	if err != nil {
		return nil, fmt.Errorf("bill close: exec update: %w", err)
	}

	// --- Step 5: Commit ---
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("bill close: commit: %w", err)
	}

	// Step 6: Return committed state.
	return r.GetBill(ctx, checkNo)
}

// CashUpBill marks a closed bill as cashed-up during end-of-shift reconciliation.
// The bill must already be closed (BILLOPEN='F') before it can be cashed up.
func (r *BillRepository) CashUpBill(ctx context.Context, checkNo int32) (*models.Bill, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("bill cashup: resolve tenant db: %w", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("bill cashup: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Lock + guard: must be closed before cashing up.
	var billOpen string
	const lockSQL = `SELECT BILLOPEN FROM BILLS WHERE CHECKNO = ? FOR UPDATE WITH LOCK`
	if err := tx.QueryRowContext(ctx, lockSQL, checkNo).Scan(&billOpen); err != nil {
		return nil, fmt.Errorf("bill cashup: lock row: %w", err)
	}
	if billOpen == "T" || billOpen == "t" {
		return nil, fmt.Errorf("bill %d is still open — close it before cashing up", checkNo)
	}

	const updateSQL = `UPDATE BILLS SET CASHEDUP = 'T' WHERE CHECKNO = ?`
	if _, err := tx.ExecContext(ctx, updateSQL, checkNo); err != nil {
		return nil, fmt.Errorf("bill cashup: exec update: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("bill cashup: commit: %w", err)
	}

	return r.GetBill(ctx, checkNo)
}

// VoidBill voids a bill that has been opened but not yet settled.
// Sets BILLOPEN='F', INUSE='F' and records who voided it and when.
// Refusing to void an already-closed bill preserves the audit trail.
func (r *BillRepository) VoidBill(ctx context.Context, checkNo int32, req *models.VoidBillRequest) (*models.Bill, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("bill void: resolve tenant db: %w", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("bill void: begin tx: %w", err)
	}
	defer tx.Rollback()

	var billOpen string
	const lockSQL = `SELECT BILLOPEN FROM BILLS WHERE CHECKNO = ? FOR UPDATE WITH LOCK`
	if err := tx.QueryRowContext(ctx, lockSQL, checkNo).Scan(&billOpen); err != nil {
		return nil, fmt.Errorf("bill void: lock row: %w", err)
	}
	if billOpen != "T" && billOpen != "t" {
		return nil, fmt.Errorf("bill %d is not open — cannot void a closed bill", checkNo)
	}

	const updateSQL = `
		UPDATE BILLS SET
			BILLOPEN   = 'F',
			INUSE      = 'F',
			CLOSEDTIME = ?,
			CLOSEDBY   = ?
		WHERE CHECKNO = ?`

	if _, err := tx.ExecContext(ctx, updateSQL, time.Now(), req.ClosedBy, checkNo); err != nil {
		return nil, fmt.Errorf("bill void: exec update: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("bill void: commit: %w", err)
	}

	return r.GetBill(ctx, checkNo)
}

// ---------------------------------------------------------------------------
// private null-coercion helpers
// ---------------------------------------------------------------------------

func toNullInt32(v *int32) sql.NullInt32 {
	if v == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *v, Valid: true}
}

func toNullString(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

func toNullTime(v *time.Time) sql.NullTime {
	if v == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *v, Valid: true}
}
