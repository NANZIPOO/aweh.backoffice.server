package repository

import (
	"context"

	"github.com/aweh-pos/gateway/internal/models"
)

type SalesMenuRepository interface {
	// Read operations
	GetGroups(ctx context.Context) ([]models.SalesMenuGroup, error)
	GetItems(ctx context.Context, groupID string) ([]models.SalesMenuItem, error)
	GetItem(ctx context.Context, itemID int64) (*models.SalesMenuItem, error)

	// Write operations - Groups
	CreateGroup(ctx context.Context, group *models.SalesMenuGroup) error
	UpdateGroup(ctx context.Context, id string, group *models.SalesMenuGroup) error
	DeleteGroup(ctx context.Context, id string) error

	// Write operations - Items
	CreateItem(ctx context.Context, item *models.SalesMenuItem) (int64, error)
	UpdateItem(ctx context.Context, id int64, item *models.SalesMenuItem) error
	DeleteItem(ctx context.Context, id int64) error
}
