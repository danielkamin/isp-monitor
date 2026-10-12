package prober

import (
	"context"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type ProbeResult struct {
	TargetID   pgtype.UUID
	SiteID     pgtype.UUID
	Success    bool
	LatencyMs  int32     // RTT w milisekundach
	PacketLoss float64   // 0.0–1.0 (przy jednym pakiecie: 0 albo 1)
	ErrorMsg   string    // pusty przy Success=true
	ProbedAt   time.Time // kiedy wykonano pomiar
	HopResult  []HopResult
}

type Prober interface {
	Probe(ctx context.Context, ip netip.Addr, port int) (ProbeResult, error)
}
