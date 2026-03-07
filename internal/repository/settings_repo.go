package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aweh-pos/gateway/internal/models"
	"gopkg.in/ini.v1"
)

type SettingsRepository struct {
	BaseRepository
	iniPath string
}

func NewSettingsRepository(tm *TenantManager, iniPath string) *SettingsRepository {
	// Default to user's Documents/gateway_settings.ini if not specified
	if iniPath == "" {
		home, _ := os.UserHomeDir()
		iniPath = filepath.Join(home, "Documents", "gateway_settings.ini")
	}
	return &SettingsRepository{
		BaseRepository: BaseRepository{TM: tm},
		iniPath:        iniPath,
	}
}

// ensureINIFile creates the INI file with defaults if it doesn't exist
func (r *SettingsRepository) ensureINIFile() (*ini.File, error) {
	cfg, err := ini.Load(r.iniPath)
	if err != nil {
		// File doesn't exist, create with defaults
		cfg = ini.Empty()
		
		// Create SYSPARAMS section with defaults
		sec, _ := cfg.NewSection("SYSPARAMS")
		sec.NewKey("APPSERVER", "127.0.0.1")
		sec.NewKey("AppDatabase", "dinem.fdb")
		sec.NewKey("DocsPath", "C:/Aweh/Docs")
		sec.NewKey("BackUpServerPath", "C:/Aweh/Backup")
		sec.NewKey("Addressline1", "")
		sec.NewKey("Addressline2", "")
		sec.NewKey("Addressline3", "")
		sec.NewKey("Phone_No", "")
		sec.NewKey("Vat_No", "")
		sec.NewKey("HEADER1", "WELCOME")
		sec.NewKey("HEADER2", "")
		sec.NewKey("FOOTER1", "THANK YOU")
		sec.NewKey("FOOTER2", "")
		sec.NewKey("VAT", "15.0")
		sec.NewKey("VisaMCom", "0.0")
		sec.NewKey("MasterMCom", "0.0")
		sec.NewKey("CashupType", "STANDARD")
		sec.NewKey("EmailReports", "0")
		sec.NewKey("UserMode", "POS")
		sec.NewKey("AppKey", "1234")
		sec.NewKey("MultiBar", "0")
		sec.NewKey("STKMID", "STK-001")
		sec.NewKey("PRINTERPORT", "COM1")
		sec.NewKey("CashupReport", "1")
		
		// Ensure directory exists
		dir := filepath.Dir(r.iniPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create settings directory: %w", err)
		}
		
		if err := cfg.SaveTo(r.iniPath); err != nil {
			return nil, fmt.Errorf("failed to save default settings: %w", err)
		}
	}
	return cfg, nil
}

// GetCoreSetup reads database and path settings from INI
func (r *SettingsRepository) GetCoreSetup(ctx context.Context) (*models.CoreSetupSettings, error) {
	cfg, err := r.ensureINIFile()
	if err != nil {
		return nil, err
	}
	
	sec := cfg.Section("SYSPARAMS")
	return &models.CoreSetupSettings{
		AppServer:   sec.Key("APPSERVER").String(),
		AppDatabase: sec.Key("AppDatabase").String(),
		DocsPath:    sec.Key("DocsPath").String(),
		BackupPath:  sec.Key("BackUpServerPath").String(),
	}, nil
}

// SaveCoreSetup writes database and path settings to INI
func (r *SettingsRepository) SaveCoreSetup(ctx context.Context, settings *models.CoreSetupSettings) error {
	cfg, err := r.ensureINIFile()
	if err != nil {
		return err
	}
	
	sec := cfg.Section("SYSPARAMS")
	sec.Key("APPSERVER").SetValue(settings.AppServer)
	sec.Key("AppDatabase").SetValue(settings.AppDatabase)
	sec.Key("DocsPath").SetValue(settings.DocsPath)
	sec.Key("BackUpServerPath").SetValue(settings.BackupPath)
	
	return cfg.SaveTo(r.iniPath)
}

