package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	voiceBroadcastMenuID        int64 = 1621
	voiceBroadcastMenuPath            = "/app/voice-broadcast-config"
	voiceBroadcastMenuComponent       = "/app/voice-broadcast"
	voiceBroadcastMenuName            = "AppVoiceBroadcastConfig"
	voiceBroadcastAuthCode            = "App:VoiceBroadcast:Manage"
	voiceBroadcastMigrationKey        = "seed.app_voice_broadcast_menu.v1"
)

// seedVoiceBroadcastMenu mounts one and only one page below the deployed App
// 管理 catalog. It never creates a second AppManage parent.
func seedVoiceBroadcastMenu(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('seed:app-voice-broadcast-menu',0))`); err != nil {
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

	meta, _ := json.Marshal(map[string]any{"icon": "lucide:volume-2", "title": "语音播报配置"})
	var menuID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE path=$1 OR name=$2 ORDER BY CASE WHEN id=$3 THEN 0 ELSE 1 END,id LIMIT 1`, voiceBroadcastMenuPath, voiceBroadcastMenuName, voiceBroadcastMenuID).Scan(&menuID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := tx.QueryRowContext(ctx, `INSERT INTO menus(id,pid,name,path,component,auth_code,type,status,sort,meta) VALUES($1,$2,$3,$4,$5,$6,'menu',1,18,$7::jsonb) RETURNING id`, voiceBroadcastMenuID, parentID, voiceBroadcastMenuName, voiceBroadcastMenuPath, voiceBroadcastMenuComponent, voiceBroadcastAuthCode, string(meta)).Scan(&menuID); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE menus SET pid=$2,name=$3,path=$4,component=$5,auth_code=$6,type='menu',status=1,sort=18,meta=$7::jsonb WHERE id=$1`, menuID, parentID, voiceBroadcastMenuName, voiceBroadcastMenuPath, voiceBroadcastMenuComponent, voiceBroadcastAuthCode, string(meta)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM menus WHERE id<>$1 AND (path=$2 OR name=$3)`, menuID, voiceBroadcastMenuPath, voiceBroadcastMenuName); err != nil {
		return err
	}

	var marker string
	err = tx.QueryRowContext(ctx, `INSERT INTO migration_logs(key,detail) VALUES($1,$2::jsonb) ON CONFLICT(key) DO NOTHING RETURNING key`, voiceBroadcastMigrationKey, `{"description":"挂载会话语音播报配置到 App 管理"}`).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		// The page metadata is still self-healed above; do not restore manually
		// revoked role bindings after the one-time migration.
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO role_menus(role_id,menu_id) SELECT DISTINCT rm.role_id,$1::bigint FROM role_menus rm WHERE rm.menu_id=$2::bigint ON CONFLICT(role_id,menu_id) DO NOTHING`, menuID, parentID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO role_menus(role_id,menu_id) SELECT role.id,$1::bigint FROM roles role WHERE role.code='admin' ON CONFLICT(role_id,menu_id) DO NOTHING`, menuID); err != nil {
		return err
	}
	return tx.Commit()
}
