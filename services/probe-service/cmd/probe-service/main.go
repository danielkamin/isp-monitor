package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
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
	"github.com/jackc/pgx/v5/pgtype"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	probeLogger := logger.With("service", "probe")

	sp, err := secrets.New()
	if err != nil {
		probeLogger.Error("secrets init", "error", err.Error())
	}

	cfg, err := config.Load(sp)
	if err != nil {
		probeLogger.Error("config init", "error", err.Error())
	}

	pool, err := db.Connect(ctx, cfg.Database)
	if err != nil {
		probeLogger.Error("db init", "error", err.Error())
	}
	defer pool.Close()
	dbQueries := queries.New(pool)

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%s", cfg.GRPC.Port))
	if err != nil {
		probeLogger.Error("tcp init", "error", err.Error())
	}

	grpcServer := grpc.NewServer()
	broadcaster := server.NewBroadcaster(probeLogger.With("component", "broadcaster"))
	pb.RegisterResultsServiceServer(grpcServer, server.NewResultsServer(broadcaster, probeLogger.With("component", "grpc-server")))
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			probeLogger.Error("grpc server failed", "error", err)
		}
	}()

	resultsChan := make(chan prober.ProbeResult)

	sch := scheduler.NewScheduler(dbQueries, resultsChan, cfg.Probe.ProbeIntervalSeconds, probeLogger.With("component", "scheduler"))
	go sch.Start(ctx)

	for res := range resultsChan {
		probeLogger.Debug("resultschan", slog.Any("res", fmt.Sprintf("%v", res)))
		if _, err := dbQueries.CreateProbe(ctx, queries.CreateProbeParams{
			TargetID:   res.TargetID,
			LatencyMs:  pgtype.Int4{Int32: res.LatencyMs, Valid: true},
			PacketLoss: res.PacketLoss,
			Success:    res.Success,
			ErrorMsg:   pgtype.Text{String: res.ErrorMsg, Valid: res.ErrorMsg != ""}},
		); err != nil {
			probeLogger.Error("failed to save probe", "error", err)
		}
		broadcaster.Publish(&pb.StreamResultsResponse{
			TargetId:   res.TargetID.String(),
			SiteId:     res.SiteID.String(),
			Success:    res.Success,
			LatencyMs:  res.LatencyMs,
			PacketLoss: res.PacketLoss,
			ErrorMsg:   res.ErrorMsg,
			ProbedAt:   res.ProbedAt.UnixMilli(),
		})
	}

	log.Println("probe-service started")
	<-ctx.Done()
	log.Println("shutting down")
}