// GetBusinessProfile reads company info (DB + INI hybrid)
func (r *SettingsRepository) GetBusinessProfile(ctx context.Context) (*models.BusinessProfileSettings, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}
	
	// Read INI fields
	cfg, err := r.ensureINIFile()
	if err != nil {
		return nil, err
	}
	sec := cfg.Section("SYSPARAMS")
	
	// Read company name and branch from DB (COMPANYINFO table)
	const query = `SELECT COMPANYNAME, BRANCHNAME FROM COMPANYINFO ROWS 1`
	var dbData struct {
		CompanyName models.NullString `db:"COMPANYNAME"`
		BranchName  models.NullString `db:"BRANCHNAME"`
	}
	
	profile := &models.BusinessProfileSettings{
		Address1: sec.Key("Addressline1").String(),
		Address2: sec.Key("Addressline2").String(),
		Address3: sec.Key("Addressline3").String(),
		Phone:    sec.Key("Phone_No").String(),
		VatNo:    sec.Key("Vat_No").String(),
		Header1:  sec.Key("HEADER1").String(),
		Header2:  sec.Key("HEADER2").String(),
		Footer1:  sec.Key("FOOTER1").String(),
		Footer2:  sec.Key("FOOTER2").String(),
	}
	
	// Try to fetch from DB, but don't fail if table is empty
	if err := db.GetContext(ctx, &dbData, query); err == nil {
		profile.CompanyName = dbData.CompanyName.String
		profile.BranchName = dbData.BranchName.String
	}
	
	return profile, nil
}

// SaveBusinessProfile writes company info to both DB and INI
func (r *SettingsRepository) SaveBusinessProfile(ctx context.Context, settings *models.BusinessProfileSettings) error {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return err
	}
	
	// 1. Update INI file
	cfg, err := r.ensureINIFile()
	if err != nil {
		return err
	}
	sec := cfg.Section("SYSPARAMS")
	sec.Key("Addressline1").SetValue(settings.Address1)
	sec.Key("Addressline2").SetValue(settings.Address2)
	sec.Key("Addressline3").SetValue(settings.Address3)
	sec.Key("Phone_No").SetValue(settings.Phone)
	sec.Key("Vat_No").SetValue(settings.VatNo)
	sec.Key("HEADER1").SetValue(settings.Header1)
	sec.Key("HEADER2").SetValue(settings.Header2)
	sec.Key("FOOTER1").SetValue(settings.Footer1)
	sec.Key("FOOTER2").SetValue(settings.Footer2)
	
	if err := cfg.SaveTo(r.iniPath); err != nil {
		return err
	}
	
	// 2. Update DB (COMPANYINFO table)
	const update = `
		UPDATE COMPANYINFO 
		SET COMPANYNAME = ?, BRANCHNAME = ?
		WHERE COMPANYNO = (SELECT FIRST 1 COMPANYNO FROM COMPANYINFO)`
	
	_, err = db.ExecContext(ctx, update, settings.CompanyName, settings.BranchName)
	return err
}

// GetFinancialControl reads tax and commission settings from INI
func (r *SettingsRepository) GetFinancialControl(ctx context.Context) (*models.FinancialControlSettings, error) {
	cfg, err := r.ensureINIFile()
	if err != nil {
		return nil, err
	}
	
	sec := cfg.Section("SYSPARAMS")
	vat, _ := strconv.ParseFloat(sec.Key("VAT").String(), 64)
	visa, _ := strconv.ParseFloat(sec.Key("VisaMCom").String(), 64)
	master, _ := strconv.ParseFloat(sec.Key("MasterMCom").String(), 64)
	emailReports := sec.Key("EmailReports").MustInt() == 1
	
	return &models.FinancialControlSettings{
		VATRate:          vat,
		VisaCommission:   visa,
		MasterCommission: master,
		CashupType:       sec.Key("CashupType").String(),
		EmailReports:     emailReports,
	}, nil
}

