package prober

import (
	"context"
	"net/netip"
)

const (
	maxHops       = 30
	icmpProtoType = 1
)

type TracerouteProber struct {
}

type HopResult struct {
	Ttl int32
	Rtt int32 // traceroute: RTT jednej próby; mtr: średnia
	Ip  string

	// Wypełnia silnik ICMP (traceroute: Sent=Recv=1 lub 0/1 przy timeoucie).
	Sent  int32
	Recv  int32
	Loss  float64 // 0.0–1.0
	Best  int32
	Worst int32
}

// Probe = jedna runda silnika mtr (patrz icmpTrace), dzięki czemu odpowiedzi są dopasowywane po ID/seq
// i współbieżne proby ICMP nie mylą hopów.
func (p *TracerouteProber) Probe(ctx context.Context, ip netip.Addr, port int) (ProbeResult, error) {
	return icmpTrace(ctx, ip, 1)
}
