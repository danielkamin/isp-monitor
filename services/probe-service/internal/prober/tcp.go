package prober

import (
	"context"
	"log"
)

type TcpProber struct {
}

func (p *TcpProber) Probe(ctx context.Context, target string) (ProbeResult, error) {
	log.Print("Probing tcp")
	return ProbeResult{}, nil
}
