package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const enneagramLibraryMenuMigrationKey = "seed.app_enneagram_library_menu.v1"

// seedEnneagramLibraryMenu attaches the single management page to the
// deployment-owned App catalog. The parent is intentionally never created by
// this migration: a missing or ambiguous deployment menu must be fixed at the
// deployment boundary rather than silently creating a second catalog.
func seedEnneagramLibraryMenu(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('seed:app-enneagram-library-menu',0))`); err != nil {
		return err
	}
	var parentID int64
	var parentCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM menus WHERE name='AppManage' AND path='/app' AND status=1`).Scan(&parentCount); err != nil {
		return err
	}
	if parentCount != 1 {
		return fmt.Errorf("App 管理父菜单 name=AppManage path=/app 必须唯一，当前找到 %d 个", parentCount)
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE name='AppManage' AND path='/app' AND status=1`).Scan(&parentID); err != nil {
		return err
	}
	var marker string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO migration_logs(key,detail)
		VALUES ($1,$2::jsonb)
		ON CONFLICT (key) DO NOTHING
		RETURNING key`, enneagramLibraryMenuMigrationKey, `{"description":"挂载九型人格库到部署中的 App 管理"}`).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Rollback()
	}
	if err != nil {
		return err
	}
	var pageID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE path='/app/enneagram-library'`).Scan(&pageID)
	if errors.Is(err, sql.ErrNoRows) {
		meta, _ := json.Marshal(map[string]any{"icon": "lucide:brain-circuit", "title": "九型人格库"})
		err = tx.QueryRowContext(ctx, `
			INSERT INTO menus(pid,name,path,component,auth_code,type,status,sort,meta)
			VALUES ($1,'AppEnneagramLibrary','/app/enneagram-library','/app/enneagram-library','App:EnneagramLibrary:View','menu',1,1,$2::jsonb)
			RETURNING id`, parentID, string(meta)).Scan(&pageID)
	} else if err == nil {
		var duplicateCount int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM menus WHERE path='/app/enneagram-library'`).Scan(&duplicateCount); err != nil {
			return err
		}
		if duplicateCount != 1 {
			return fmt.Errorf("九型人格库菜单路径必须唯一，当前找到 %d 个", duplicateCount)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE menus SET pid=$2,name='AppEnneagramLibrary',component='/app/enneagram-library',auth_code='App:EnneagramLibrary:View',type='menu',status=1,meta=jsonb_build_object('icon','lucide:brain-circuit','title','九型人格库') WHERE id=$1`, pageID, parentID); err != nil {
			return err
		}
	} else {
		return err
	}
	buttons := []struct {
		name, code, title, icon string
		sort                    int
	}{
		{"AppEnneagramLibraryEdit", "App:EnneagramLibrary:Edit", "编辑九型人格库", "lucide:pencil", 1},
		{"AppEnneagramLibraryReview", "App:EnneagramLibrary:Review", "审核九型人格库", "lucide:clipboard-check", 2},
		{"AppEnneagramLibraryPublish", "App:EnneagramLibrary:Publish", "发布九型人格库", "lucide:send", 3},
	}
	for _, button := range buttons {
		meta, _ := json.Marshal(map[string]any{"icon": button.icon, "title": button.title})
		var buttonCount int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM menus WHERE pid=$1 AND auth_code=$2 AND type='button'`, pageID, button.code).Scan(&buttonCount); err != nil {
			return err
		}
		if buttonCount > 1 {
			return fmt.Errorf("九型人格库权限按钮 %s 重复", button.code)
		}
		if buttonCount == 0 {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO menus(pid,name,auth_code,type,status,sort,meta)
				VALUES ($1,$2,$3,'button',1,$4,$5::jsonb)`, pageID, button.name, button.code, button.sort, string(meta)); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `UPDATE menus SET name=$2,sort=$3,status=1,meta=$4::jsonb WHERE pid=$1 AND auth_code=$5 AND type='button'`, pageID, button.name, button.sort, string(meta), button.code); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO role_menus(role_id,menu_id)
		SELECT roles.id,menus.id FROM roles CROSS JOIN menus
		WHERE roles.code='admin' AND menus.id IN ($1, (SELECT id FROM menus WHERE pid=$1 AND auth_code='App:EnneagramLibrary:Edit'), (SELECT id FROM menus WHERE pid=$1 AND auth_code='App:EnneagramLibrary:Review'), (SELECT id FROM menus WHERE pid=$1 AND auth_code='App:EnneagramLibrary:Publish'))
		ON CONFLICT (role_id,menu_id) DO NOTHING`, pageID); err != nil {
		return err
	}
	return tx.Commit()
}
