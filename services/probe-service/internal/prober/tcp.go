package prober

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"time"
)

type TcpProber struct {
}

func (p *TcpProber) Probe(ctx context.Context, ip netip.Addr, port int) (ProbeResult, error) {
	var d net.Dialer
	address := net.JoinHostPort(ip.String(), strconv.Itoa(port))

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	conn, err := d.DialContext(dialCtx, "tcp", address)
	if err != nil {
		if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
			return ProbeResult{Success: false, ErrorMsg: err.Error(), ProbedAt: start.UTC(), PacketLoss: 1}, nil

		}
		return ProbeResult{Success: false, ErrorMsg: err.Error(), ProbedAt: start.UTC(), PacketLoss: 0}, nil
	}
	defer conn.Close()

	latency := time.Since(start)

	return ProbeResult{
		Success:    true,
		LatencyMs:  int32(latency.Milliseconds()),
		ProbedAt:   start.UTC(),
		PacketLoss: 0,
	}, nil
}
