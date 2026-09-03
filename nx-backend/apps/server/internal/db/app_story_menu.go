package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const storyManagementIcon = "lucide:book-open-text"

var storyManagementButtons = []struct {
	name, code, title, icon string
	sort                    int
}{
	{"AppStoryManagementEdit", "App:StoryManagement:Edit", "上传和编辑故事技能", "lucide:pencil", 1},
	{"AppStoryManagementDelete", "App:StoryManagement:Delete", "删除故事技能", "lucide:trash-2", 2},
	{"AppStoryManagementPublish", "App:StoryManagement:Publish", "发布故事技能", "lucide:send", 3},
}

func seedStoryManagementMenu(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var parentID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE name='AppManage' AND path='/app' AND status=1`).Scan(&parentID); err != nil {
		return fmt.Errorf("locate App 管理 menu: %w", err)
	}
	meta, _ := json.Marshal(map[string]any{"icon": storyManagementIcon, "title": "故事管理"})
	var pageID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE path='/app/story-management' ORDER BY id LIMIT 1`).Scan(&pageID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `INSERT INTO menus(pid,name,path,component,auth_code,type,status,sort,meta) VALUES($1,'AppStoryManagement','/app/story-management','/app/story-management','App:StoryManagement:View','menu',1,11,$2::jsonb) RETURNING id`, parentID, meta).Scan(&pageID)
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE menus SET pid=$2,name='AppStoryManagement',component='/app/story-management',auth_code='App:StoryManagement:View',type='menu',status=1,sort=11,meta=$3::jsonb WHERE id=$1`, pageID, parentID, meta)
	}
	if err != nil {
		return err
	}
	for _, button := range storyManagementButtons {
		buttonMeta, _ := json.Marshal(map[string]any{"icon": button.icon, "title": button.title})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO menus(pid,name,auth_code,type,status,sort,meta)
			SELECT $1,$2,$3,'button',1,$4,$5::jsonb
			WHERE NOT EXISTS (SELECT 1 FROM menus WHERE pid=$1 AND auth_code=$3 AND type='button')`, pageID, button.name, button.code, button.sort, buttonMeta); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO role_menus(role_id,menu_id)
		SELECT role.id,menu.id FROM roles role CROSS JOIN menus menu
		WHERE role.code='admin' AND (menu.id=$1 OR menu.pid=$1)
		ON CONFLICT(role_id,menu_id) DO NOTHING`, pageID); err != nil {
		return err
	}
	return tx.Commit()
}
