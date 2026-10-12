package prober

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestTracerouteProber_Loopback(t *testing.T) {
	requireICMP(t, "ip4:icmp")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := (&TracerouteProber{}).Probe(ctx, netip.MustParseAddr("127.0.0.1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("loopback not reached: %+v", res)
	}
	if len(res.HopResult) == 0 {
		t.Fatal("no hops")
	}
	last := res.HopResult[len(res.HopResult)-1]
	if last.Ip != "127.0.0.1" {
		t.Errorf("last hop ip = %s, want 127.0.0.1", last.Ip)
	}
	if len(res.HopResult) != 1 {
		t.Errorf("loopback should be 1 hop, got %d: %+v", len(res.HopResult), res.HopResult)
	}
	if res.ProbedAt.IsZero() {
		t.Error("ProbedAt not set")
	}
	if h := res.HopResult[0]; h.Sent != 1 || h.Recv != 1 || h.Loss != 0 {
		t.Errorf("single-round stats = %+v, want sent=1 recv=1 loss=0", h)
	}
}

func TestTracerouteProber_ConcurrentDoNotMixHops(t *testing.T) {
	requireICMP(t, "ip4:icmp")
	const n = 8
	type out struct {
		res ProbeResult
		err error
	}
	ch := make(chan out, n)
	for range n {
		go func() {
			res, err := (&TracerouteProber{}).Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), 0)
			ch <- out{res, err}
		}()
	}
	for range n {
		o := <-ch
		if o.err != nil || !o.res.Success || len(o.res.HopResult) != 1 {
			t.Errorf("res=%+v err=%v, want success with 1 hop", o.res, o.err)
		}
	}
}

func TestTracerouteProber_HopInvariants(t *testing.T) {
	requireICMP(t, "ip4:icmp")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := (&TracerouteProber{}).Probe(ctx, netip.MustParseAddr("127.0.0.1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, h := range res.HopResult {
		if h.Ttl != int32(i+1) {
			t.Errorf("hop %d has ttl %d, want %d", i, h.Ttl, i+1)
		}
		if h.Ip == "" {
			t.Errorf("hop %d has empty ip", i)
		}
		if h.Ip == "*" && h.Rtt != 0 {
			t.Errorf("hop %d: timeout with rtt %d", i, h.Rtt)
		}
		if h.Rtt < 0 {
			t.Errorf("hop %d: negative rtt", i)
		}
	}
}

func TestTracerouteProber_CancelledContext(t *testing.T) {
	requireICMP(t, "ip4:icmp")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&TracerouteProber{}).Probe(ctx, netip.MustParseAddr("127.0.0.1"), 0)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestTracerouteProber_ContextCancelledMidTrace(t *testing.T) {
	if testing.Short() {
		t.Skip("czeka na timeouty hopów")
	}
	requireICMP(t, "ip4:icmp")
	// TEST-NET-1: brak odpowiedzi, każdy hop = timeout 2 s; ctx ma przerwać trasę wcześniej
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	_, err := (&TracerouteProber{}).Probe(ctx, netip.MustParseAddr("192.0.2.1"), 0)
	if err == nil {
		return // środowisko odpowiedziało/zakończyło trasę samo — nic do sprawdzenia
	}
	if d := time.Since(start); d > 8*time.Second {
		t.Errorf("probe took %v after ctx deadline", d)
	}
}
