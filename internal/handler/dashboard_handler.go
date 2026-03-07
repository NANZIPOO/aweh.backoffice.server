package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aweh-pos/gateway/internal/models"
	"github.com/aweh-pos/gateway/internal/repository"
)

// DashboardHandler handles dashboard-related HTTP requests
type DashboardHandler struct {
	repo repository.DashboardRepository
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(repo repository.DashboardRepository) *DashboardHandler {
	return &DashboardHandler{repo: repo}
}

// GetDashboardSummary handles GET /api/v1/dashboard/summary
// Query params: period (today|yesterday|wtd|last_week|mtd|last_month|ytd|custom)
//               from (YYYY-MM-DD, required if period=custom)
//               to (YYYY-MM-DD, required if period=custom)
//               compare (true|false, default false)
func (h *DashboardHandler) GetDashboardSummary(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "today"
	}

	customFrom := r.URL.Query().Get("from")
	customTo := r.URL.Query().Get("to")
	compare := r.URL.Query().Get("compare") == "true"

	// Calculate date ranges
	from, to, err := calculateDateRange(period, customFrom, customTo)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_PERIOD", err.Error())
		return
	}

	// Fetch sales metrics
	salesData, err := h.repo.GetSalesMetrics(r.Context(), from, to)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Fetch operational status
	operationalData, err := h.repo.GetOperationalStatus(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Compute metrics
	metrics := computeSalesMetrics(salesData)
	payment := computePaymentBreakdown(salesData)
	operational := models.OperationalStatus{
		OpenChecksCount:  operationalData.OpenChecksCount,
		ActiveStaffCount: operationalData.ActiveStaffCount,
	}

	summary := &models.DashboardSummary{
		Period:      period,
		From:        from,
		To:          to,
		Metrics:     *metrics,
		Payment:     *payment,
		Operational: operational,
	}

	// Compute comparison if requested
	if compare {
		compMetrics, err := h.computeComparison(r, period, from, to, salesData)
		if err == nil {
			summary.Comparison = compMetrics
		}
	}

	respondJSON(w, http.StatusOK, summary)
}

// calculateDateRange computes from/to dates based on period filter
func calculateDateRange(period, customFrom, customTo string) (time.Time, time.Time, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	switch period {
	case "today":
		return today, today.Add(24*time.Hour - time.Second), nil

	case "yesterday":
		yesterday := today.Add(-24 * time.Hour)
		return yesterday, yesterday.Add(24*time.Hour - time.Second), nil

	case "wtd": // Week-to-date (Monday-based)
		weekStart := startOfWeek(today)
		return weekStart, today.Add(24*time.Hour - time.Second), nil

	case "last_week":
		thisWeekStart := startOfWeek(today)
		lastWeekStart := thisWeekStart.Add(-7 * 24 * time.Hour)
		lastWeekEnd := thisWeekStart.Add(-time.Second)
		return lastWeekStart, lastWeekEnd, nil

	case "mtd": // Month-to-date
		monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		return monthStart, today.Add(24*time.Hour - time.Second), nil

	case "last_month":
		monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		lastMonthStart := monthStart.AddDate(0, -1, 0)
		lastMonthEnd := monthStart.Add(-time.Second)
		return lastMonthStart, lastMonthEnd, nil

	case "ytd": // Year-to-date
		yearStart := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, today.Location())
		return yearStart, today.Add(24*time.Hour - time.Second), nil

	case "custom":
		if customFrom == "" || customTo == "" {
			return time.Time{}, time.Time{}, http.ErrMissingFile // reuse standard error
		}
		from, err := time.Parse("2006-01-02", customFrom)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to, err := time.Parse("2006-01-02", customTo)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return from, to.Add(24*time.Hour - time.Second), nil

	default:
		return time.Time{}, time.Time{}, http.ErrNotSupported
	}
}

// startOfWeek returns Monday of the week containing the given date
func startOfWeek(d time.Time) time.Time {
	// Go's Weekday: Sunday=0, Monday=1, ..., Saturday=6
	offset := int(d.Weekday()) - 1
	if offset < 0 {
		offset = 6 // Sunday → go back 6 days to Monday
	}
	return d.Add(-time.Duration(offset) * 24 * time.Hour)
}

