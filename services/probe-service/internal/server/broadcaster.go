package server

import (
	"log/slog"
	"sync"

	pb "github.com/danielkamin/isp-monitor/pkg/pb/probe/v1"
)

type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]chan *pb.StreamResultsResponse
	logger      *slog.Logger
}

func (b *Broadcaster) Publish(r *pb.StreamResultsResponse) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- r:
		default:
			b.logger.Info("dropping publishing")
		}
	}
}

func (b *Broadcaster) Subscribe(id string) chan *pb.StreamResultsResponse {
	ch := make(chan *pb.StreamResultsResponse, 64)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[id] = ch
	return ch
}
func (b *Broadcaster) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, id)
}

func NewBroadcaster(logger *slog.Logger) *Broadcaster {
	b := &Broadcaster{subscribers: make(map[string]chan *pb.StreamResultsResponse), logger: logger}
	return b
}
