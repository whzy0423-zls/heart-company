package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/server"
)

const gracefulShutdownTimeout = time.Duration(modelconfig.MaxChatTimeoutSeconds+30) * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	env := config.Load()
	if err := config.ValidateProduction(env); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	databaseCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	database, err := db.OpenWithPoolConfig(databaseCtx, env.DatabaseURL, env.AdminUsername, env.AdminPassword, env.DBMaxOpenConns, env.DBMaxIdleConns)
	cancel()
	if err != nil {
		return fmt.Errorf("database init failed: %w", err)
	}
	defer func() { _ = database.Close() }()

	address := fmt.Sprintf(":%d", env.Port)
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
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	log.Printf("Nine Xing Go server listening on http://localhost%s", address)
	return serveHTTPServer(ctx, httpServer, listener, gracefulShutdownTimeout)
}

func serveHTTPServer(ctx context.Context, httpServer *http.Server, listener net.Listener, shutdownTimeout time.Duration) error {
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- httpServer.Serve(listener)
	}()

	select {
	case err := <-serveDone:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		_ = httpServer.Close()
		<-serveDone
		return fmt.Errorf("graceful HTTP shutdown: %w", err)
	}

	err := <-serveDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