// computeSalesMetrics calculates derived metrics from raw sales data
func computeSalesMetrics(data *models.RawSalesData) *models.SalesMetrics {
	avgTransaction := 0.0
	if data.TransactionCount > 0 {
		avgTransaction = data.GrossSales / float64(data.TransactionCount)
	}

	avgPerHead := 0.0
	if data.GuestCount > 0 {
		avgPerHead = data.GrossSales / float64(data.GuestCount)
	}

	return &models.SalesMetrics{
		GrossSales:       roundTo2(data.GrossSales),
		NetSales:         roundTo2(data.NetSales),
		Tax:              roundTo2(data.Tax),
		Discounts:        roundTo2(data.Discount),
		Voids:            roundTo2(data.Voids),
		TransactionCount: data.TransactionCount,
		GuestCount:       data.GuestCount,
		AvgTransaction:   roundTo2(avgTransaction),
		AvgPerHead:       roundTo2(avgPerHead),
	}
}

// computePaymentBreakdown calculates payment method percentages
func computePaymentBreakdown(data *models.RawSalesData) *models.PaymentBreakdown {
	total := data.Cash + data.Card + data.Account + data.Voucher

	cashPct := 0.0
	cardPct := 0.0
	accountPct := 0.0
	voucherPct := 0.0

	if total > 0 {
		cashPct = (data.Cash / total) * 100
		cardPct = (data.Card / total) * 100
		accountPct = (data.Account / total) * 100
		voucherPct = (data.Voucher / total) * 100
	}

	return &models.PaymentBreakdown{
		Cash:       roundTo2(data.Cash),
		CashPct:    roundTo2(cashPct),
		Card:       roundTo2(data.Card),
		CardPct:    roundTo2(cardPct),
		Account:    roundTo2(data.Account),
		AccountPct: roundTo2(accountPct),
		Voucher:    roundTo2(data.Voucher),
		VoucherPct: roundTo2(voucherPct),
		Total:      roundTo2(total),
	}
}

// computeComparison calculates period-over-period variance
func (h *DashboardHandler) computeComparison(r *http.Request, period string, from, to time.Time, currentData *models.RawSalesData) (*models.ComparisonMetrics, error) {
	compFrom, compTo := getComparisonPeriod(period, from, to)

	compData, err := h.repo.GetSalesMetrics(r.Context(), compFrom, compTo)
	if err != nil {
		return nil, err
	}

	variancePct := 0.0
	if compData.GrossSales > 0 {
		variancePct = ((currentData.GrossSales - compData.GrossSales) / compData.GrossSales) * 100
	}

	varianceAmt := currentData.GrossSales - compData.GrossSales

	return &models.ComparisonMetrics{
		Period:           getComparisonLabel(period),
		From:             compFrom,
		To:               compTo,
		GrossSales:       roundTo2(compData.GrossSales),
		TransactionCount: compData.TransactionCount,
		VariancePct:      roundTo2(variancePct),
		VarianceAmount:   roundTo2(varianceAmt),
	}, nil
}

// getComparisonPeriod returns the comparison date range for a given period
func getComparisonPeriod(period string, from, to time.Time) (time.Time, time.Time) {
	duration := to.Sub(from)

	switch period {
	case "today":
		// Compare to yesterday
		return from.Add(-24 * time.Hour), to.Add(-24 * time.Hour)
	case "wtd":
		// Compare to last week same days
		return from.Add(-7 * 24 * time.Hour), to.Add(-7 * 24 * time.Hour)
	case "mtd":
		// Compare to last month same date range
		compFrom := from.AddDate(0, -1, 0)
		compTo := to.AddDate(0, -1, 0)
		return compFrom, compTo
	default:
		// Generic: shift back by duration
		return from.Add(-duration), to.Add(-duration)
	}
}

// getComparisonLabel returns a human-readable label for the comparison period
func getComparisonLabel(period string) string {
	switch period {
	case "today":
		return "yesterday"
	case "wtd":
		return "last_week"
	case "mtd":
		return "last_month"
	default:
		return "comparison_period"
	}
}

// roundTo2 rounds a float64 to 2 decimal places
func roundTo2(val float64) float64 {
	return float64(int(val*100+0.5)) / 100
}

// respondJSON sends a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError sends a JSON error response
func respondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
