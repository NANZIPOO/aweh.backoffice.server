package repository

import (
	"context"
	"fmt"

	"github.com/aweh-pos/gateway/internal/models"
)

type CompanyRepository struct {
	BaseRepository
}

func NewCompanyRepository(tm *TenantManager) *CompanyRepository {
	return &CompanyRepository{BaseRepository{TM: tm}}
}

func (r *CompanyRepository) GetCompany(ctx context.Context) (*models.Company, error) {
	db, err := r.TM.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	const query = `
		SELECT
			COMPANYNO, COMPANYNAME, BRANCHNAME, ADDRESS, CITY,
			VATNO, PHONENO, FAXNO, EMAILADDRESS
		FROM COMPANYINFO
		ROWS 1`

	var comp models.Company
	if err := db.GetContext(ctx, &comp, query); err != nil {
		return nil, fmt.Errorf("failed to fetch company: %w", err)
	}

	return &comp, nil
}
