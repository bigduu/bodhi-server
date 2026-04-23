package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bigduu/bodhi-server/api/router"
	"github.com/bigduu/bodhi-server/internal/cache"
	"github.com/bigduu/bodhi-server/internal/config"
	"github.com/bigduu/bodhi-server/internal/database"
	"github.com/bigduu/bodhi-server/internal/health"
	"github.com/bigduu/bodhi-server/internal/logger"
)

//go:embed all:web/dist
var webFS embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger.Init(os.Getenv("BODHI_LOG_JSON") == "true")

	db, err := database.Connect(cfg.Database, 10, 3*time.Second)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// Check if embedded web/dist exists
	var staticFiles fs.FS
	if _, err := fs.Sub(webFS, "web/dist"); err == nil {
		staticFiles = webFS
		log.Println("embedded web UI loaded")
	} else {
		staticFiles = nil
		log.Println("no embedded web UI found, running API-only mode")
	}

	r := router.Setup(db, cfg, staticFiles)

	// Start background services
	cache.StartCleanupLoop(db)
	healthChecker := health.NewChecker(db)
	healthChecker.Start(5 * time.Minute)
	log.Println("background services started (cache cleanup, health checks)")

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Bind, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("bodhi-server starting on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("server stopped")
}
