package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const (
	skillLibraryManagementPath           = "/app/skill-library"
	skillLibraryManagementComponent      = "/app/skill-library-management"
	skillLibraryManagementViewPermission = "App:SkillLibrary:View"
	skillLibraryManagementEditPermission = "App:SkillLibrary:Edit"
)

func seedSkillLibraryManagementMenu(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var parentID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM menus WHERE name='AppManage' AND path='/app' AND status=1").Scan(&parentID); err != nil {
		return fmt.Errorf("locate App 管理 menu: %w", err)
	}
	meta, _ := json.Marshal(map[string]any{"icon": "lucide:library-big", "title": "成长技能库"})
	var pageID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM menus WHERE path=$1 ORDER BY id LIMIT 1", skillLibraryManagementPath).Scan(&pageID)
	if err == sql.ErrNoRows {
		query := "INSERT INTO menus(pid,name,path,component,auth_code,type,status,sort,meta) VALUES($1,'AppSkillLibrary',$2,$3,$4,'menu',1,10,$5::jsonb) RETURNING id"
		err = tx.QueryRowContext(ctx, query, parentID, skillLibraryManagementPath, skillLibraryManagementComponent, skillLibraryManagementViewPermission, meta).Scan(&pageID)
	} else if err == nil {
		query := "UPDATE menus SET pid=$2,name='AppSkillLibrary',component=$3,auth_code=$4,type='menu',status=1,sort=10,meta=$5::jsonb WHERE id=$1"
		_, err = tx.ExecContext(ctx, query, pageID, parentID, skillLibraryManagementComponent, skillLibraryManagementViewPermission, meta)
	}
	if err != nil {
		return err
	}

	buttonMeta, _ := json.Marshal(map[string]any{"icon": "lucide:pencil", "title": "编辑成长技能库"})
	buttonQuery := "INSERT INTO menus(pid,name,auth_code,type,status,sort,meta) SELECT $1,'AppSkillLibraryEdit',$2,'button',1,1,$3::jsonb WHERE NOT EXISTS (SELECT 1 FROM menus WHERE pid=$1 AND auth_code=$2 AND type='button')"
	if _, err := tx.ExecContext(ctx, buttonQuery, pageID, skillLibraryManagementEditPermission, buttonMeta); err != nil {
		return err
	}
	roleQuery := "INSERT INTO role_menus(role_id,menu_id) SELECT role.id,menu.id FROM roles role CROSS JOIN menus menu WHERE role.code='admin' AND (menu.id=$1 OR menu.pid=$1) ON CONFLICT(role_id,menu_id) DO NOTHING"
	if _, err := tx.ExecContext(ctx, roleQuery, pageID); err != nil {
		return err
	}
	return tx.Commit()
}
