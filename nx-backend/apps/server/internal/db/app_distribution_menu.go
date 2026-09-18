package db

import (
	"context"
	"database/sql"
)

const distributionMenuBindingSQL = `
INSERT INTO role_menus(role_id,menu_id)
SELECT DISTINCT source.role_id,target.id
FROM role_menus source
JOIN menus source_menu ON source_menu.id=source.menu_id
JOIN menus target ON target.name IN (
  'AppDistributionManagement',
  'AppDistributionCommissions',
  'AppDistributionRules',
  'AppDistributionSettlements',
  'AppDistributionPosterManagement'
)
WHERE source_menu.name IN ('AppManage','CustomerAppUsers','CustomerAppUsersEdit')
   OR source_menu.auth_code IN ('Customer:App:List','Customer:App:Write')
ON CONFLICT(role_id,menu_id) DO NOTHING;
`

// seedDistributionMenuBindings gives existing App customer-management roles
// the newly introduced agent-management pages. Fresh deployments already get
// these rows from defaultMenus before seedRoles; this covers upgraded
// deployments where roles were created before the distribution pages existed.
func seedDistributionMenuBindings(ctx context.Context, database *sql.DB) error {
	_, err := database.ExecContext(ctx, distributionMenuBindingSQL)
	return err
}
