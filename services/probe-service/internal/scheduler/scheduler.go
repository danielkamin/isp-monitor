package scheduler

import (
	"context"
	"log/slog"
	"time"

	db "github.com/danielkamin/probe-service/internal/db/queries"
	prober "github.com/danielkamin/probe-service/internal/prober"
)

type Scheduler struct {
	queries     *db.Queries
	resultsChan chan<- prober.ProbeResult
	interval    time.Duration
	logger      *slog.Logger
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		targets, err := s.queries.ListActiveTargets(ctx)
		if err != nil {
			s.logger.Error("Error during getting targets from db: ", slog.Any("err", err))
			continue
		}
		for _, t := range targets {
			go func(target db.Target) {
				pctx, cancel := context.WithTimeout(ctx, s.interval)
				defer cancel()
				s.logger.Debug("target", "ip", target.Ip, "port", target.Port.Int32)

				pb, err := prober.New(target.Protocol)
				if err != nil {
					s.logger.Error("unknown probe protocol", "target", target.Name, "protocol", target.Protocol, slog.Any("err", err))
					return
				}

				if !target.Port.Valid && target.Protocol == db.ProbeProtocolTcp {
					s.logger.Error("target has no port", "target", target.Name, "protocol", target.Protocol)
					return
				}
				port := int(target.Port.Int32)

				res, err := pb.Probe(pctx, target.Ip, port)
				if err != nil {
					s.logger.Error("Error during probing", "target", target.Name, "protocol", target.Protocol, slog.Any("err", err))
					return
				}
				res.TargetID = target.ID
				res.SiteID = target.SiteID

				select {
				case s.resultsChan <- res:
				case <-ctx.Done():
				}
			}(t)
		}
	}
}

func NewScheduler(queries *db.Queries, resultsChan chan<- prober.ProbeResult, intervalSeconds int, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		queries:     queries,
		resultsChan: resultsChan,
		interval:    time.Duration(intervalSeconds) * time.Second,
		logger:      logger,
	}
}
