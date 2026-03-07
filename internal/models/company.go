package models

// Company represents the COMPANYINFO table in Firebird.
// NOTE: LOGO_PATH requires: ALTER TABLE COMPANYINFO ADD LOGO_PATH VARCHAR(255);
type Company struct {
	CompanyNo   int32      `db:"COMPANYNO"   json:"company_no"`
	CompanyName NullString `db:"COMPANYNAME"  json:"company_name"`
	BranchName  NullString `db:"BRANCHNAME"   json:"branch_name"`
	Address     NullString `db:"ADDRESS"      json:"address"`
	City        NullString `db:"CITY"         json:"city"`
	VatNo       NullString `db:"VATNO"        json:"vat_no"`
	PhoneNo     NullString `db:"PHONENO"      json:"phone_no"`
	FaxNo       NullString `db:"FAXNO"        json:"fax_no"`
	Email       NullString `db:"EMAILADDRESS" json:"email"`
	LogoPath    NullString `db:"LOGO_PATH"    json:"logo_path"`
}
