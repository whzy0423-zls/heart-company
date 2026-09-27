package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	appEmailMenuID         int64 = 1623
	appEmailManageMenuID   int64 = 1624
	appEmailMenuPath             = "/app/email-config"
	appEmailMenuComponent        = "/app/email-config"
	appEmailMenuName             = "AppEmailConfig"
	appEmailManageMenuName       = "AppEmailConfigManage"
	appEmailAuthCode             = "App:Email:View"
	appEmailManageAuthCode       = "App:Email:Manage"
	appEmailMigrationKey         = "seed.app_email_menu.v1"
)

func seedAppEmailMenu(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var parentID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE name='AppManage' AND path='/app' AND status=1`).Scan(&parentID); err != nil {
		return fmt.Errorf("find App 管理 menu: %w", err)
	}
	meta, _ := json.Marshal(map[string]any{"icon": "lucide:mail-cog", "title": "邮箱与 SMTP"})
	var menuID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE path=$1 OR name=$2 ORDER BY CASE WHEN id=$3 THEN 0 ELSE 1 END,id LIMIT 1`, appEmailMenuPath, appEmailMenuName, appEmailMenuID).Scan(&menuID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := tx.QueryRowContext(ctx, `INSERT INTO menus(id,pid,name,path,component,auth_code,type,status,sort,meta) VALUES($1,$2,$3,$4,$5,$6,'menu',1,19,$7::jsonb) RETURNING id`, appEmailMenuID, parentID, appEmailMenuName, appEmailMenuPath, appEmailMenuComponent, appEmailAuthCode, string(meta)).Scan(&menuID); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE menus SET pid=$2,name=$3,path=$4,component=$5,auth_code=$6,type='menu',status=1,sort=19,meta=$7::jsonb WHERE id=$1`, menuID, parentID, appEmailMenuName, appEmailMenuPath, appEmailMenuComponent, appEmailAuthCode, string(meta)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM menus WHERE id<>$1 AND (path=$2 OR name=$3)`, menuID, appEmailMenuPath, appEmailMenuName); err != nil {
		return err
	}
	manageMeta, _ := json.Marshal(map[string]any{"icon": "lucide:save", "title": "保存邮箱配置"})
	var manageMenuID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE id=$1 OR name=$2 ORDER BY CASE WHEN id=$1 THEN 0 ELSE 1 END,id LIMIT 1`, appEmailManageMenuID, appEmailManageMenuName).Scan(&manageMenuID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := tx.QueryRowContext(ctx, `INSERT INTO menus(id,pid,name,path,component,auth_code,type,status,sort,meta) VALUES($1,$2,$3,'','',$4,'button',1,1,$5::jsonb) RETURNING id`, appEmailManageMenuID, menuID, appEmailManageMenuName, appEmailManageAuthCode, string(manageMeta)).Scan(&manageMenuID); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE menus SET pid=$2,name=$3,path='',component='',auth_code=$4,type='button',status=1,sort=1,meta=$5::jsonb WHERE id=$1`, manageMenuID, menuID, appEmailManageMenuName, appEmailManageAuthCode, string(manageMeta)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO migration_logs(key,detail) VALUES($1,$2::jsonb) ON CONFLICT(key) DO NOTHING`, appEmailMigrationKey, `{"description":"挂载 App 邮箱与 SMTP 配置"}`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO role_menus(role_id,menu_id) SELECT role.id,$1::bigint FROM roles role WHERE role.code='admin' ON CONFLICT(role_id,menu_id) DO NOTHING`, menuID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO role_menus(role_id,menu_id) SELECT role.id,$1::bigint FROM roles role WHERE role.code='admin' ON CONFLICT(role_id,menu_id) DO NOTHING`, manageMenuID); err != nil {
		return err
	}
	return tx.Commit()
}
