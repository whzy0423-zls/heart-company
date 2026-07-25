package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"nine-xing/nx-backend/apps/server/internal/theorystore"
)

type cliCommand struct {
	name        string
	packagePath string
	packageID   string
	reviewType  string
	notes       string
	actorID     int64
	reviewerID  int64
	dryRun      bool
	apply       bool
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, output io.Writer) error {
	command, err := parseCLI(args)
	if err != nil {
		return err
	}
	if command.name == "validate" {
		result, err := theorystore.ValidatePackage(command.packagePath)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(result)
	}
	dsn, err := theorystore.TheoryDatabaseURL(getenv)
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return theorystore.RedactDatabaseError(err)
	}
	defer db.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return theorystore.RedactDatabaseError(err)
	}
	syncer := theorystore.NewPackageSyncer(db)
	var result any
	switch command.name {
	case "plan":
		result, err = syncer.Plan(ctx, command.packagePath)
	case "stage":
		result, err = syncer.Stage(ctx, command.packagePath, command.actorID)
	case "review":
		result, err = syncer.RecordReview(ctx, command.packageID, theorystore.ReviewType(command.reviewType), command.reviewerID, command.notes)
	case "promote":
		result, err = syncer.Promote(ctx, command.packageID, command.actorID)
	case "activate":
		err = theorystore.ActivatePackage(ctx, db, command.packageID, command.actorID)
		result = map[string]any{"packageId": command.packageID, "activated": false}
	default:
		return errors.New("unknown theorysync command")
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}

func parseCLI(args []string) (cliCommand, error) {
	if len(args) == 0 {
		return cliCommand{}, errors.New("usage: theorysync <validate|plan|stage|review|promote|activate> [options]")
	}
	command := cliCommand{name: args[0]}
	flags := flag.NewFlagSet(command.name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var actor, reviewer string
	flags.StringVar(&command.packagePath, "package", "", "portable package directory")
	flags.StringVar(&command.packageID, "package-id", "", "staged package id")
	flags.StringVar(&command.reviewType, "type", "", "review type")
	flags.StringVar(&command.notes, "notes", "", "review notes")
	flags.StringVar(&actor, "actor", "", "database user id")
	flags.StringVar(&reviewer, "reviewer", "", "database reviewer user id")
	flags.BoolVar(&command.dryRun, "dry-run", false, "show plan without writes")
	flags.BoolVar(&command.apply, "apply", false, "explicitly permit stage writes")
	if err := flags.Parse(args[1:]); err != nil {
		return cliCommand{}, fmt.Errorf("invalid %s options", command.name)
	}
	if flags.NArg() != 0 {
		return cliCommand{}, errors.New("unexpected positional arguments")
	}
	parseID := func(name, value string) (int64, error) {
		if strings.TrimSpace(value) == "" {
			return 0, nil
		}
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("%s must be a positive database user id", name)
		}
		return id, nil
	}
	var err error
	if command.actorID, err = parseID("actor", actor); err != nil {
		return cliCommand{}, err
	}
	if command.reviewerID, err = parseID("reviewer", reviewer); err != nil {
		return cliCommand{}, err
	}
	switch command.name {
	case "validate":
		if command.packagePath == "" {
			return cliCommand{}, errors.New("validate requires --package")
		}
	case "plan":
		if command.packagePath == "" || !command.dryRun {
			return cliCommand{}, errors.New("plan requires --package and --dry-run")
		}
	case "stage":
		if command.packagePath == "" || !command.apply || command.actorID <= 0 {
			return cliCommand{}, errors.New("stage requires --package, --apply, and --actor")
		}
	case "review":
		if command.packageID == "" || command.reviewerID <= 0 {
			return cliCommand{}, errors.New("review requires --package-id and --reviewer")
		}
		if _, ok := map[string]bool{"source-verification": true, "theory-review": true, "safety-review": true}[command.reviewType]; !ok {
			return cliCommand{}, errors.New("review requires a valid --type")
		}
	case "promote", "activate":
		if command.packageID == "" || command.actorID <= 0 {
			return cliCommand{}, fmt.Errorf("%s requires --package-id and --actor", command.name)
		}
	default:
		return cliCommand{}, errors.New("unknown theorysync command")
	}
	return command, nil
}
