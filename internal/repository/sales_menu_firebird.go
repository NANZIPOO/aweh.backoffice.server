package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/aweh-pos/gateway/internal/models"
)

type SalesMenuFirebird struct {
	BaseRepository
}

func NewSalesMenuRepository(tm *TenantManager) *SalesMenuFirebird {
	return &SalesMenuFirebird{BaseRepository{TM: tm}}
}

func (r *SalesMenuFirebird) GetGroups(ctx context.Context) ([]models.SalesMenuGroup, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	const q = `
		SELECT
			TRIM(COALESCE(CATEGORY, ''))   AS ID,
			TRIM(COALESCE(CATEGORY, ''))   AS LABEL,
			TRIM(COALESCE(DEPARTMENT, '')) AS DEPARTMENT,
			COALESCE(BUTTONPOS, 0)         AS BUTTONPOS
		FROM MENUGROUPS
		ORDER BY BUTTONPOS, CATEGORY
	`

	var groups []models.SalesMenuGroup
	if err := db.SelectContext(ctx, &groups, q); err != nil {
		return nil, fmt.Errorf("GetGroups: %w", err)
	}
	return groups, nil
}

func (r *SalesMenuFirebird) GetItems(ctx context.Context, groupID string) ([]models.SalesMenuItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT
			RECORD_NO                        AS ID,
			TRIM(COALESCE(MENUFAMILY, ''))   AS GROUP_ID,
			TRIM(COALESCE(DESCRIPTION, ''))  AS LABEL,
			COALESCE(PRICE, 0)               AS PRICE,
			COALESCE(COSTPRICE, 0)           AS COST_PRICE,
			COALESCE(SALESTAX, 0)            AS SALES_TAX,
			TRIM(COALESCE(BARCODE, ''))      AS BARCODE,
			TRIM(COALESCE(MPARTNO, ''))      AS MPARTNO,
			TRIM(COALESCE(PARTNO, ''))       AS PARTNO,
			TRIM(COALESCE(ITEMTYPE, ''))     AS ITEM_TYPE,
			TRIM(COALESCE(CATEGORYFLAG, '')) AS CATEGORY_FLAG,
			TRIM(COALESCE(SPEEDSCREEN, ''))  AS SPEED_SCREEN,
			TRIM(COALESCE(FORCEDPOPUP, ''))  AS FORCED_POPUP,
			TRIM(COALESCE(ALLOWDISCOUNT, '')) AS ALLOW_DISCOUNT
		FROM SALESMENU
	`
	args := make([]any, 0, 1)
	if strings.TrimSpace(groupID) != "" {
		query += " WHERE MENUFAMILY = ?"
		args = append(args, groupID)
	}
	query += " ORDER BY DESCRIPTION"

	var items []models.SalesMenuItem
	if err := db.SelectContext(ctx, &items, query, args...); err != nil {
		return nil, fmt.Errorf("GetItems: %w", err)
	}
	for i := range items {
		items[i].Enabled = strings.ToUpper(strings.TrimSpace(items[i].CategoryFlag)) != "F"
	}
	return items, nil
}

func (r *SalesMenuFirebird) GetItem(ctx context.Context, itemID int64) (*models.SalesMenuItem, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	const q = `
		SELECT
			RECORD_NO                        AS ID,
			TRIM(COALESCE(MENUFAMILY, ''))   AS GROUP_ID,
			TRIM(COALESCE(DESCRIPTION, ''))  AS LABEL,
			COALESCE(PRICE, 0)               AS PRICE,
			COALESCE(COSTPRICE, 0)           AS COST_PRICE,
			COALESCE(SALESTAX, 0)            AS SALES_TAX,
			TRIM(COALESCE(BARCODE, ''))      AS BARCODE,
			TRIM(COALESCE(MPARTNO, ''))      AS MPARTNO,
			TRIM(COALESCE(PARTNO, ''))       AS PARTNO,
			TRIM(COALESCE(ITEMTYPE, ''))     AS ITEM_TYPE,
			TRIM(COALESCE(CATEGORYFLAG, '')) AS CATEGORY_FLAG,
			TRIM(COALESCE(SPEEDSCREEN, ''))  AS SPEED_SCREEN,
			TRIM(COALESCE(FORCEDPOPUP, ''))  AS FORCED_POPUP,
			TRIM(COALESCE(ALLOWDISCOUNT, '')) AS ALLOW_DISCOUNT
		FROM SALESMENU
		WHERE RECORD_NO = ?
	`

	var item models.SalesMenuItem
	if err := db.GetContext(ctx, &item, q, itemID); err != nil {
		return nil, fmt.Errorf("ERR_NOT_FOUND: sales menu item %d not found", itemID)
	}
	item.Enabled = strings.ToUpper(strings.TrimSpace(item.CategoryFlag)) != "F"
	return &item, nil
}

// ── Write Operations: Groups ─────────────────────────────────────────────────

func (r *SalesMenuFirebird) CreateGroup(ctx context.Context, group *models.SalesMenuGroup) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}

	// MENUGROUPS uses CATEGORY (string) as primary key - no generator needed
	const q = `
		INSERT INTO MENUGROUPS (CATEGORY, DEPARTMENT, BUTTONPOS)
		VALUES (?, ?, ?)
	`
	_, err = db.ExecContext(ctx, q, group.ID, group.Department, group.ButtonPos)
	if err != nil {
		return fmt.Errorf("CreateGroup: %w", err)
	}
	return nil
}

func (r *SalesMenuFirebird) UpdateGroup(ctx context.Context, id string, group *models.SalesMenuGroup) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}

	const q = `
		UPDATE MENUGROUPS
		SET DEPARTMENT = ?, BUTTONPOS = ?
		WHERE CATEGORY = ?
	`
	result, err := db.ExecContext(ctx, q, group.Department, group.ButtonPos, id)
	if err != nil {
		return fmt.Errorf("UpdateGroup: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ERR_NOT_FOUND: group '%s' not found", id)
	}
	return nil
}

func (r *SalesMenuFirebird) DeleteGroup(ctx context.Context, id string) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}

	// Hard delete - check if any items reference this group first
	var count int
	if err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM SALESMENU WHERE MENUFAMILY = ?", id); err != nil {
		return fmt.Errorf("DeleteGroup: check references: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("ERR_CONFLICT: cannot delete group '%s' - %d items still reference it", id, count)
	}

	const q = `DELETE FROM MENUGROUPS WHERE CATEGORY = ?`
	result, err := db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("DeleteGroup: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ERR_NOT_FOUND: group '%s' not found", id)
	}
	return nil
}

// ── Write Operations: Items ──────────────────────────────────────────────────

// Mandatory DB Sequence: BeginTxx → GEN_ID(SALESMENU_GEN) → INSERT → Commit
func (r *SalesMenuFirebird) CreateItem(ctx context.Context, item *models.SalesMenuItem) (int64, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, err
	}

	// Step 1: Begin transaction
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateItem BeginTxx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Step 2: Fetch next RECORD_NO using generator
	// Note: Firebird generator name may vary - common patterns are SALESMENU_GEN, SM_GEN, or RECORD_NO_GEN
	var recordNo int64
	genQuery := "SELECT GEN_ID(SALESMENU_GEN, 1) FROM RDB$DATABASE"
	if err := tx.GetContext(ctx, &recordNo, genQuery); err != nil {
		// Try alternative generator name if first fails
		genQuery = "SELECT GEN_ID(SM_GEN, 1) FROM RDB$DATABASE"
		if err2 := tx.GetContext(ctx, &recordNo, genQuery); err2 != nil {
			return 0, fmt.Errorf("CreateItem GEN_ID: %w (tried SALESMENU_GEN and SM_GEN)", err)
		}
	}

	// Step 3: Insert with fetched ID
	const insertQ = `
		INSERT INTO SALESMENU (
			RECORD_NO, MENUFAMILY, DESCRIPTION, PRICE, COSTPRICE, SALESTAX,
			BARCODE, MPARTNO, PARTNO, ITEMTYPE, CATEGORYFLAG,
			SPEEDSCREEN, FORCEDPOPUP, ALLOWDISCOUNT
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	categoryFlag := item.CategoryFlag
	if !item.Enabled {
		categoryFlag = "F"
	}

	_, err = tx.ExecContext(ctx, insertQ,
		recordNo, item.GroupID, item.Label, item.Price, item.CostPrice, item.SalesTax,
		item.Barcode, item.MPartNo, item.PartNo, item.ItemType, categoryFlag,
		item.SpeedScreen, item.ForcedPopup, item.AllowDiscount,
	)
	if err != nil {
		return 0, fmt.Errorf("CreateItem INSERT: %w", err)
	}

	// Step 4: Commit
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("CreateItem Commit: %w", err)
	}

	return recordNo, nil
}

