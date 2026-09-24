package db

import (
	"context"
	"database/sql"
	"encoding/json"
)

const appAgentDiscountMenuID int64 = 1622

const appAgentDiscountMenuBindingSQL = `
INSERT INTO role_menus(role_id,menu_id)
SELECT DISTINCT rm.role_id,$1::bigint
FROM role_menus rm
JOIN menus source ON source.id=rm.menu_id
WHERE source.name='AppPlanManagement'
   OR source.auth_code='App:PlanManagement:View'
ON CONFLICT(role_id,menu_id) DO NOTHING`

func seedAppAgentDiscountMenu(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var parentID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM menus WHERE name='AppManage' AND path='/app' AND status=1`).Scan(&parentID); err != nil {
		return err
	}
	meta, err := json.Marshal(map[string]any{"icon": "lucide:badge-percent", "title": "代理购卡优惠"})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO menus(id,pid,name,path,component,auth_code,type,status,sort,meta)
		VALUES($1,$2,'AppAgentDiscounts','/app/agent-discounts','/app/agent-discounts','App:PlanManagement:View','menu',1,12,$3::jsonb)
		ON CONFLICT(id) DO UPDATE SET
			pid=EXCLUDED.pid,
			name=EXCLUDED.name,
			path=EXCLUDED.path,
			component=EXCLUDED.component,
			auth_code=EXCLUDED.auth_code,
			type=EXCLUDED.type,
			status=EXCLUDED.status,
			sort=EXCLUDED.sort,
			meta=EXCLUDED.meta`, appAgentDiscountMenuID, parentID, string(meta)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, appAgentDiscountMenuBindingSQL, appAgentDiscountMenuID); err != nil {
		return err
	}
	return tx.Commit()
}
