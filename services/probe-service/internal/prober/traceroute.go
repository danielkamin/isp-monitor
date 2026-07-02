package prober

import (
	"context"
	"log"
)

type TracerouteProber struct {
}

func (p *TracerouteProber) Probe(ctx context.Context, target string) (ProbeResult, error) {
	log.Print("Probing traceroute")
	return ProbeResult{}, nil
}
