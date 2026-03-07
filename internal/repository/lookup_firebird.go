package repository

import (
	"context"
	"fmt"

	"github.com/aweh-pos/gateway/internal/models"
)

// LookupFirebird implements LookupRepository using DISTINCT queries against DMASTER.
type LookupFirebird struct {
	BaseRepository
}

func NewLookupRepository(tm *TenantManager) *LookupFirebird {
	return &LookupFirebird{BaseRepository{TM: tm}}
}

// GetStockSheets returns all distinct INVFORM values from DMASTER.
func (r *LookupFirebird) GetStockSheets(ctx context.Context) ([]models.LookupItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Val string `db:"VAL"`
	}
	const q = `SELECT DISTINCT INVFORM AS VAL FROM DMASTER WHERE INVFORM IS NOT NULL ORDER BY 1`
	if err := db.SelectContext(ctx, &rows, q); err != nil {
		return nil, fmt.Errorf("GetStockSheets: %w", err)
	}
	items := make([]models.LookupItem, len(rows))
	for i, row := range rows {
		items[i] = models.LookupItem{Value: row.Val, Label: row.Val}
	}
	return items, nil
}

// GetBins returns all distinct BIN values from DMASTER, excluding the placeholder 'Bin'.
func (r *LookupFirebird) GetBins(ctx context.Context) ([]models.LookupItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Val string `db:"VAL"`
	}
	const q = `SELECT DISTINCT BIN AS VAL FROM DMASTER WHERE BIN IS NOT NULL AND BIN <> 'Bin' ORDER BY 1`
	if err := db.SelectContext(ctx, &rows, q); err != nil {
		return nil, fmt.Errorf("GetBins: %w", err)
	}
	items := make([]models.LookupItem, len(rows))
	for i, row := range rows {
		items[i] = models.LookupItem{Value: row.Val, Label: row.Val}
	}
	return items, nil
}

// GetCategories returns all distinct CATOGORY values from DMASTER.
// Note: CATOGORY is the legacy column name — typo preserved per Project Constitution §4.
func (r *LookupFirebird) GetCategories(ctx context.Context) ([]models.LookupItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Val string `db:"VAL"`
	}
	const q = `SELECT DISTINCT CATOGORY AS VAL FROM DMASTER WHERE CATOGORY IS NOT NULL ORDER BY 1`
	if err := db.SelectContext(ctx, &rows, q); err != nil {
		return nil, fmt.Errorf("GetCategories: %w", err)
	}
	items := make([]models.LookupItem, len(rows))
	for i, row := range rows {
		items[i] = models.LookupItem{Value: row.Val, Label: row.Val}
	}
	return items, nil
}

// GetCostCategories returns all distinct COSTCATEGORY values from DMASTER.
func (r *LookupFirebird) GetCostCategories(ctx context.Context) ([]models.LookupItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Val string `db:"VAL"`
	}
	const q = `SELECT DISTINCT COSTCATEGORY AS VAL FROM DMASTER WHERE COSTCATEGORY IS NOT NULL ORDER BY 1`
	if err := db.SelectContext(ctx, &rows, q); err != nil {
		return nil, fmt.Errorf("GetCostCategories: %w", err)
	}
	items := make([]models.LookupItem, len(rows))
	for i, row := range rows {
		items[i] = models.LookupItem{Value: row.Val, Label: row.Val}
	}
	return items, nil
}

// GetSuppliers returns all suppliers from SUPPLIERS ordered by name.
func (r *LookupFirebird) GetSuppliers(ctx context.Context) ([]models.InventorySupplier, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}
	var suppliers []models.InventorySupplier
	const q = `SELECT SUPPLIERNO, SUPPLIER, PHONE, EMAIL FROM SUPPLIERS ORDER BY SUPPLIER`
	if err := db.SelectContext(ctx, &suppliers, q); err != nil {
		return nil, fmt.Errorf("GetSuppliers: %w", err)
	}
	return suppliers, nil
}
