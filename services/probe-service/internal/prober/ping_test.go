package prober

import (
	"context"
	"errors"
	"net/netip"
	"runtime"
	"testing"
	"time"

	"golang.org/x/net/icmp"
)

// requireICMP pomija test, gdy środowisko nie pozwala otworzyć gniazda ICMP danego typu
// ("udp4" = ping bez roota, "ip4:icmp" = raw socket).
func requireICMP(t *testing.T, network string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("proby ICMP są pod Linuksa (SetControlMessage / udp4 ICMP nie istnieją na Windows w x/net)")
	}
	c, err := icmp.ListenPacket(network, "0.0.0.0")
	if err != nil {
		t.Skipf("ICMP %s niedostępny: %v", network, err)
	}
	c.Close()
}

func TestPingProber_Loopback(t *testing.T) {
	requireICMP(t, "udp4")
	before := time.Now().UTC().Add(-time.Second)

	res, err := (&PingProber{}).Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.ErrorMsg != "" {
		t.Fatalf("want success, got %+v", res)
	}
	if res.LatencyMs < 0 || res.LatencyMs > 1000 {
		t.Errorf("latency = %d ms, implausible for loopback", res.LatencyMs)
	}
	if res.PacketLoss != 0 {
		t.Errorf("PacketLoss = %v on success, want 0", res.PacketLoss)
	}
	if res.ProbedAt.Before(before) || res.ProbedAt.Location() != time.UTC {
		t.Errorf("ProbedAt = %v, want recent UTC", res.ProbedAt)
	}
}

func TestPingProber_RepeatedProbes(t *testing.T) {
	requireICMP(t, "udp4")
	for i := range 5 {
		res, err := (&PingProber{}).Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), 0)
		if err != nil || !res.Success {
			t.Fatalf("probe %d: res=%+v err=%v", i, res, err)
		}
	}
}

func TestPingProber_Concurrent(t *testing.T) {
	requireICMP(t, "udp4")
	const n = 10
	done := make(chan error, n)
	for range n {
		go func() {
			res, err := (&PingProber{}).Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), 0)
			if err == nil && !res.Success {
				err = context.DeadlineExceeded
			}
			done <- err
		}()
	}
	for range n {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

func TestPingProber_CancelledContext(t *testing.T) {
	requireICMP(t, "udp4")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&PingProber{}).Probe(ctx, netip.MustParseAddr("192.0.2.1"), 0)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestPingProber_CancelUnblocksRead(t *testing.T) {
	requireICMP(t, "udp4")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := (&PingProber{}).Probe(ctx, netip.MustParseAddr("192.0.2.1"), 0)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("cancel ignored, probe took %v", d)
	}
	_ = err // brak trasy może zakończyć probe błędem zapisu — tu liczy się tylko czas
}

func TestPingProber_ParentDeadlineWins(t *testing.T) {
	requireICMP(t, "udp4")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := (&PingProber{}).Probe(ctx, netip.MustParseAddr("192.0.2.1"), 0)
	if res.Success {
		t.Errorf("TEST-NET host answered: %+v", res)
	}
	if err == nil && res.PacketLoss != 1 {
		t.Errorf("PacketLoss = %v on timeout, want 1", res.PacketLoss)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("probe took %v, parent deadline ignored", d)
	}
}

func TestPingProber_UnreachableHost(t *testing.T) {
	if testing.Short() {
		t.Skip("czeka pełny timeout 5 s")
	}
	requireICMP(t, "udp4")

	// 192.0.2.1 (TEST-NET-1): brak odpowiedzi -> Success=false. Timeout odczytu jest wynikiem probe'a (err == nil),
	// brak trasy może zwrócić błąd zapisu — oba warianty nie mogą dać Success=true.
	res, err := (&PingProber{}).Probe(context.Background(), netip.MustParseAddr("192.0.2.1"), 0)
	if res.Success {
		t.Errorf("TEST-NET host answered: %+v (err=%v)", res, err)
	}
	if err == nil && res.ErrorMsg == "" {
		t.Errorf("failure without message: %+v", res)
	}
	if err == nil && res.PacketLoss != 1 {
		t.Errorf("PacketLoss = %v on no reply, want 1", res.PacketLoss)
	}
}
