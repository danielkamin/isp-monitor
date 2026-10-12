package prober

import (
	"fmt"

	db "github.com/danielkamin/probe-service/internal/db/queries"
)

func New(protocol db.ProbeProtocol) (Prober, error) {
	switch protocol {
	case db.ProbeProtocolIcmp:
		return &PingProber{}, nil
	case db.ProbeProtocolTcp:
		return &TcpProber{}, nil
	case db.ProbeProtocolMtr:
		return &MtrProber{}, nil
	case db.ProbeProtocolTraceroute:
		return &TracerouteProber{}, nil
	case db.ProbeProtocolHttp:
		return &HttpProber{}, nil
	default:
		return nil, fmt.Errorf("Unknown protocol: %s", protocol)
	}
}
