package db

import (
	"context"
	"database/sql"
	"fmt"
)

// organizeAppManagementMenus keeps all App-facing administration entries
// under the deployment-owned App 管理 catalog while preserving their URLs.
func organizeAppManagementMenus(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var parentID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM menus
		WHERE name='AppManage' AND path='/app' AND status=1`).Scan(&parentID); err != nil {
		return fmt.Errorf("locate App 管理 menu: %w", err)
	}

	menuOrder := []struct {
		name string
		sort int
	}{
		{"DashboardAppAnalytics", 1},
		{"CustomerAppUsers", 2},
		{"CustomerUserInsights", 3},
		{"CustomerAppOrders", 4},
		{"AppProducts", 5},
		{"WebsiteAppReleases", 6},
		{"CustomerAppChat", 7},
		{"CustomerAppMemory", 8},
		{"CustomerQuizQuestions", 9},
		{"AppEnneagramLibrary", 10},
	}

	for _, item := range menuOrder {
		if _, err := tx.ExecContext(ctx, `
			UPDATE menus SET pid=$1, sort=$2
			WHERE name=$3 AND type='menu'`, parentID, item.sort, item.name); err != nil {
			return err
		}
	}

	// A role that could see a moved child must also be able to see its new catalog.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO role_menus(role_id, menu_id)
		SELECT DISTINCT rm.role_id, $1::bigint
		FROM role_menus rm
		JOIN menus m ON m.id=rm.menu_id
		WHERE m.pid=$1::bigint
		ON CONFLICT (role_id,menu_id) DO NOTHING`, parentID); err != nil {
		return err
	}

	return tx.Commit()
}
