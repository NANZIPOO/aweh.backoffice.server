package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aweh-pos/gateway/internal/config"
	"github.com/aweh-pos/gateway/internal/handler"
	"github.com/aweh-pos/gateway/internal/middleware"
	"github.com/aweh-pos/gateway/internal/migrations"
	"github.com/aweh-pos/gateway/internal/models"
	"github.com/aweh-pos/gateway/internal/repository"
)

// legacyAccessLevelToInt maps the Delphi POS VARCHAR ACCESSLEVEL values to
// integer levels used by RequireLevel middleware.
//
//	ADMIN / MANAGER → 5  (full CRUD + finalize)
//	SUPERVISOR      → 3  (stock-take + CRUD, no delete/finalize)
//	CASHIER / USER  → 2  (stock-take entry)
//	WAITER + others → 1  (read-only)
func legacyAccessLevelToInt(s string) int {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ADMIN", "MANAGER", "ADMINISTRATOR":
		return 5
	case "SUPERVISOR":
		return 3
	case "CASHIER", "USER":
		return 2
	default:
		return 1
	}
}

func main() {
	// 0. Load configuration from env vars (dev defaults provided by config.Load)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// 1. Initialise Tenant Manager
	tm := repository.NewTenantManager()

	// Register a default local tenant — DSN comes from config
	tm.RegisterTenantDB("tenant_test_001", cfg.FirebirdDSN())

	// Eager DB ping — fail fast so we know at boot whether Firebird is reachable.
	if err := tm.PingTenant("tenant_test_001"); err != nil {
		log.Fatalf("DB ping failed for tenant_test_001: %v", err)
	}
	log.Println("DB connection OK —", cfg.FirebirdDSN())

	// 1.5 Run migrations if AUTO_MIGRATE is enabled
	tenantDB, err := tm.GetTenantDB("tenant_test_001")
	if err != nil {
		log.Fatalf("migrations: failed to get tenant DB: %v", err)
	}
	if err := migrations.RunMigrations(tenantDB.DB, cfg.AutoMigrate); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// 2. Initialise legacy Repositories
	compRepo := repository.NewCompanyRepository(tm)
	empRepo := repository.NewEmployeeRepository(tm)
	billRepo := repository.NewBillRepository(tm)
	poRepo := repository.NewPurchaseOrderRepository(tm)

	// 3. Initialise inventory repositories
	invRepo := repository.NewInventoryRepository(tm)
	luRepo := repository.NewLookupRepository(tm)
	stRepo := repository.NewStockTakeRepository(tm)
	grvRepo := repository.NewGrvRepository(tm)
	wastRepo := repository.NewWastageRepository(tm)
	groupRepo := repository.NewGroupRepository(tm)

	// 4. Initialise dashboard repository
	dashRepo := repository.NewDashboardRepository(tm)

	// 5. Initialise settings repository
	settingsRepo := repository.NewSettingsRepository(tm, "") // empty string uses default path

	// 6. Initialise sales menu repository
	salesMenuRepo := repository.NewSalesMenuRepository(tm)

	// 7. Define Handlers
	jwtSecret := cfg.JWTSecret
	wireCrypt := "false"
	if cfg.WireCrypt {
		wireCrypt = "true"
	}
	buildDSN := func(dbHost, dbPort, dbPath string) string {
		return fmt.Sprintf("%s:%s@%s:%s/%s?auth_plugin_name=%s&wire_crypt=%s",
			cfg.DBUser, cfg.DBPass, dbHost, dbPort, dbPath, cfg.AuthPlugin, wireCrypt)
	}

	// Health Check / Company Info for Flutter Branding
	http.HandleFunc("/api/v1/company", func(w http.ResponseWriter, r *http.Request) {
		// Log the check
		log.Printf("Health check from %s", r.RemoteAddr)

		// 1. DYNAMIC CONFIGURATION OVERRIDE
		// Flutter sends its local config in query params.
		// If they differ from what the Go server started with, we update it.
		q := r.URL.Query()
		h, p, path := q.Get("db_host"), q.Get("db_port"), q.Get("db_path")
		if h != "" && path != "" {
			// Rebuild DSN with runtime auth settings from config.
			dsn := buildDSN(h, p, path)
			tm.RegisterTenantDB("tenant_test_001", dsn)
		}

		// 2. FETCH COMPANY INFO
		// This endpoint is pre-auth (used for login screen branding), so we
		// inject the default tenant_id directly — no JWT middleware runs here.
		ctx := context.WithValue(r.Context(), middleware.TenantIDKey, "tenant_test_001")
		comp, err := compRepo.GetCompany(ctx)
		if err != nil {
			log.Printf("DB error: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "Database connection failed",
				"details": err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"company_name": comp.CompanyName.String,
			"branch_name":  comp.BranchName.String,
			"address":      comp.Address.String,
			"city":         comp.City.String,
			"vat_no":       comp.VatNo.String,
			"phone_no":     comp.PhoneNo.String,
			"email":        comp.Email.String,
			"logo_path":    comp.LogoPath.String,
			"status":       "ok",
		})
	})

	// Explicit Health Check for config dialog testing (Gateway + Database)
	http.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		h, p, path := q.Get("db_host"), q.Get("db_port"), q.Get("db_path")

		if h == "" || p == "" || path == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"gateway_ok": true,
				"db_ok":      false,
				"message":    "Missing db_host, db_port, or db_path",
			})
			return
		}

		dsn := buildDSN(h, p, path)
		tm.RegisterTenantDB("tenant_test_001", dsn)

		if err := tm.PingTenant("tenant_test_001"); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"gateway_ok": true,
				"db_ok":      false,
				"message":    err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"gateway_ok": true,
			"db_ok":      true,
			"message":    "Connection successful",
		})
	})

	// Version Check Handler - For app update notifications
	http.HandleFunc("/api/v1/updates/check", handler.CheckVersionHandler)

	// Version Info Handler - Returns server build metadata
	http.HandleFunc("/api/v1/version", handler.GetVersionHandler)

	// Download endpoint for published client binaries (APK/EXE).
	// Place files in DOWNLOADS_DIR on host (default: ./downloads).
	downloadsDir := os.Getenv("DOWNLOADS_DIR")
	if downloadsDir == "" {
		downloadsDir = "downloads"
	}
	log.Printf("Downloads directory: %s", downloadsDir)
	http.Handle("/downloads/", http.StripPrefix("/downloads/", http.FileServer(http.Dir(downloadsDir))))

	// Login Handler - Authenticate with FirstName + PIN
	http.HandleFunc("/api/v1/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
			return
		}

		var loginReq struct {
			TenantID  string `json:"tenant_id"`
			FirstName string `json:"first_name"`
			PIN       string `json:"pin"`
		}

		if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request format"})
			return
		}

		// Inject tenant context for authentication query
		ctx := context.WithValue(r.Context(), middleware.TenantIDKey, loginReq.TenantID)

		// Authenticate against EMPLOYEE table using FirstName + PIN (case-insensitive)
		emp, err := empRepo.GetEmployeeByPIN(ctx, loginReq.FirstName, loginReq.PIN)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid credentials"})
			return
		}

		// Map legacy string ACCESSLEVEL to integer for JWT claims.
		// Legacy Delphi POS stores human-readable role names in VARCHAR ACCESSLEVEL.
		accessInt := legacyAccessLevelToInt(emp.AccessLevel.String)

		// Generate JWT with full claims including access_level so RequireLevel works.
		token, err := middleware.GenerateFullToken(
			loginReq.TenantID,
			emp.UserNo,
			int64(emp.UserNo), // userID — reuse UserNo as ID until a separate ID column is used
			emp.FirstName.String,
			accessInt,
			jwtSecret,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to generate token"})
			return
		}

		// Return token and employee info
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":        token,
			"user_no":      emp.UserNo,
			"first_name":   emp.FirstName.String,
			"last_name":    emp.LastName.String,
			"access_level": emp.AccessLevel.String,
		})
	})

	// Test endpoint to list all employees (for debugging - REMOVE IN PRODUCTION)
	http.HandleFunc("/api/v1/employees/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx := context.WithValue(r.Context(), middleware.TenantIDKey, "tenant_test_001")

		db, err := tm.GetDB(ctx)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		var employees []models.Employee
		query := `SELECT USERNO, FIRSTNAME, LASTNAME, PIN, ACCESSLEVEL FROM EMPLOYEE ORDER BY USERNO`
		if err := db.SelectContext(ctx, &employees, query); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// Debug mode: Show full PIN if ?debug=true
		showFullPin := r.URL.Query().Get("debug") == "true"

		// Return list with PIN masked (first 2 chars visible for debugging)
		result := make([]map[string]interface{}, len(employees))
		for i, emp := range employees {
			pinPreview := "****"
			if showFullPin {
				pinPreview = fmt.Sprintf("%q (len=%d)", emp.PIN, len(emp.PIN))
			} else if len(emp.PIN) > 0 {
				if len(emp.PIN) <= 2 {
					pinPreview = emp.PIN
				} else {
					pinPreview = emp.PIN[:2] + strings.Repeat("*", len(emp.PIN)-2)
				}
			}
			result[i] = map[string]interface{}{
				"user_no":      emp.UserNo,
				"first_name":   emp.FirstName.String,
				"last_name":    emp.LastName.String,
				"pin_preview":  pinPreview,
				"access_level": emp.AccessLevel.String,
			}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"count":     len(employees),
			"employees": result,
		})
	})

	// ADMIN ONLY: Update employee PIN
	http.HandleFunc("/api/v1/employees/update-pin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
			return
		}

		var req struct {
			UserNo int16  `json:"user_no"`
			NewPIN string `json:"new_pin"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request"})
			return
		}

		ctx := context.WithValue(r.Context(), middleware.TenantIDKey, "tenant_test_001")
		db, err := tm.GetDB(ctx)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		updateQuery := `UPDATE EMPLOYEE SET PIN = ? WHERE USERNO = ?`
		_, err = db.ExecContext(ctx, updateQuery, req.NewPIN, req.UserNo)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("PIN updated for UserNo %d", req.UserNo),
		})
	})

	http.Handle("/employee", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if r.Method == http.MethodGet {
			userNo, _ := middleware.GetUserNo(ctx)
			emp, err := empRepo.GetEmployee(ctx, userNo)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(emp)
		}
	})))

	// -------------------------------------------------------------------------
	// Bill endpoints
	// -------------------------------------------------------------------------

	// GET /bills?date=YYYY-MM-DD  — list all bills for a business day
	http.Handle("GET /bills", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			http.Error(w, `missing ?date parameter (YYYY-MM-DD)`, http.StatusBadRequest)
			return
		}
		day, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, "invalid date format, expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		bills, err := billRepo.ListBillsByBusinessDay(r.Context(), day)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if bills == nil {
			bills = []*models.Bill{} // never return null JSON array
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bills)
	})))

	// GET /bills/{checkno}  — fetch a single bill by PK
	http.Handle("GET /bills/{checkno}", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkNoStr := r.PathValue("checkno")
		checkNo64, err := strconv.ParseInt(checkNoStr, 10, 32)
		if err != nil {
			http.Error(w, "invalid check_no: must be an integer", http.StatusBadRequest)
			return
		}
		bill, err := billRepo.GetBill(r.Context(), int32(checkNo64))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bill)
	})))

	// POST /bills  — open a new bill (system assigns CHECKNO via GEN_ID)
	http.Handle("POST /bills", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.CreateBillRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		bill, err := billRepo.InsertBill(r.Context(), &req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bill)
	})))

	// PATCH /bills/{checkno}/close  — settle a bill; the money path
	http.Handle("PATCH /bills/{checkno}/close", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkNo64, err := strconv.ParseInt(r.PathValue("checkno"), 10, 32)
		if err != nil {
			http.Error(w, "invalid check_no: must be an integer", http.StatusBadRequest)
			return
		}
		var req models.CloseBillRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.ClosedBy == "" {
			http.Error(w, "closed_by is required", http.StatusBadRequest)
			return
		}
		bill, err := billRepo.CloseBill(r.Context(), int32(checkNo64), &req)
		if err != nil {
			// "not open" is a business rule rejection, not a server error
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bill)
	})))

	// PATCH /bills/{checkno}/cashup  — mark a closed bill as cashed-up
	http.Handle("PATCH /bills/{checkno}/cashup", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkNo64, err := strconv.ParseInt(r.PathValue("checkno"), 10, 32)
		if err != nil {
			http.Error(w, "invalid check_no: must be an integer", http.StatusBadRequest)
			return
		}
		bill, err := billRepo.CashUpBill(r.Context(), int32(checkNo64))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bill)
	})))

	// PATCH /bills/{checkno}/void  — void an open bill (no settlement)
	http.Handle("PATCH /bills/{checkno}/void", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkNo64, err := strconv.ParseInt(r.PathValue("checkno"), 10, 32)
		if err != nil {
			http.Error(w, "invalid check_no: must be an integer", http.StatusBadRequest)
			return
		}
		var req models.VoidBillRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.ClosedBy == "" {
			http.Error(w, "closed_by is required", http.StatusBadRequest)
			return
		}
		bill, err := billRepo.VoidBill(r.Context(), int32(checkNo64), &req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bill)
	})))

	// -------------------------------------------------------------------------
	// Purchase Order endpoints
	// -------------------------------------------------------------------------

	// Shared helpers — write JSON body with status code.
	writeJSON := func(w http.ResponseWriter, status int, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	writeErr := func(w http.ResponseWriter, status int, msg string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(models.APIError{Error: msg})
	}
	parseOrderNo := func(w http.ResponseWriter, r *http.Request) (int64, bool) {
		n, err := strconv.ParseInt(r.PathValue("order_no"), 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid order_no: must be an integer")
			return 0, false
		}
		return n, true
	}

	// GET /api/v1/purchase-orders?supplier_no={s}
	http.Handle("GET /api/v1/purchase-orders", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orders, err := poRepo.ListOrders(r.Context(), r.URL.Query().Get("supplier_no"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if orders == nil {
			orders = []models.OrderSummary{}
		}
		writeJSON(w, http.StatusOK, orders)
	})))

	// GET /api/v1/purchase-orders/{order_no}
	http.Handle("GET /api/v1/purchase-orders/{order_no}", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		order, err := poRepo.GetOrder(r.Context(), orderNo)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, order)
	})))

	// POST /api/v1/purchase-orders
	http.Handle("POST /api/v1/purchase-orders", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.CreateOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		if req.SupplierNo == "" {
			writeErr(w, http.StatusBadRequest, "supplier_no is required")
			return
		}
		order, err := poRepo.CreateOrder(r.Context(), &req)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, order)
	})))

	// DELETE /api/v1/purchase-orders/{order_no}
	http.Handle("DELETE /api/v1/purchase-orders/{order_no}", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		if err := poRepo.DeleteOrder(r.Context(), orderNo); err != nil {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	// GET /api/v1/purchase-orders/{order_no}/lines
	http.Handle("GET /api/v1/purchase-orders/{order_no}/lines", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		order, err := poRepo.GetOrder(r.Context(), orderNo)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		lines := order.Lines
		if lines == nil {
			lines = []models.OrderLineDetail{}
		}
		writeJSON(w, http.StatusOK, lines)
	})))

	// POST /api/v1/purchase-orders/{order_no}/lines
	http.Handle("POST /api/v1/purchase-orders/{order_no}/lines", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		var req models.AddLineItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		line, err := poRepo.AddLineItem(r.Context(), orderNo, &req)
		if err != nil {
			code := http.StatusInternalServerError
			if isNotFound(err) {
				code = http.StatusNotFound
			} else if isUnprocessable(err) {
				code = http.StatusUnprocessableEntity
			} else if isConflict(err) {
				code = http.StatusConflict
			}
			writeErr(w, code, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, line)
	})))

	// PUT /api/v1/purchase-orders/{order_no}/lines/{item_no}
	http.Handle("PUT /api/v1/purchase-orders/{order_no}/lines/{item_no}", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		itemNo, err := strconv.ParseInt(r.PathValue("item_no"), 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid item_no")
			return
		}
		var req models.UpdateLineItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		line, err := poRepo.UpdateLineItem(r.Context(), orderNo, itemNo, &req)
		if err != nil {
			code := http.StatusInternalServerError
			if isConflict(err) {
				code = http.StatusConflict
			} else if isNotFound(err) {
				code = http.StatusNotFound
			}
			writeErr(w, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, line)
	})))

	// DELETE /api/v1/purchase-orders/{order_no}/lines/{item_no}
	http.Handle("DELETE /api/v1/purchase-orders/{order_no}/lines/{item_no}", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		itemNo, err := strconv.ParseInt(r.PathValue("item_no"), 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid item_no")
			return
		}
		if err := poRepo.DeleteLineItem(r.Context(), orderNo, itemNo); err != nil {
			code := http.StatusConflict
			if isNotFound(err) {
				code = http.StatusNotFound
			}
			writeErr(w, code, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	// GET /api/v1/purchase-orders/{order_no}/totals
	http.Handle("GET /api/v1/purchase-orders/{order_no}/totals", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		totals, err := poRepo.GetOrderTotals(r.Context(), orderNo)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, totals)
	})))

	// POST /api/v1/purchase-orders/{order_no}/capture-invoice
	http.Handle("POST /api/v1/purchase-orders/{order_no}/capture-invoice", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		var req models.CaptureInvoiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		totals, err := poRepo.CaptureInvoice(r.Context(), orderNo, &req)
		if err != nil {
			code := http.StatusBadRequest
			if isConflict(err) {
				code = http.StatusConflict
			}
			writeErr(w, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, totals)
	})))

	// POST /api/v1/purchase-orders/{order_no}/post
	http.Handle("POST /api/v1/purchase-orders/{order_no}/post", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderNo, ok := parseOrderNo(w, r)
		if !ok {
			return
		}
		var req models.PostInvoiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		resp, err := poRepo.PostInvoice(r.Context(), orderNo, &req)
		if err != nil {
			code := http.StatusInternalServerError
			if isConflict(err) {
				code = http.StatusConflict
			} else if isUnprocessable(err) {
				code = http.StatusUnprocessableEntity
			}
			writeErr(w, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})))

	// PUT /api/v1/purchase-orders/{order_no}/update-costs
	http.Handle("PUT /api/v1/purchase-orders/{order_no}/update-costs", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.UpdateCostsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		resp, err := poRepo.UpdateInventoryCosts(r.Context(), &req)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})))

	// GET /api/v1/suppliers
	http.Handle("GET /api/v1/suppliers", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suppliers, err := poRepo.ListSuppliers(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if suppliers == nil {
			suppliers = []models.SupplierResponse{}
		}
		writeJSON(w, http.StatusOK, suppliers)
	})))

	// GET /api/v1/suppliers/{supplier_no}/items
	http.Handle("GET /api/v1/suppliers/{supplier_no}/items", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sNo := r.PathValue("supplier_no")
		items, err := poRepo.GetSupplierItems(r.Context(), sNo)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if items == nil {
			items = []models.SupplierItem{}
		}
		writeJSON(w, http.StatusOK, items)
	})))

	// GET /api/v1/inventory/search?mpart_no={p}
	http.Handle("GET /api/v1/inventory/search", middleware.AuthMiddleware(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mPartNo := r.URL.Query().Get("mpart_no")
		if mPartNo == "" {
			writeErr(w, http.StatusBadRequest, "missing ?mpart_no parameter")
			return
		}
		item, err := poRepo.SearchInventoryItem(r.Context(), mPartNo)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	})))

	// Register inventory routes (all 24 endpoints)
	invHandler := handler.NewInventoryHandler(invRepo, groupRepo, luRepo, stRepo, grvRepo, wastRepo)
	handler.RegisterInventoryRoutes(http.DefaultServeMux, invHandler, groupRepo, jwtSecret)

	// Register dashboard routes
	dashHandler := handler.NewDashboardHandler(dashRepo)
	handler.RegisterDashboardRoutes(http.DefaultServeMux, dashHandler, jwtSecret)

	// Register settings routes
	settingsHandler := handler.NewSettingsHandler(settingsRepo)
	handler.RegisterSettingsRoutes(http.DefaultServeMux, settingsHandler, jwtSecret)

	// Register sales menu routes
	salesMenuHandler := handler.NewSalesMenuHandler(salesMenuRepo)
	handler.RegisterSalesMenuRoutes(http.DefaultServeMux, salesMenuHandler, jwtSecret)

	freePort(cfg.Port)
	log.Printf("Aweh POS Gateway starting on :%s...", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// gatewayProcessNames is the set of executables we are allowed to kill when
// freeing our listen port.  We never touch system processes like svchost.
var gatewayProcessNames = map[string]bool{
	"pos-gateway.exe": true,
	"pos-gateway":     true,
	"go.exe":          true, // `go run` dev mode
	"go":              true,
}

// freePort kills any *gateway* process currently bound to the given TCP port so
// the gateway can always start cleanly after a crash. It deliberately refuses to
// kill system processes (svchost, etc.).
func freePort(port string) {
	switch runtime.GOOS {
	case "windows":
		// netstat -ano prints lines like:
		//   TCP  0.0.0.0:8080  0.0.0.0:0  LISTENING  <pid>
		out, err := exec.Command("netstat", "-ano").Output()
		if err != nil {
			return
		}
		target := ":" + port
		seen := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, target) || !strings.Contains(line, "LISTENING") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			pid := fields[len(fields)-1]
			if pid == "0" || seen[pid] {
				continue
			}
			seen[pid] = true

			// Resolve the process name — only kill our own gateway binary.
			nameOut, err := exec.Command("powershell", "-NoProfile", "-Command",
				"(Get-Process -Id "+pid+" -ErrorAction SilentlyContinue).Name").Output()
			if err != nil {
				continue
			}
			name := strings.TrimSpace(strings.ToLower(string(nameOut))) + ".exe"
			if !gatewayProcessNames[name] {
				log.Printf("freePort: PID %s (%s) is not a gateway process — skipping", pid, name)
				continue
			}
			log.Printf("freePort: port %s held by gateway PID %s (%s) — killing...", port, pid, name)
			exec.Command("taskkill", "/PID", pid, "/F").Run() //nolint:errcheck
			time.Sleep(400 * time.Millisecond)
		}
	default:
		// Linux / macOS — pkill is safer than fuser because it matches by name.
		exec.Command("pkill", "-f", "pos-gateway").Run() //nolint:errcheck
		time.Sleep(300 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// Error classification helpers — read error message strings from the repo.
// These avoid importing a custom error type; matches the simple error wrapping
// pattern used in this codebase.
// ---------------------------------------------------------------------------

func isNotFound(err error) bool { return containsAny(err, "not found") }
func isConflict(err error) bool {
	return containsAny(err, "already posted", "already exists", "cannot delete", "cannot void")
}
func isUnprocessable(err error) bool {
	return containsAny(err, "zero pack size", "pay reference required")
}

func containsAny(err error, phrases ...string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, p := range phrases {
		if len(msg) >= len(p) {
			for i := 0; i <= len(msg)-len(p); i++ {
				if msg[i:i+len(p)] == p {
					return true
				}
			}
		}
	}
	return false
}
