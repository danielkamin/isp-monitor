package prober

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// fakeNet symuluje trasę: path[i] opisuje odpowiedź na TTL i+1.
// Dla TTL poza path zwraca timeout.
type fakeHop struct {
	ip      string
	rtt     time.Duration
	end     bool
	reached bool
	// drop(callNo) == true -> timeout dla tego wywołania (callNo liczone od 0 per hop)
	drop func(n int) bool
}

func fakeProbe(path []fakeHop) (hopProbeFunc, map[int]int) {
	calls := map[int]int{}
	return func(ctx context.Context, ttl, seq int) (hopReply, error) {
		n := calls[ttl]
		calls[ttl]++
		if ttl > len(path) {
			return hopReply{}, nil
		}
		h := path[ttl-1]
		if h.ip == "" || (h.drop != nil && h.drop(n)) {
			return hopReply{}, nil
		}
		return hopReply{ip: h.ip, rtt: h.rtt, got: true, reached: h.reached, end: h.end}, nil
	}, calls
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestRunMtr_CleanPath(t *testing.T) {
	probe, calls := fakeProbe([]fakeHop{
		{ip: "10.0.0.1", rtt: ms(1)},
		{ip: "10.0.1.1", rtt: ms(5)},
		{ip: "8.8.8.8", rtt: ms(10), end: true, reached: true},
	})
	res, err := runMtr(context.Background(), probe, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.ErrorMsg != "" {
		t.Fatalf("want success, got %+v", res)
	}
	if len(res.HopResult) != 3 {
		t.Fatalf("want 3 hops, got %d", len(res.HopResult))
	}
	if res.LatencyMs != 10 || res.PacketLoss != 0 {
		t.Errorf("latency/loss = %d/%v, want 10/0", res.LatencyMs, res.PacketLoss)
	}
	for i, h := range res.HopResult {
		if h.Ttl != int32(i+1) || h.Sent != 5 || h.Recv != 5 || h.Loss != 0 {
			t.Errorf("hop %d = %+v", i, h)
		}
	}
	if res.HopResult[1].Ip != "10.0.1.1" {
		t.Errorf("hop 2 ip = %s", res.HopResult[1].Ip)
	}
	// po odkryciu trasy nikt nie sonduje TTL > 3
	for ttl := range calls {
		if ttl > 3 {
			t.Errorf("probed ttl %d beyond path", ttl)
		}
	}
}

func TestRunMtr_PacketLossPerHop(t *testing.T) {
	probe, _ := fakeProbe([]fakeHop{
		{ip: "10.0.0.1", rtt: ms(1)},
		{ip: "10.0.1.1", rtt: ms(2), drop: func(n int) bool { return n%2 == 1 }}, // 5 prób: gubi 2 (n=1,3)
		{ip: "1.1.1.1", rtt: ms(3), end: true, reached: true},
	})
	res, err := runMtr(context.Background(), probe, 5)
	if err != nil {
		t.Fatal(err)
	}
	h := res.HopResult[1]
	if h.Sent != 5 || h.Recv != 3 || h.Loss != 0.4 {
		t.Errorf("hop 2 = %+v, want sent=5 recv=3 loss=0.4", h)
	}
	if res.HopResult[0].Loss != 0 || res.HopResult[2].Loss != 0 {
		t.Error("loss leaked to other hops")
	}
	if res.PacketLoss != 0 {
		t.Errorf("end-to-end loss = %v, want 0", res.PacketLoss)
	}
}

func TestRunMtr_SilentRouterButDestinationReached(t *testing.T) {
	// popularny przypadek: router nie odpowiada na TTL-exceeded
	probe, _ := fakeProbe([]fakeHop{
		{ip: "10.0.0.1", rtt: ms(1)},
		{ip: ""}, // zawsze timeout
		{ip: "9.9.9.9", rtt: ms(4), end: true, reached: true},
	})
	res, err := runMtr(context.Background(), probe, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("want success, got %+v", res)
	}
	h := res.HopResult[1]
	if h.Ip != "*" || h.Loss != 1 || h.Recv != 0 || h.Rtt != 0 || h.Best != 0 {
		t.Errorf("silent hop = %+v", h)
	}
	if len(res.HopResult) != 3 {
		t.Errorf("want 3 hops, got %d", len(res.HopResult))
	}
}

func TestRunMtr_DestinationNeverAnswers(t *testing.T) {
	// 2 hopy odpowiadają, potem cisza -> runda 1 kończy po mtrMaxMisses timeoutach
	probe, calls := fakeProbe([]fakeHop{
		{ip: "10.0.0.1", rtt: ms(1)},
		{ip: "10.0.1.1", rtt: ms(2)},
	})
	res, err := runMtr(context.Background(), probe, 3)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || res.ErrorMsg == "" {
		t.Fatalf("want failure with message, got %+v", res)
	}
	wantLen := 2 + mtrMaxMisses
	if len(res.HopResult) != wantLen {
		t.Errorf("hops = %d, want %d", len(res.HopResult), wantLen)
	}
	if res.PacketLoss != 1 {
		t.Errorf("loss = %v, want 1", res.PacketLoss)
	}
	if calls[wantLen+1] != 0 {
		t.Error("probed past the cutoff")
	}
}

func TestRunMtr_DestinationUnreachableEndsPath(t *testing.T) {
	probe, calls := fakeProbe([]fakeHop{
		{ip: "10.0.0.1", rtt: ms(1)},
		{ip: "10.0.1.1", rtt: ms(2), end: true}, // unreachable: end, ale nie reached
		{ip: "7.7.7.7", rtt: ms(3)},
	})
	res, err := runMtr(context.Background(), probe, 3)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Error("unreachable must not be success")
	}
	if len(res.HopResult) != 2 {
		t.Errorf("hops = %d, want 2", len(res.HopResult))
	}
	if calls[3] != 0 {
		t.Error("probed beyond unreachable hop")
	}
}

func TestRunMtr_RttStats(t *testing.T) {
	rtts := []int{10, 30, 20}
	i := 0
	probe := func(ctx context.Context, ttl, seq int) (hopReply, error) {
		r := hopReply{ip: "8.8.8.8", rtt: ms(rtts[i%3]), got: true, reached: true, end: true}
		i++
		return r, nil
	}
	res, err := runMtr(context.Background(), probe, 3)
	if err != nil {
		t.Fatal(err)
	}
	h := res.HopResult[0]
	if h.Best != 10 || h.Worst != 30 || h.Rtt != 20 {
		t.Errorf("best/worst/avg = %d/%d/%d, want 10/30/20", h.Best, h.Worst, h.Rtt)
	}
}

func TestRunMtr_SeqUniquePerProbe(t *testing.T) {
	seen := map[int]bool{}
	probe := func(ctx context.Context, ttl, seq int) (hopReply, error) {
		if seen[seq] {
			t.Errorf("duplicate seq %d", seq)
		}
		seen[seq] = true
		if seq < 0 || seq > 0xffff {
			t.Errorf("seq %d out of uint16", seq)
		}
		if ttl == 3 {
			return hopReply{ip: "x", got: true, reached: true, end: true}, nil
		}
		return hopReply{ip: "r", got: true}, nil
	}
	if _, err := runMtr(context.Background(), probe, mtrRounds); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3*mtrRounds {
		t.Errorf("seen %d probes, want %d", len(seen), 3*mtrRounds)
	}
}

func TestRunMtr_PathShrinksInLaterRound(t *testing.T) {
	round := 0
	probe := func(ctx context.Context, ttl, seq int) (hopReply, error) {
		round = seq / maxHops
		end := 3
		if round >= 1 {
			end = 2 // zmiana trasy
		}
		if ttl == end {
			return hopReply{ip: "d", got: true, reached: true, end: true}, nil
		}
		return hopReply{ip: "r", got: true}, nil
	}
	res, err := runMtr(context.Background(), probe, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || len(res.HopResult) != 2 {
		t.Errorf("success=%v hops=%d, want true/2", res.Success, len(res.HopResult))
	}
}

func TestRunMtr_ProbeErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	probe := func(ctx context.Context, ttl, seq int) (hopReply, error) { return hopReply{}, boom }
	if _, err := runMtr(context.Background(), probe, 3); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}

func TestRunMtr_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	probe := func(ctx context.Context, ttl, seq int) (hopReply, error) {
		n++
		if n == 2 {
			cancel()
		}
		return hopReply{ip: "r", got: true}, nil
	}
	_, err := runMtr(ctx, probe, 5)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if n != 2 {
		t.Errorf("probe called %d times after cancel, want 2", n)
	}
}

