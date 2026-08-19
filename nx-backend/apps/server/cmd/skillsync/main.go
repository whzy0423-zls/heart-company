package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"nine-xing/nx-backend/apps/server/internal/skillcatalog"
	"nine-xing/nx-backend/apps/server/internal/theorystore"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("skillsync", flag.ContinueOnError)
	action := flags.String("action", "", "explicit workflow action: draft, ready, publish, retire, or rollback")
	version := flags.String("version", "", "semantic catalog version, for example 1.0.0")
	sourceDir := flags.String("source-dir", "", "directory containing the 35 skill package directories")
	manifestPath := flags.String("review-manifest", "", "machine-verifiable product review decision manifest")
	if err := flags.Parse(args); err != nil {
		return err
	}
	command := skillcatalog.CatalogCommand{Action: *action, Version: *version, SourceDir: *sourceDir, ManifestPath: *manifestPath}
	if err := command.Validate(); err != nil {
		return fmt.Errorf("usage: skillsync -action publish -version 1.0.0 -source-dir /path/to/skills -review-manifest /path/to/review.json: %w", err)
	}
	dsn, err := theorystore.TheoryDatabaseURL(os.Getenv)
	if err != nil {
		return err
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		return theorystore.RedactDatabaseError(err)
	}
	defer database.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		return theorystore.RedactDatabaseError(err)
	}
	result, err := skillcatalog.NewStore(database).ApplyLearningGrowthCatalog(ctx, command)
	if err != nil {
		return theorystore.RedactDatabaseError(err)
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
