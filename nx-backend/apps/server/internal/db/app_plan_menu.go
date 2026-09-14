package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

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
	err = tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE path='/app/plan-management' ORDER BY id LIMIT 1`).Scan(&pageID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `INSERT INTO menus(pid,name,path,component,auth_code,type,status,sort,meta)
			VALUES($1,'AppPlanManagement','/app/plan-management','/app/plan-management','App:PlanManagement:View','menu',1,12,$2::jsonb) RETURNING id`, parentID, meta).Scan(&pageID)
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE menus SET pid=$2,name='AppPlanManagement',component='/app/plan-management',auth_code='App:PlanManagement:View',type='menu',status=1,sort=12,meta=$3::jsonb WHERE id=$1`, pageID, parentID, meta)
	}
	if err != nil {
		return err
	}
	buttonMeta, _ := json.Marshal(map[string]any{"icon": "lucide:pencil", "title": "编辑套餐"})
	if _, err := tx.ExecContext(ctx, `INSERT INTO menus(pid,name,auth_code,type,status,sort,meta)
		SELECT $1,'AppPlanManagementWrite','App:PlanManagement:Write','button',1,1,$2::jsonb
		WHERE NOT EXISTS (SELECT 1 FROM menus WHERE pid=$1 AND auth_code='App:PlanManagement:Write' AND type='button')`, pageID, buttonMeta); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO role_menus(role_id,menu_id)
		SELECT role.id,menu.id FROM roles role CROSS JOIN menus menu
		WHERE role.code='admin' AND (menu.id=$1 OR menu.pid=$1)
		ON CONFLICT(role_id,menu_id) DO NOTHING`, pageID); err != nil {
		return err
	}
	return tx.Commit()
}
