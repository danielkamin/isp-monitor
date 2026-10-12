package prober

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type PingProber struct {
}

func (p *PingProber) Probe(ctx context.Context, ip netip.Addr, port int) (ProbeResult, error) {
	c, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return ProbeResult{}, err
	}
	defer c.Close()

	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID: os.Getpid() & 0xffff, Seq: 1,
			Data: []byte("HELLO"),
		},
	}

	wb, err := wm.Marshal(nil)
	if err != nil {
		return ProbeResult{}, err
	}

	start := time.Now()
	if _, err := c.WriteTo(wb, &net.UDPAddr{IP: net.ParseIP(ip.String()), Zone: ""}); err != nil {
		return ProbeResult{}, err

	}

	deadline := start.Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	c.SetReadDeadline(deadline)
	// anulowanie ctx odblokowuje ReadFrom
	stop := context.AfterFunc(ctx, func() { c.SetReadDeadline(time.Now()) })
	defer stop()

	rb := make([]byte, 1500)
	n, _, err := c.ReadFrom(rb)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return ProbeResult{}, ctx.Err()
		}
		return ProbeResult{Success: false, ErrorMsg: err.Error(), ProbedAt: start.UTC(), PacketLoss: 1}, nil
	}
	rtt := time.Since(start)

	rm, err := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), rb[:n])
	if err != nil {
		return ProbeResult{}, err
	}
	if rm.Type == ipv4.ICMPTypeEchoReply {
		return ProbeResult{Success: true, LatencyMs: int32(rtt.Milliseconds()), ProbedAt: start.UTC(), PacketLoss: 0}, nil
	}
	return ProbeResult{Success: false, ErrorMsg: fmt.Sprintf("unexpected ICMP type: %v", rm.Type), ProbedAt: start.UTC(), PacketLoss: 1}, nil
}
