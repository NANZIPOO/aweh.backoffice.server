package models

// CoreSetupSettings combines database connection and core paths (Sub-Module 1 + 4 merged)
type CoreSetupSettings struct {
	AppServer  string `json:"app_server" ini:"APPSERVER"`
	AppDatabase string `json:"app_database" ini:"AppDatabase"`
	DocsPath   string `json:"docs_path" ini:"DocsPath"`
	BackupPath string `json:"backup_path" ini:"BackUpServerPath"`
}

// BusinessProfileSettings combines company info and receipt templates (Sub-Module 2 + 3 merged)
type BusinessProfileSettings struct {
	CompanyName string `json:"company_name" db:"COMPANYNAME"`
	BranchName  string `json:"branch_name" db:"BRANCHNAME"`
	Address1    string `json:"address1" ini:"Addressline1"`
	Address2    string `json:"address2" ini:"Addressline2"`
	Address3    string `json:"address3" ini:"Addressline3"`
	Phone       string `json:"phone" ini:"Phone_No"`
	VatNo       string `json:"vat_no" ini:"Vat_No"`
	Header1     string `json:"header1" ini:"HEADER1"`
	Header2     string `json:"header2" ini:"HEADER2"`
	Footer1     string `json:"footer1" ini:"FOOTER1"`
	Footer2     string `json:"footer2" ini:"FOOTER2"`
}

// FinancialControlSettings combines tax/commission rates and cashup settings (Sub-Module 5 + 8 partial)
type FinancialControlSettings struct {
	VATRate          float64 `json:"vat_rate" ini:"VAT"`
	VisaCommission   float64 `json:"visa_commission" ini:"VisaMCom"`
	MasterCommission float64 `json:"master_commission" ini:"MasterMCom"`
	CashupType       string  `json:"cashup_type" ini:"CashupType"`
	EmailReports     bool    `json:"email_reports" ini:"EmailReports"`
}

// SecurityAccessSettings combines security and user mode (Sub-Module 6)
type SecurityAccessSettings struct {
	UserMode    string `json:"user_mode" ini:"UserMode"`
	AppKeyHint  string `json:"app_key_hint"` // Never return actual AppKey, only hint like "••••"
	MultiBar    bool   `json:"multi_bar" ini:"MultiBar"`
}

// DeviceTerminalSettings combines hardware and operations (Sub-Module 7 + 8 partial)
type DeviceTerminalSettings struct {
	StockMachineID    string `json:"stock_machine_id" ini:"STKMID"`
	PrinterPort       string `json:"printer_port" ini:"PRINTERPORT"`
	CashupReport      bool   `json:"cashup_report" ini:"CashupReport"`
	AnimationsEnabled bool   `json:"animations_enabled"` // Local Flutter preference, not INI
}

// SettingsBundle groups all 5 setting sections for bulk operations
type SettingsBundle struct {
	CoreSetup        CoreSetupSettings        `json:"core_setup"`
	BusinessProfile  BusinessProfileSettings  `json:"business_profile"`
	FinancialControl FinancialControlSettings `json:"financial_control"`
	SecurityAccess   SecurityAccessSettings   `json:"security_access"`
	DeviceTerminal   DeviceTerminalSettings   `json:"device_terminal"`
}

// AppKeyChangeRequest handles admin override key updates
type AppKeyChangeRequest struct {
	CurrentKey string `json:"current_key"`
	NewKey     string `json:"new_key"`
}