func (r *SalesMenuFirebird) UpdateItem(ctx context.Context, id int64, item *models.SalesMenuItem) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}

	categoryFlag := item.CategoryFlag
	if !item.Enabled {
		categoryFlag = "F"
	}

	const q = `
		UPDATE SALESMENU
		SET MENUFAMILY = ?, DESCRIPTION = ?, PRICE = ?, COSTPRICE = ?, SALESTAX = ?,
		    BARCODE = ?, MPARTNO = ?, PARTNO = ?, ITEMTYPE = ?, CATEGORYFLAG = ?,
		    SPEEDSCREEN = ?, FORCEDPOPUP = ?, ALLOWDISCOUNT = ?
		WHERE RECORD_NO = ?
	`
	result, err := db.ExecContext(ctx, q,
		item.GroupID, item.Label, item.Price, item.CostPrice, item.SalesTax,
		item.Barcode, item.MPartNo, item.PartNo, item.ItemType, categoryFlag,
		item.SpeedScreen, item.ForcedPopup, item.AllowDiscount,
		id,
	)
	if err != nil {
		return fmt.Errorf("UpdateItem: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ERR_NOT_FOUND: item %d not found", id)
	}
	return nil
}

func (r *SalesMenuFirebird) DeleteItem(ctx context.Context, id int64) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}

	// Soft delete: set CATEGORYFLAG = 'F' to mark as disabled
	const q = `
		UPDATE SALESMENU
		SET CATEGORYFLAG = 'F'
		WHERE RECORD_NO = ?
	`
	result, err := db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("DeleteItem: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ERR_NOT_FOUND: item %d not found", id)
	}
	return nil
}

