package prober

import "context"

type ProbeResult struct {
}

type Prober interface {
	Probe(ctx context.Context, target string) (ProbeResult, error)
}