// --- classify / matchesOrig na prawdziwych bajtach ICMP ---

const testID, testSeq = 4242, 77

func echoRequestBytes(t *testing.T, id, seq int) []byte {
	t.Helper()
	b, err := (&icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Body: &icmp.Echo{ID: id, Seq: seq},
	}).Marshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// origDatagram = nagłówek IPv4 (IHL=5) + nasz Echo Request, tak jak router cytuje go w błędzie.
func origDatagram(t *testing.T, id, seq int) []byte {
	ip := make([]byte, 20)
	ip[0] = 0x45
	return append(ip, echoRequestBytes(t, id, seq)...)
}

// roundTrip serializuje i parsuje wiadomość tak jak robi to socket.
func roundTrip(t *testing.T, m *icmp.Message) *icmp.Message {
	t.Helper()
	b, err := m.Marshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := icmp.ParseMessage(icmpProtoType, b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestClassify(t *testing.T) {
	timeExceeded := func(data []byte) *icmp.Message {
		return roundTrip(t, &icmp.Message{Type: ipv4.ICMPTypeTimeExceeded, Body: &icmp.TimeExceeded{Data: data}})
	}
	unreach := func(data []byte) *icmp.Message {
		return roundTrip(t, &icmp.Message{Type: ipv4.ICMPTypeDestinationUnreachable, Code: 1, Body: &icmp.DstUnreach{Data: data}})
	}
	echo := func(typ ipv4.ICMPType, id, seq int) *icmp.Message {
		return roundTrip(t, &icmp.Message{Type: typ, Body: &icmp.Echo{ID: id, Seq: seq}})
	}

	ipOpts := origDatagram(t, testID, testSeq) // IHL=6 (opcje IP) — nagłówek 24 B
	ipOpts = append(append([]byte{0x46}, ipOpts[1:20]...), append([]byte{0, 0, 0, 0}, ipOpts[20:]...)...)

	nonEcho := origDatagram(t, testID, testSeq)
	nonEcho[20] = byte(ipv4.ICMPTypeEchoReply) // oryginał nie jest Echo Request

	tests := []struct {
		name string
		msg  *icmp.Message
		want replyKind
	}{
		{"echo reply match", echo(ipv4.ICMPTypeEchoReply, testID, testSeq), replyReached},
		{"echo reply wrong id", echo(ipv4.ICMPTypeEchoReply, testID+1, testSeq), replyNone},
		{"echo reply wrong seq", echo(ipv4.ICMPTypeEchoReply, testID, testSeq+1), replyNone},
		{"echo request (loopback widzi własny)", echo(ipv4.ICMPTypeEcho, testID, testSeq), replyNone},
		{"time exceeded match", timeExceeded(origDatagram(t, testID, testSeq)), replyHop},
		{"time exceeded wrong id", timeExceeded(origDatagram(t, testID+1, testSeq)), replyNone},
		{"time exceeded wrong seq", timeExceeded(origDatagram(t, testID, testSeq+1)), replyNone},
		{"time exceeded IP z opcjami", timeExceeded(ipOpts), replyHop},
		{"time exceeded obcięty datagram", timeExceeded(origDatagram(t, testID, testSeq)[:24]), replyNone},
		{"time exceeded pusty", timeExceeded(nil), replyNone},
		{"time exceeded oryginał nie-echo", timeExceeded(nonEcho), replyNone},
		{"unreachable match", unreach(origDatagram(t, testID, testSeq)), replyEnd},
		{"unreachable cudzy", unreach(origDatagram(t, testID+5, testSeq)), replyNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.msg, testID, testSeq); got != tc.want {
				t.Errorf("classify = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMatchesOrig_BadHeaders(t *testing.T) {
	if matchesOrig(nil, 1, 1) {
		t.Error("nil matched")
	}
	bad := origDatagram(t, 1, 1)
	bad[0] = 0x41 // IHL=1 -> 4 B, niepoprawne
	if matchesOrig(bad, 1, 1) {
		t.Error("IHL<5 matched")
	}
	huge := origDatagram(t, 1, 1)
	huge[0] = 0x4f // IHL=15 -> 60 B > len
	if matchesOrig(huge, 1, 1) {
		t.Error("IHL beyond buffer matched")
	}
}

// --- integracja: loopback, wymaga raw socketów (root / CAP_NET_RAW) ---

func TestMtrProber_Loopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := (&MtrProber{}).Probe(ctx, netip.MustParseAddr("127.0.0.1"), 0)
	if err != nil {
		t.Skipf("raw ICMP niedostępny w tym środowisku: %v", err)
	}
	if !res.Success {
		t.Fatalf("loopback unreachable: %+v", res)
	}
	if len(res.HopResult) != 1 || res.HopResult[0].Ip != "127.0.0.1" {
		t.Errorf("hops = %+v, want single hop 127.0.0.1", res.HopResult)
	}
	if h := res.HopResult[0]; h.Sent != mtrRounds || h.Recv != mtrRounds {
		t.Errorf("sent/recv = %d/%d, want %d/%d", h.Sent, h.Recv, mtrRounds, mtrRounds)
	}
}

func TestMtrProber_RejectsIPv6(t *testing.T) {
	if _, err := (&MtrProber{}).Probe(context.Background(), netip.MustParseAddr("::1"), 0); err == nil {
		t.Error("want error for IPv6")
	}
}

func TestMtrProber_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&MtrProber{}).Probe(ctx, netip.MustParseAddr("127.0.0.1"), 0)
	if err == nil {
		t.Error("want error for cancelled context")
	}
}
