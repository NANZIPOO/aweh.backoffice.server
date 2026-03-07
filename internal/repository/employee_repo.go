package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/aweh-pos/gateway/internal/models"
)

type EmployeeRepository struct {
	BaseRepository
}

func NewEmployeeRepository(tm *TenantManager) *EmployeeRepository {
	return &EmployeeRepository{BaseRepository{TM: tm}}
}

// GetEmployee retrieves a single employee from the current tenant's DB
func (r *EmployeeRepository) GetEmployee(ctx context.Context, userNo int16) (*models.Employee, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	const query = `
		SELECT
			USERNO, ID, PIN, CARDNO, FIRSTNAME, LASTNAME,
			ACCESSLEVEL, ISCLOCKEDIN, CLOCKEDIN, CLOCKEDOUT,
			HOURLYRATE, TOC, CONFIRMORDER, CANVOID
		FROM EMPLOYEE
		WHERE USERNO = ?`

	var emp models.Employee
	// sqlx maps Firebird column names to struct fields using db:"COLUMN_NAME" tags
	if err := db.GetContext(ctx, &emp, query, userNo); err != nil {
		return nil, fmt.Errorf("failed to fetch employee: %w", err)
	}

	return &emp, nil
}

// GetEmployeeByPIN authenticates an employee by first name + PIN.
// FIRSTNAME comparison is case-insensitive via Firebird UPPER().
// Returns errors.New("invalid credentials") for a no-match (caller should 401).
func (r *EmployeeRepository) GetEmployeeByPIN(ctx context.Context, firstName, pin string) (*models.Employee, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	const query = `
		SELECT
			USERNO, ID, PIN, CARDNO, FIRSTNAME, LASTNAME,
			ACCESSLEVEL, ISCLOCKEDIN, CLOCKEDIN, CLOCKEDOUT,
			HOURLYRATE, TOC, CONFIRMORDER, CANVOID
		FROM EMPLOYEE
		WHERE UPPER(FIRSTNAME) = UPPER(?) AND PIN = ?`

	var emp models.Employee
	if err := db.GetContext(ctx, &emp, query, firstName, pin); err != nil {
		return nil, errors.New("invalid credentials")
	}

	return &emp, nil
}

// InsertEmployee fetches a new ID from Firebird before inserting the record
func (r *EmployeeRepository) InsertEmployee(ctx context.Context, emp *models.Employee) (int16, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return 0, err
	}

	// Begin a transaction to ensure atomic ID generation and use
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Fetch the next ID from the generator
	var newUserNo int16
	err = tx.QueryRowContext(ctx, models.NextIDQuery(models.GenEmployee)).Scan(&newUserNo)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch next employee ID: %w", err)
	}

	emp.UserNo = newUserNo

	// 2. Perform the insert
	const insertSQL = `
		INSERT INTO EMPLOYEE (
			USERNO, PIN, CARDNO, FIRSTNAME, LASTNAME,
			ACCESSLEVEL, ISCLOCKEDIN, CANVOID, HOURLYRATE, TOC, CONFIRMORDER
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = tx.ExecContext(ctx, insertSQL,
		emp.UserNo, emp.PIN, emp.CardNo, emp.FirstName, emp.LastName,
		emp.AccessLevel, emp.IsClockedIn, emp.CanVoid, emp.HourlyRate, emp.TOC, emp.ConfirmOrder,
	)

	if err != nil {
		return 0, fmt.Errorf("failed to insert employee: %w", err)
	}

	// 3. Commit
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return newUserNo, nil
}
