package server

import (
	pb "github.com/danielkamin/isp-monitor/pkg/pb/probe/v1"
	uuid "github.com/google/uuid"
)

type ResultsServer struct {
	pb.UnimplementedResultsServiceServer
	broadcaster *Broadcaster
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
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

func NewResultsServer(broadcaster *Broadcaster) *ResultsServer {
	s := &ResultsServer{broadcaster: broadcaster}
	return s
}
