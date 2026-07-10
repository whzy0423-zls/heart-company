package videoproject

import (
	"context"
	"database/sql"
)

func runLegacyWorkflowMigration(ctx context.Context, database *sql.DB) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, `SELECT migrate_legacy_video_project_workflows()`); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS uq_video_shot_assets_exact_reference
		ON video_shot_assets(
			shot_id,
			asset_type,
			reference_role,
			COALESCE(source_type,''),
			COALESCE(source_id,''),
			object_url
		)`); err != nil {
		return err
	}
	return transaction.Commit()
}
