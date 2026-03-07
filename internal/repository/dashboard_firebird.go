package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
)

// DashboardFirebirdRepository implements DashboardRepository for Firebird
type DashboardFirebirdRepository struct {
	BaseRepository
}

// NewDashboardRepository creates a new Firebird-backed dashboard repository
func NewDashboardRepository(tm *TenantManager) DashboardRepository {
	return &DashboardFirebirdRepository{BaseRepository{TM: tm}}
}

// GetSalesMetrics fetches aggregated sales data for a given date range
func (r *DashboardFirebirdRepository) GetSalesMetrics(ctx context.Context, from, to time.Time) (*models.RawSalesData, error) {
	// Determine if we need to query BILLSHISTORY or BILLS or both
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Normalize from/to to dates only (strip time)
	fromDate := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	toDate := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, to.Location())

	var result models.RawSalesData

	// Case 1: Range ends before today → Query BILLSHISTORY only
	if toDate.Before(today) {
		return r.querySalesFromHistory(ctx, fromDate, toDate)
	}

	// Case 2: Range is today only → Query BILLS only
	if fromDate.Equal(today) && toDate.After(today) {
		return r.querySalesFromLive(ctx)
	}

	// Case 3: Range spans historical + today → Merge both queries
	histData := &models.RawSalesData{}
	liveData := &models.RawSalesData{}

	if fromDate.Before(today) {
		// Get historical data up to yesterday
		yesterday := today.Add(-24 * time.Hour)
		histData, _ = r.querySalesFromHistory(ctx, fromDate, yesterday)
	}

	// Get today's live data
	liveData, _ = r.querySalesFromLive(ctx)

	// Merge results
	result.GrossSales = histData.GrossSales + liveData.GrossSales
	result.NetSales = histData.NetSales + liveData.NetSales
	result.Tax = histData.Tax + liveData.Tax
	result.Discount = histData.Discount + liveData.Discount
	result.Voids = histData.Voids + liveData.Voids
	result.TransactionCount = histData.TransactionCount + liveData.TransactionCount
	result.GuestCount = histData.GuestCount + liveData.GuestCount
	result.Cash = histData.Cash + liveData.Cash
	result.Card = histData.Card + liveData.Card
	result.Account = histData.Account + liveData.Account
	result.Voucher = histData.Voucher + liveData.Voucher

	return &result, nil
}

// querySalesFromHistory queries BILLSHISTORY for historical data
func (r *DashboardFirebirdRepository) querySalesFromHistory(ctx context.Context, from, to time.Time) (*models.RawSalesData, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT
			COALESCE(SUM(GRANDTOTAL), 0) AS GROSSSALES,
			COALESCE(SUM(NETAMOUNT), 0) AS NETSALES,
			COALESCE(SUM(SALESTAX), 0) AS TAX,
			COALESCE(SUM(DISCOUNT), 0) AS DISCOUNT,
			COALESCE(SUM(VOIDS), 0) AS VOIDS,
			COUNT(*) AS TRANSACTION_COUNT,
			COALESCE(SUM(PAX), 0) AS GUEST_COUNT,
			COALESCE(SUM(CASH), 0) AS CASH,
			COALESCE(SUM(CREDITCARD), 0) AS CARD,
			COALESCE(SUM(ACCOUNT), 0) AS ACCOUNT,
			COALESCE(SUM(VOUCHER), 0) AS VOUCHER
		FROM BILLSHISTORY
		WHERE BUSINESSDAY >= ? AND BUSINESSDAY <= ?
			AND BILLOPEN = 'T'
	`

	var data models.RawSalesData
	err = db.GetContext(ctx, &data, query, from, to)
	if err != nil {
		if err == sql.ErrNoRows {
			// No data for this period → return zeros
			return &models.RawSalesData{}, nil
		}
		return nil, fmt.Errorf("query sales history failed: %w", err)
	}

	return &data, nil
}

// querySalesFromLive queries BILLS for today's live data
func (r *DashboardFirebirdRepository) querySalesFromLive(ctx context.Context) (*models.RawSalesData, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT
			COALESCE(SUM(GRANDTOTAL), 0) AS GROSSSALES,
			COALESCE(SUM(NETAMOUNT), 0) AS NETSALES,
			COALESCE(SUM(SALESTAX), 0) AS TAX,
			COALESCE(SUM(DISCOUNT), 0) AS DISCOUNT,
			COALESCE(SUM(VOIDS), 0) AS VOIDS,
			COUNT(*) AS TRANSACTION_COUNT,
			COALESCE(SUM(PAX), 0) AS GUEST_COUNT,
			COALESCE(SUM(CASH), 0) AS CASH,
			COALESCE(SUM(CREDITCARD), 0) AS CARD,
			COALESCE(SUM(ACCOUNT), 0) AS ACCOUNT,
			COALESCE(SUM(VOUCHER), 0) AS VOUCHER
		FROM BILLS
		WHERE BILLOPEN = 'T'
	`

	var data models.RawSalesData
	err = db.GetContext(ctx, &data, query)
	if err != nil {
		if err == sql.ErrNoRows {
			return &models.RawSalesData{}, nil
		}
		return nil, fmt.Errorf("query live sales failed: %w", err)
	}

	return &data, nil
}

// GetOperationalStatus fetches real-time operational data
func (r *DashboardFirebirdRepository) GetOperationalStatus(ctx context.Context) (*models.RawOperationalData, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT
			COUNT(*) AS OPEN_CHECKS_COUNT,
			COUNT(DISTINCT CASHIER) AS ACTIVE_STAFF_COUNT
		FROM BILLS
		WHERE BILLOPEN = 'O'
	`

	var data models.RawOperationalData
	err = db.GetContext(ctx, &data, query)
	if err != nil {
		if err == sql.ErrNoRows {
			return &models.RawOperationalData{}, nil
		}
		return nil, fmt.Errorf("query operational status failed: %w", err)
	}

	return &data, nil
}