// SaveFinancialControl writes tax and commission settings to INI
func (r *SettingsRepository) SaveFinancialControl(ctx context.Context, settings *models.FinancialControlSettings) error {
	cfg, err := r.ensureINIFile()
	if err != nil {
		return err
	}
	
	sec := cfg.Section("SYSPARAMS")
	sec.Key("VAT").SetValue(fmt.Sprintf("%.2f", settings.VATRate))
	sec.Key("VisaMCom").SetValue(fmt.Sprintf("%.2f", settings.VisaCommission))
	sec.Key("MasterMCom").SetValue(fmt.Sprintf("%.2f", settings.MasterCommission))
	sec.Key("CashupType").SetValue(settings.CashupType)
	
	emailVal := "0"
	if settings.EmailReports {
		emailVal = "1"
	}
	sec.Key("EmailReports").SetValue(emailVal)
	
	return cfg.SaveTo(r.iniPath)
}

// GetSecurityAccess reads security settings from INI (AppKey is masked)
func (r *SettingsRepository) GetSecurityAccess(ctx context.Context) (*models.SecurityAccessSettings, error) {
	cfg, err := r.ensureINIFile()
	if err != nil {
		return nil, err
	}
	
	sec := cfg.Section("SYSPARAMS")
	multiBar := sec.Key("MultiBar").MustInt() == 1
	
	// Never return actual AppKey, only a hint
	appKey := sec.Key("AppKey").String()
	hint := strings.Repeat("•", len(appKey))
	
	return &models.SecurityAccessSettings{
		UserMode:   sec.Key("UserMode").String(),
		AppKeyHint: hint,
		MultiBar:   multiBar,
	}, nil
}

// SaveSecurityAccess writes security settings to INI (excluding AppKey)
func (r *SettingsRepository) SaveSecurityAccess(ctx context.Context, settings *models.SecurityAccessSettings) error {
	cfg, err := r.ensureINIFile()
	if err != nil {
		return err
	}
	
	sec := cfg.Section("SYSPARAMS")
	sec.Key("UserMode").SetValue(settings.UserMode)
	
	multiBarVal := "0"
	if settings.MultiBar {
		multiBarVal = "1"
	}
	sec.Key("MultiBar").SetValue(multiBarVal)
	
	return cfg.SaveTo(r.iniPath)
}

// ChangeAppKey validates current key and updates to new key
func (r *SettingsRepository) ChangeAppKey(ctx context.Context, currentKey, newKey string) error {
	if len(newKey) < 4 {
		return fmt.Errorf("new AppKey must be at least 4 characters")
	}
	
	cfg, err := r.ensureINIFile()
	if err != nil {
		return err
	}
	
	sec := cfg.Section("SYSPARAMS")
	storedKey := sec.Key("AppKey").String()
	
	if storedKey != currentKey {
		return fmt.Errorf("current AppKey is incorrect")
	}
	
	sec.Key("AppKey").SetValue(newKey)
	return cfg.SaveTo(r.iniPath)
}

// GetDeviceTerminal reads hardware and terminal settings from INI
func (r *SettingsRepository) GetDeviceTerminal(ctx context.Context) (*models.DeviceTerminalSettings, error) {
	cfg, err := r.ensureINIFile()
	if err != nil {
		return nil, err
	}
	
	sec := cfg.Section("SYSPARAMS")
	cashupReport := sec.Key("CashupReport").MustInt() == 1
	
	return &models.DeviceTerminalSettings{
		StockMachineID:    sec.Key("STKMID").String(),
		PrinterPort:       sec.Key("PRINTERPORT").String(),
		CashupReport:      cashupReport,
		AnimationsEnabled: true, // Default, not stored in INI
	}, nil
}

// SaveDeviceTerminal writes hardware and terminal settings to INI
func (r *SettingsRepository) SaveDeviceTerminal(ctx context.Context, settings *models.DeviceTerminalSettings) error {
	cfg, err := r.ensureINIFile()
	if err != nil {
		return err
	}
	
	sec := cfg.Section("SYSPARAMS")
	sec.Key("STKMID").SetValue(settings.StockMachineID)
	sec.Key("PRINTERPORT").SetValue(settings.PrinterPort)
	
	cashupVal := "0"
	if settings.CashupReport {
		cashupVal = "1"
	}
	sec.Key("CashupReport").SetValue(cashupVal)
	
	// AnimationsEnabled is local Flutter preference, not persisted to INI
	
	return cfg.SaveTo(r.iniPath)
}
