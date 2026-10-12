package prober

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"net/netip"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const (
	mtrRounds    = 5
	mtrTimeout   = time.Second
	mtrMaxMisses = 5 // kolejne timeouty w 1. rundzie = koniec trasy (cel nie odpowiada)
)

type MtrProber struct {
}

type replyKind int

const (
	replyNone    replyKind = iota // nie nasz pakiet
	replyHop                      // Time Exceeded od routera pośredniego
	replyReached                  // Echo Reply od celu
	replyEnd                      // Destination Unreachable — koniec trasy
)

type hopReply struct {
	ip      string
	rtt     time.Duration
	got     bool // ktoś odpowiedział przed timeoutem
	reached bool // odpowiedział cel
	end     bool // koniec trasy (reached albo unreachable)
}

type hopProbeFunc func(ctx context.Context, ttl, seq int) (hopReply, error)

type hopAcc struct {
	ip               string
	sent, recv       int
	sum, best, worst time.Duration
}

func (p *MtrProber) Probe(ctx context.Context, ip netip.Addr, port int) (ProbeResult, error) {
	return icmpTrace(ctx, ip, mtrRounds)
}

// icmpTrace to wspólny silnik mtr (rounds > 1) i traceroute (rounds = 1).
func icmpTrace(ctx context.Context, ip netip.Addr, rounds int) (ProbeResult, error) {
	ip = ip.Unmap()
	if !ip.Is4() {
		return ProbeResult{}, errors.New("tylko IPv4")
	}
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return ProbeResult{}, err
	}
	defer conn.Close()
	pc := conn.IPv4PacketConn()

	dst := &net.IPAddr{IP: net.IP(ip.AsSlice())}
	id := rand.IntN(0xffff) + 1 // unikalne per Probe — współbieżne proby nie mylą odpowiedzi
	buf := make([]byte, 1500)

	probe := func(ctx context.Context, ttl, seq int) (hopReply, error) {
		wb, err := (&icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Body: &icmp.Echo{ID: id, Seq: seq},
		}).Marshal(nil)
		if err != nil {
			return hopReply{}, err
		}
		if err := pc.SetTTL(ttl); err != nil {
			return hopReply{}, err
		}
		start := time.Now()
		if _, err := pc.WriteTo(wb, nil, dst); err != nil {
			return hopReply{}, err
		}
		if err := pc.SetReadDeadline(start.Add(mtrTimeout)); err != nil {
			return hopReply{}, err
		}
		for {
			if err := ctx.Err(); err != nil {
				return hopReply{}, err
			}
			n, _, peer, err := pc.ReadFrom(buf)
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					return hopReply{}, nil
				}
				return hopReply{}, err
			}
			msg, err := icmp.ParseMessage(icmpProtoType, buf[:n])
			if err != nil {
				continue
			}
			kind := classify(msg, id, seq)
			if kind == replyNone {
				continue
			}
			return hopReply{
				ip:      peer.String(),
				rtt:     time.Since(start),
				got:     true,
				reached: kind == replyReached,
				end:     kind != replyHop,
			}, nil
		}
	}
	return runMtr(ctx, probe, rounds)
}

// classify rozpoznaje, czy pakiet ICMP jest odpowiedzią na naszą próbę (id, seq).
func classify(msg *icmp.Message, id, seq int) replyKind {
	switch b := msg.Body.(type) {
	case *icmp.Echo:
		if msg.Type == ipv4.ICMPTypeEchoReply && b.ID == id && b.Seq == seq {
			return replyReached
		}
	case *icmp.TimeExceeded:
		if matchesOrig(b.Data, id, seq) {
			return replyHop
		}
	case *icmp.DstUnreach:
		if matchesOrig(b.Data, id, seq) {
			return replyEnd
		}
	}
	return replyNone
}

// matchesOrig sprawdza oryginalny datagram zagnieżdżony w błędzie ICMP:
// nagłówek IP + pierwsze 8 B naszego Echo Request.
func matchesOrig(data []byte, id, seq int) bool {
	if len(data) < 1 {
		return false
	}
	ihl := int(data[0]&0x0f) * 4
	if ihl < 20 || len(data) < ihl+8 {
		return false
	}
	icmpHdr := data[ihl:]
	return icmpHdr[0] == byte(ipv4.ICMPTypeEcho) &&
		int(icmpHdr[4])<<8|int(icmpHdr[5]) == id &&
		int(icmpHdr[6])<<8|int(icmpHdr[7]) == seq
}

// runMtr: runda 1 odkrywa długość trasy (jak traceroute), kolejne rundy mierzą tylko tę trasę.
// ponytail: sekwencyjnie, 1 próba naraz; martwy cel = ~mtrMaxMisses*mtrTimeout*rounds.
// Równoległe TTL-e, jeśli scheduler będzie to odczuwalnie blokował.
func runMtr(ctx context.Context, probe hopProbeFunc, rounds int) (ProbeResult, error) {
	probedAt := time.Now().UTC()
	acc := make([]hopAcc, maxHops)
	pathLen, reached := maxHops, false

	for round := range rounds {
		misses := 0
		for ttl := 1; ttl <= pathLen; ttl++ {
			if err := ctx.Err(); err != nil {
				return ProbeResult{}, err
			}
			r, err := probe(ctx, ttl, round*maxHops+ttl)
			if err != nil {
				return ProbeResult{}, err
			}
			h := &acc[ttl-1]
			h.sent++
			if r.got {
				misses = 0
				h.ip = r.ip
				h.recv++
				h.sum += r.rtt
				if h.recv == 1 || r.rtt < h.best {
					h.best = r.rtt
				}
				if r.rtt > h.worst {
					h.worst = r.rtt
				}
			} else {
				misses++
			}
			if r.reached {
				reached = true
			}
			if r.end {
				pathLen = ttl
				break
			}
			if round == 0 && misses >= mtrMaxMisses {
				pathLen = ttl
				break
			}
		}
	}

	hops := make([]HopResult, pathLen)
	for i := range hops {
		a := acc[i]
		hr := HopResult{Ttl: int32(i + 1), Ip: "*", Sent: int32(a.sent), Recv: int32(a.recv)}
		if a.sent > 0 {
			hr.Loss = 1 - float64(a.recv)/float64(a.sent)
		}
		if a.recv > 0 {
			hr.Ip = a.ip
			hr.Rtt = int32((a.sum / time.Duration(a.recv)).Milliseconds())
			hr.Best = int32(a.best.Milliseconds())
			hr.Worst = int32(a.worst.Milliseconds())
		}
		hops[i] = hr
	}

	last := hops[pathLen-1]
	res := ProbeResult{ProbedAt: probedAt, HopResult: hops, PacketLoss: last.Loss}
	if reached {
		res.Success = true
		res.LatencyMs = last.Rtt
	} else {
		res.ErrorMsg = "destination not reached"
	}
	return res, nil
}
