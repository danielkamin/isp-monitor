package prober

import (
	"context"
	"log"
)

type PingProber struct {
}

func (p *PingProber) Probe(ctx context.Context, target string) (ProbeResult, error) {
	log.Print("Probing ping")

	return ProbeResult{}, nil
}
