package models

type SalesMenuGroup struct {
	ID         string `db:"ID" json:"id"`
	Label      string `db:"LABEL" json:"label"`
	Department string `db:"DEPARTMENT" json:"department"`
	ButtonPos  int    `db:"BUTTONPOS" json:"button_pos"`
}

type SalesMenuItem struct {
	ID            int64   `db:"ID" json:"id"`
	GroupID       string  `db:"GROUP_ID" json:"group_id"`
	Label         string  `db:"LABEL" json:"label"`
	Price         float64 `db:"PRICE" json:"price"`
	CostPrice     float64 `db:"COST_PRICE" json:"cost_price"`
	SalesTax      float64 `db:"SALES_TAX" json:"sales_tax"`
	Barcode       string  `db:"BARCODE" json:"barcode"`
	MPartNo       string  `db:"MPARTNO" json:"m_part_no"`
	PartNo        string  `db:"PARTNO" json:"part_no"`
	ItemType      string  `db:"ITEM_TYPE" json:"item_type"`
	CategoryFlag  string  `db:"CATEGORY_FLAG" json:"category_flag"`
	SpeedScreen   string  `db:"SPEED_SCREEN" json:"speed_screen"`
	ForcedPopup   string  `db:"FORCED_POPUP" json:"forced_popup"`
	AllowDiscount string  `db:"ALLOW_DISCOUNT" json:"allow_discount"`
	Enabled       bool    `db:"-" json:"enabled"`
}
