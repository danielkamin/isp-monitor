package prober

import (
	"context"

	"log"
)

type MtrProber struct {
}

func (p *MtrProber) Probe(ctx context.Context, target string) (ProbeResult, error) {
	log.Print("Probing mtr")
	return ProbeResult{}, nil
}
