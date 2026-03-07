package repository

import (
	"context"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
)

// DashboardRepository defines operations for fetching dashboard analytics
type DashboardRepository interface {
	// GetSalesMetrics fetches aggregated sales data for a given date range
	// Uses BILLSHISTORY for historical data, BILLS for today's live data
	GetSalesMetrics(ctx context.Context, from, to time.Time) (*models.RawSalesData, error)

	// GetOperationalStatus fetches real-time operational data (open checks, active staff)
	// Uses BILLS table only (live data)
	GetOperationalStatus(ctx context.Context) (*models.RawOperationalData, error)
}
