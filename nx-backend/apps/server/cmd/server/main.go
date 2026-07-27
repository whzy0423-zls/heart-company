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
	if err := config.ValidateProduction(env); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	database, err := db.Open(ctx, env.DatabaseURL, env.AdminUsername, env.AdminPassword)
	cancel()
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	address := fmt.Sprintf(":%d", env.Port)
	log.Printf("Nine Xing Go server listening on http://localhost%s", address)
	handler := server.New(env, database)
	if shutdowner, ok := handler.(interface{ Shutdown() }); ok {
		defer shutdowner.Shutdown()
	}
	httpServer := &http.Server{
		Addr:              address,
		Handler:           handler,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      120 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
