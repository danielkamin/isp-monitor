package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/danielkamin/api-service/internal/config"
	"github.com/danielkamin/api-service/internal/db"
	"github.com/danielkamin/isp-monitor/pkg/secrets"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sp, err := secrets.New()
	if err != nil {
		log.Fatalf("secrets: %v", err)
	}

	cfg, err := config.Load(sp)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pool, err := db.Connect(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	log.Println("api-service started")

	<-ctx.Done()
	log.Println("shutting down")
}
