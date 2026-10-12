package prober

import (
	"context"
	"net/netip"

	"log"
)

type HttpProber struct {
}

func (p *HttpProber) Probe(ctx context.Context, ip netip.Addr, port int) (ProbeResult, error) {
	log.Print("Probing http")
	return ProbeResult{}, nil
}
