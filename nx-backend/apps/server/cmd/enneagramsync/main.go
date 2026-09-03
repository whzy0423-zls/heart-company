package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/enneagramcatalog"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "enneagramsync:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("enneagramsync", flag.ContinueOnError)
	catalogDir := flags.String("catalog", "", "directory containing manifest.json and package files")
	databaseURL := flags.String("database-url", strings.TrimSpace(os.Getenv("DATABASE_URL")), "PostgreSQL connection string")
	actorID := flags.Int64("actor-id", 0, "admin user id recorded for import and review")
	validateOnly := flags.Bool("validate-only", false, "validate files without connecting to PostgreSQL")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	catalog, err := enneagramcatalog.LoadCatalog(*catalogDir)
	if err != nil {
		return err
	}
	if *validateOnly {
		fmt.Printf("validated %d enneagram packages\n", len(catalog.Packages))
		return nil
	}
	if strings.TrimSpace(*databaseURL) == "" {
		return fmt.Errorf("database-url is required unless validate-only is set")
	}
	if *actorID <= 0 {
		return fmt.Errorf("actor-id must be positive")
	}
	database, err := sql.Open("pgx", *databaseURL)
	if err != nil {
		return err
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		return err
	}
	results, err := enneagramcatalog.NewStore(database).ImportCatalog(ctx, catalog, *actorID)
	if err != nil {
		return err
	}
	created := 0
	for _, result := range results {
		if result.Created {
			created++
		}
	}
	fmt.Printf("imported %d packages (%d new drafts)\n", len(results), created)
	return nil
}
