package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/danielkamin/isp-monitor/pkg/secrets"
	"github.com/danielkamin/probe-service/internal/config"
	"github.com/danielkamin/probe-service/internal/db"
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

	log.Println("probe-service started")

	<-ctx.Done()
	log.Println("shutting down")
}
