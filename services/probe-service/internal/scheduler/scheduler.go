package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	db "github.com/danielkamin/probe-service/internal/db/queries"
	prober "github.com/danielkamin/probe-service/internal/prober"
)

type Scheduler struct {
	queries     *db.Queries
	resultsChan chan<- prober.ProbeResult
	interval    time.Duration
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	for {
		<-ticker.C
		targets, err := s.queries.ListActiveTargets(ctx)
		if err != nil {
			ticker.Stop()
			log.Fatalf("Error during getting targets from db: %v", err)
			return
		}
		for _, t := range targets {
			go func(target db.Target) {
				fmt.Printf("target %v", target)
				pb, err := prober.New(t.Protocol)
				if err != nil {
					ticker.Stop()
					log.Fatalf("Error during getting targets from db: %v", err)
					return
				}
				res, err := pb.Probe(ctx, "")
				if err != nil {
					log.Fatalf("Error during probing target %s ,with protocol %s", t.Name, t.Protocol)
				}
				s.resultsChan <- res
			}(t)
		}
	}
}

func NewScheduler(queries *db.Queries, resultsChan chan<- prober.ProbeResult) *Scheduler {
	return &Scheduler{
		queries:     queries,
		resultsChan: resultsChan,
		interval:    time.Second,
	}
}
