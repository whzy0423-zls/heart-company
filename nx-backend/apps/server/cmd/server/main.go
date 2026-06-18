package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/server"
)

func main() {
	env := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	database, err := db.Open(ctx, env.DatabaseURL, env.AdminUsername, env.AdminPassword)
	cancel()
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	address := fmt.Sprintf(":%d", env.Port)
	log.Printf("Nine Xing Go server listening on http://localhost%s", address)
	if err := http.ListenAndServe(address, server.New(env, database)); err != nil {
		log.Fatal(err)
	}
}
