package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "github.com/danielkamin/isp-monitor/pkg/pb/probe/v1"
	"github.com/danielkamin/isp-monitor/pkg/secrets"
	"github.com/danielkamin/probe-service/internal/config"
	"github.com/danielkamin/probe-service/internal/db"
	"github.com/danielkamin/probe-service/internal/db/queries"
	"github.com/danielkamin/probe-service/internal/prober"
	"github.com/danielkamin/probe-service/internal/scheduler"
	"github.com/danielkamin/probe-service/internal/server"
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
	dbQueries := queries.New(pool)

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%s", cfg.GRPC.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	broadcaster := server.NewBroadcaster()
	pb.RegisterResultsServiceServer(grpcServer, server.NewResultsServer(broadcaster))
	grpcServer.Serve(lis)

	resultsChan := make(chan prober.ProbeResult)

	sch := scheduler.NewScheduler(dbQueries, resultsChan)
	sch.Start(ctx)
	for res := range resultsChan {
		//fill data with result from channel
		log.Printf("resultschan res: %v", res)
		dbQueries.CreateProbe(ctx, queries.CreateProbeParams{})
		broadcaster.Publish(&pb.StreamResultsResponse{})
	}
	log.Println("probe-service started")
	<-ctx.Done()
	log.Println("shutting down")
}
