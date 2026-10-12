package prober

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

func listenTCP(t *testing.T, addr string) (netip.Addr, int) {
	t.Helper()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("listen %s: %v", addr, err)
	}
	t.Cleanup(func() { l.Close() })
	tcp := l.Addr().(*net.TCPAddr)
	ip, _ := netip.AddrFromSlice(tcp.IP)
	return ip.Unmap(), tcp.Port
}

func TestTcpProber_OpenPort(t *testing.T) {
	ip, port := listenTCP(t, "127.0.0.1:0")
	before := time.Now().UTC().Add(-time.Second)

	res, err := (&TcpProber{}).Probe(context.Background(), ip, port)
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

func TestTcpProber_ClosedPort(t *testing.T) {
	// bierzemy wolny port i od razu go zwalniamy — nikt na nim nie słucha
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	closedPort := l.Addr().(*net.TCPAddr).Port
	l.Close()

	res, err := (&TcpProber{}).Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), closedPort)
	if err != nil {
		t.Fatalf("closed port is a probe result, not an error: %v", err)
	}
	if res.Success || res.ErrorMsg == "" {
		t.Errorf("want failure with message, got %+v", res)
	}
	if res.LatencyMs != 0 {
		t.Errorf("latency = %d on failure, want 0", res.LatencyMs)
	}
	// odmowa (RST) = host odpowiedział, to nie jest utrata pakietu
	if res.PacketLoss != 0 {
		t.Errorf("PacketLoss = %v on refused, want 0", res.PacketLoss)
	}
	if res.ProbedAt.IsZero() {
		t.Error("ProbedAt not set on failure")
	}
}

func TestTcpProber_InvalidPort(t *testing.T) {
	for _, port := range []int{-1, 0, 65536, 70000} {
		res, err := (&TcpProber{}).Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), port)
		if err != nil {
			t.Errorf("port %d: unexpected err %v", port, err)
		}
		if res.Success || res.ErrorMsg == "" {
			t.Errorf("port %d: want failure with message, got %+v", port, res)
		}
	}
}

func TestTcpProber_CancelledContext(t *testing.T) {
	ip, port := listenTCP(t, "127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := (&TcpProber{}).Probe(ctx, ip, port)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || res.ErrorMsg == "" {
		t.Errorf("cancelled ctx must fail the probe, got %+v", res)
	}
}

func TestTcpProber_TimeoutIsPacketLoss(t *testing.T) {
	// deadline już minął -> dial kończy się timeoutem niezależnie od sieci
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	res, err := (&TcpProber{}).Probe(ctx, netip.MustParseAddr("127.0.0.1"), 80)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || res.PacketLoss != 1 {
		t.Errorf("want Success=false PacketLoss=1, got %+v", res)
	}
	if res.ErrorMsg == "" {
		t.Error("timeout without error message")
	}
}

func TestTcpProber_ParentDeadlineWins(t *testing.T) {
	// 192.0.2.1 (TEST-NET-1) nie jest routowalne: albo blackhole (timeout), albo od razu "unreachable".
	// W obu przypadkach probe ma wrócić szybko (deadline 200 ms << wewnętrzne 5 s) i z Success=false.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := (&TcpProber{}).Probe(ctx, netip.MustParseAddr("192.0.2.1"), 80)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || res.ErrorMsg == "" {
		t.Errorf("want failure, got %+v", res)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("probe took %v, parent deadline ignored", d)
	}
}

func TestTcpProber_IPv6(t *testing.T) {
	ip, port := listenTCP(t, "[::1]:0")

	res, err := (&TcpProber{}).Probe(context.Background(), ip, port)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("IPv6 loopback failed: %+v", res)
	}
}

func TestTcpProber_ManyConcurrent(t *testing.T) {
	ip, port := listenTCP(t, "127.0.0.1:0")
	const n = 50
	errs := make(chan string, n)
	for range n {
		go func() {
			res, err := (&TcpProber{}).Probe(context.Background(), ip, port)
			if err != nil || !res.Success {
				errs <- "probe failed"
				return
			}
			errs <- ""
		}()
	}
	for range n {
		if msg := <-errs; msg != "" {
			t.Error(msg)
		}
	}
}
