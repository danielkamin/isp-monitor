package server

import (
	"log/slog"

	pb "github.com/danielkamin/isp-monitor/pkg/pb/probe/v1"
	uuid "github.com/google/uuid"
)

type ResultsServer struct {
	pb.UnimplementedResultsServiceServer
	broadcaster *Broadcaster
	logger      *slog.Logger
}

func (rs *ResultsServer) StreamResults(req *pb.StreamResultsRequest, stream pb.ResultsService_StreamResultsServer) error {
	id := uuid.NewString()
	ch := rs.broadcaster.Subscribe(id)
	defer rs.broadcaster.Unsubscribe(id)
	for {
		select {
		case msg := <-ch:
			if req.SiteId != nil && msg.SiteId != *req.SiteId {
				continue
			}
			if err := stream.Send(msg); err != nil {
				rs.logger.Error("Error sending stream", slog.Any("err", err))
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

func NewResultsServer(broadcaster *Broadcaster, logger *slog.Logger) *ResultsServer {
	s := &ResultsServer{broadcaster: broadcaster, logger: logger}
	return s
}
