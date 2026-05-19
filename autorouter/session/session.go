package session

import (
	"autorouter/common"
	"encoding/json"

	"github.com/samber/lo"
)

type Point = common.Point
type Segment = common.Segment
type TrackSegment = common.TrackSegment
type RoutingPin = common.RoutingPin
type Net = common.Net

type Canvas interface {
	OccupyM2(seg Segment) error
	OccupyM3(seg TrackSegment) error
	GetLowerLeft() Point
	GetM3TrackWidth() int
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
	NetID    int       `json:"net_id"`
	Segments []Segment `json:"segments"`
	Err      error     `json:"-"`
}

func (r NetResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		NetID    int       `json:"net_id"`
		Segments []Segment `json:"segments,omitempty"`
		Error    string    `json:"error,omitempty"`
	}
	w := wire{NetID: r.NetID, Segments: r.Segments}
	if r.Err != nil {
		w.Error = r.Err.Error()
	}
	return json.Marshal(w)
}

func (s *Session) Route() []NetResult {
	results := make([]NetResult, len(s.nets))
	for i, net := range s.nets {
		m2From, m2To, m3, err := s.router.Route(
			Point{X: net.From.XLow, Y: net.From.YLow},
			Point{X: net.To.XLow, Y: net.To.YLow},
			net.ID,
		)
		if err != nil {
			results[i] = NetResult{NetID: net.ID, Err: err}
			continue
		}

		m2From = extendM2ToCoverPin(m2From, net.From)
		m2To = extendM2ToCoverPin(m2To, net.To)
		m2From.Layer = common.M2
		m2To.Layer = common.M2

		ll := s.canvas.GetLowerLeft()
		tw := s.canvas.GetM3TrackWidth()
		m3Seg := Segment{
			LowerLeft:  Point{X: m3.Start, Y: ll.Y + m3.TrackID*tw},
			UpperRight: Point{X: m3.End, Y: ll.Y + (m3.TrackID+1)*tw},
			NetID:      m3.NetID,
			Layer:      common.M3,
		}

		segs := []Segment{m2From, m3Seg, m2To}
		segs = appendVia(segs, pinBBox(net.From), m2From, common.Via12, net.ID)
		segs = appendVia(segs, m2From, m3Seg, common.Via23, net.ID)
		segs = appendVia(segs, m2To, m3Seg, common.Via23, net.ID)
		segs = appendVia(segs, pinBBox(net.To), m2To, common.Via12, net.ID)

		results[i] = NetResult{NetID: net.ID, Segments: segs}

		lo.Must0(s.canvas.OccupyM2(m2From))
		lo.Must0(s.canvas.OccupyM2(m2To))
		lo.Must0(s.canvas.OccupyM3(m3))
	}
	return results
}

// appendVia computes the intersection of a and b; if non-degenerate, appends it to segs.
func appendVia(segs []Segment, a, b Segment, layer common.Layer, netID int) []Segment {
	x0 := max(a.LowerLeft.X, b.LowerLeft.X)
	y0 := max(a.LowerLeft.Y, b.LowerLeft.Y)
	x1 := min(a.UpperRight.X, b.UpperRight.X)
	y1 := min(a.UpperRight.Y, b.UpperRight.Y)
	if x0 >= x1 || y0 >= y1 {
		return segs
	}
	return append(segs, Segment{
		LowerLeft:  Point{X: x0, Y: y0},
		UpperRight: Point{X: x1, Y: y1},
		NetID:      netID,
		Layer:      layer,
	})
}

// pinBBox converts a RoutingPin to its bounding-box Segment.
// When XHigh <= XLow the bbox is degenerate and appendVia will discard it.
func pinBBox(pin RoutingPin) Segment {
	return Segment{
		LowerLeft:  Point{X: pin.XLow, Y: pin.YLow},
		UpperRight: Point{X: pin.XHigh, Y: pin.YHigh},
	}
}

func extendM2ToCoverPin(m2 Segment, pin RoutingPin) Segment {
	return Segment{
		LowerLeft:  Point{X: m2.LowerLeft.X, Y: min(m2.LowerLeft.Y, pin.YLow)},
		UpperRight: Point{X: m2.UpperRight.X, Y: max(m2.UpperRight.Y, pin.YHigh)},
		NetID:      m2.NetID,
	}
}
