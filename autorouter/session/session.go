package session

import (
	"autorouter/common"

	"github.com/samber/lo"
)

type Point = common.Point
type Segment = common.Segment
type TrackSegment = common.TrackSegment
type Net = common.Net

type Canvas interface {
	OccupyM2(seg Segment) error
	OccupyM3(seg TrackSegment) error
}

type Router interface {
	Route(from, to Point, netID int) (Segment, Segment, TrackSegment, error)
}

type Session struct {
	canvas Canvas
	router Router
	nets   []*Net
}

func NewSession(canvas Canvas, router Router, nets []*Net) *Session {
	return &Session{canvas: canvas, router: router, nets: nets}
}

type NetResult struct {
	M2From Segment
	M2To   Segment
	M3     TrackSegment
	Err    error
}

func (s *Session) Route() []NetResult {
	results := make([]NetResult, len(s.nets))
	for i, net := range s.nets {
		m2From, m2To, m3, err := s.router.Route(net.From, net.To, net.ID)
		results[i] = NetResult{
			M2From: m2From,
			M2To:   m2To,
			M3:     m3,
			Err:    err,
		}
		if err == nil {
			lo.Must0(s.canvas.OccupyM2(m2From))
			lo.Must0(s.canvas.OccupyM2(m2To))
			lo.Must0(s.canvas.OccupyM3(m3))
		}
	}
	return results
}
