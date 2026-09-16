package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const (
	appPlanManagementMenuID          int64 = 1614
	appPlanManagementWriteMenuID     int64 = 1619
	appPlanManagementPath                  = "/app/plan-management"
	appPlanManagementComponent             = "/app/plan-management"
	appPlanManagementViewPermission        = "App:PlanManagement:View"
	appPlanManagementWritePermission       = "App:PlanManagement:Write"
)

const appPlanManagementMenuBindingSQL = `
INSERT INTO role_menus(role_id,menu_id)
SELECT DISTINCT source.role_id,target.id
FROM role_menus source
JOIN menus source_menu ON source_menu.id=source.menu_id
JOIN menus target ON target.name='AppPlanManagement'
WHERE source_menu.name IN ('AppManage','CustomerAppOrders','AppPlanManagement')
   OR source_menu.auth_code IN ('Customer:AppOrders:List','App:PlanManagement:View')
ON CONFLICT(role_id,menu_id) DO NOTHING;

INSERT INTO role_menus(role_id,menu_id)
SELECT DISTINCT source.role_id,target.id
FROM role_menus source
JOIN menus source_menu ON source_menu.id=source.menu_id
JOIN menus target ON target.name='AppPlanManagementWrite'
WHERE source_menu.name IN ('CustomerAppOrdersGrant','AppPlanManagementWrite')
   OR source_menu.auth_code IN ('Customer:AppOrders:Write','App:PlanManagement:Write')
ON CONFLICT(role_id,menu_id) DO NOTHING;
`

func seedAppPlanManagementMenu(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var parentID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE name='AppManage' AND path='/app' AND status=1`).Scan(&parentID); err != nil {
		return fmt.Errorf("locate App 管理 menu: %w", err)
	}
	meta, _ := json.Marshal(map[string]any{"icon": "lucide:badge-dollar-sign", "title": "套餐管理"})
	var pageID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE id=$1::bigint OR path=$2 ORDER BY CASE WHEN id=$1::bigint THEN 0 ELSE 1 END, id LIMIT 1`, appPlanManagementMenuID, appPlanManagementPath).Scan(&pageID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `INSERT INTO menus(id,pid,name,path,component,auth_code,type,status,sort,meta)
			VALUES($1::bigint,$2::bigint,'AppPlanManagement',$3,$4,$5,'menu',1,12,$6::jsonb)
			ON CONFLICT(id) DO UPDATE
			SET pid=EXCLUDED.pid,name=EXCLUDED.name,path=EXCLUDED.path,component=EXCLUDED.component,auth_code=EXCLUDED.auth_code,type=EXCLUDED.type,status=1,sort=EXCLUDED.sort,meta=EXCLUDED.meta
			RETURNING id`, appPlanManagementMenuID, parentID, appPlanManagementPath, appPlanManagementComponent, appPlanManagementViewPermission, meta).Scan(&pageID)
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE menus SET pid=$2::bigint,name='AppPlanManagement',path=$3,component=$4,auth_code=$5,type='menu',status=1,sort=12,meta=$6::jsonb WHERE id=$1::bigint`, pageID, parentID, appPlanManagementPath, appPlanManagementComponent, appPlanManagementViewPermission, meta)
	}
	if err != nil {
		return err
	}
	buttonMeta, _ := json.Marshal(map[string]any{"icon": "lucide:pencil", "title": "编辑套餐"})
	if _, err := tx.ExecContext(ctx, `INSERT INTO menus(id,pid,name,auth_code,type,status,sort,meta)
		VALUES($1::bigint,$2::bigint,'AppPlanManagementWrite',$3,'button',1,1,$4::jsonb)
		ON CONFLICT(id) DO UPDATE
		SET pid=EXCLUDED.pid,name=EXCLUDED.name,auth_code=EXCLUDED.auth_code,type='button',status=1,sort=EXCLUDED.sort,meta=EXCLUDED.meta`,
		appPlanManagementWriteMenuID, pageID, appPlanManagementWritePermission, buttonMeta); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO role_menus(role_id,menu_id)
		SELECT DISTINCT rm.role_id,$1::bigint
		FROM role_menus rm
		JOIN menus m ON m.id=rm.menu_id
		WHERE m.id<>$1::bigint AND (m.name='AppPlanManagement' OR m.path=$2)
		ON CONFLICT(role_id,menu_id) DO NOTHING`, pageID, appPlanManagementPath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO role_menus(role_id,menu_id)
		SELECT DISTINCT rm.role_id,$1::bigint
		FROM role_menus rm
		JOIN menus m ON m.id=rm.menu_id
		WHERE m.id<>$1::bigint AND m.type='button' AND (m.name='AppPlanManagementWrite' OR m.auth_code=$2)
		ON CONFLICT(role_id,menu_id) DO NOTHING`, appPlanManagementWriteMenuID, appPlanManagementWritePermission); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM menus
		WHERE id<>$1::bigint AND (name='AppPlanManagement' OR path=$2)`, pageID, appPlanManagementPath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM menus
		WHERE id<>$1::bigint AND type='button' AND (name='AppPlanManagementWrite' OR auth_code=$2)`, appPlanManagementWriteMenuID, appPlanManagementWritePermission); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, appPlanManagementMenuBindingSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO role_menus(role_id,menu_id)
		SELECT role.id,menu.id FROM roles role CROSS JOIN menus menu
		WHERE role.code='admin' AND (menu.id=$1::bigint OR menu.pid=$1::bigint)
		ON CONFLICT(role_id,menu_id) DO NOTHING`, pageID); err != nil {
		return err
	}
	return tx.Commit()
}
