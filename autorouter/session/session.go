package session

import (
	"autorouter/common"
	"encoding/json"

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
	NetID  int          `json:"net_id"`
	M2From Segment      `json:"m2_from"`
	M2To   Segment      `json:"m2_to"`
	M3     TrackSegment `json:"m3"`
	Err    error        `json:"-"`
}

func (r NetResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		NetID  int          `json:"net_id"`
		M2From Segment      `json:"m2_from"`
		M2To   Segment      `json:"m2_to"`
		M3     TrackSegment `json:"m3"`
		Error  string       `json:"error,omitempty"`
	}
	w := wire{NetID: r.NetID, M2From: r.M2From, M2To: r.M2To, M3: r.M3}
	if r.Err != nil {
		w.Error = r.Err.Error()
	}
	return json.Marshal(w)
}

func (s *Session) Route() []NetResult {
	results := make([]NetResult, len(s.nets))
	for i, net := range s.nets {
		m2From, m2To, m3, err := s.router.Route(net.From, net.To, net.ID)
		results[i] = NetResult{
			NetID:  net.ID,
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
