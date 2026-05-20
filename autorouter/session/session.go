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
type Netlist = common.Netlist
type ViaConfig = common.ViaConfig
type DRCSpec = common.DRCSpec

type Canvas interface {
	OccupyM2(seg Segment) error
	OccupyM3(seg TrackSegment) error
	GetLowerLeft() Point
	GetM3TrackWidth() int
}

type Router interface {
	Route(pins []RoutingPin, netID int) ([]Segment, TrackSegment, error)
}

type Session struct {
	canvas  Canvas
	router  Router
	netlist *Netlist
	via12   ViaConfig
	via23   ViaConfig
	m2DRC   DRCSpec
	m3DRC   DRCSpec
}

func NewSession(canvas Canvas, router Router, netlist *Netlist, via12, via23 ViaConfig, m2DRC, m3DRC DRCSpec) *Session {
	return &Session{
		canvas:  canvas,
		router:  router,
		netlist: netlist,
		via12:   via12,
		via23:   via23,
		m2DRC:   m2DRC,
		m3DRC:   m3DRC,
	}
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
	results := make([]NetResult, len(s.netlist.Nets))
	m2EndExt := s.m2DRC.EndExtension()
	via23EndExt := max(s.m2DRC.EndExtension(), s.m3DRC.EndExtension())

	for i, net := range s.netlist.Nets {
		m2Segs, m3, err := s.router.Route(net.Pins, net.ID)
		if err != nil {
			results[i] = NetResult{NetID: net.ID, Err: err}
			continue
		}

		for j, pin := range net.Pins {
			m2Segs[j] = extendM2ToCoverPin(m2Segs[j], pin, m2EndExt)
			m2Segs[j].Layer = common.M2
		}

		ll := s.canvas.GetLowerLeft()
		tw := s.canvas.GetM3TrackWidth()
		m3Seg := Segment{
			LowerLeft:  Point{X: m3.Start, Y: ll.Y + m3.TrackID*tw},
			UpperRight: Point{X: m3.End, Y: ll.Y + (m3.TrackID+1)*tw},
			NetID:      m3.NetID,
			Layer:      common.M3,
		}

		segs := make([]Segment, 0, len(m2Segs)+1)
		segs = append(segs, m2Segs...)
		segs = append(segs, m3Seg)
		for j, pin := range net.Pins {
			segs = appendViaCuts(segs, s.via12, pinBBox(pin), m2Segs[j], common.Via12, m2EndExt)
			segs = appendViaCuts(segs, s.via23, m2Segs[j], m3Seg, common.Via23, via23EndExt)
		}

		results[i] = NetResult{NetID: net.ID, Segments: segs}

		for _, m2 := range m2Segs {
			lo.Must0(s.canvas.OccupyM2(m2))
		}
		lo.Must0(s.canvas.OccupyM3(m3))
	}
	pinSegs := make([]Segment, 0, len(s.netlist.Pins))
	for _, pin := range s.netlist.Pins {
		pinSegs = append(pinSegs, Segment{
			LowerLeft:  Point{X: pin.XLow, Y: pin.YLow},
			UpperRight: Point{X: pin.XHigh, Y: pin.YHigh},
			Layer:      common.M1,
			Name:       pin.Name,
			Purpose:    common.Pin,
		})
	}
	if len(pinSegs) > 0 {
		results = append(results, NetResult{Segments: pinSegs})
	}
	return results
}

// appendViaCuts places via cuts in a grid within the overlap of segments a and b,
// inset by endExt on all sides to satisfy end-extension DRC constraints.
func appendViaCuts(segs []Segment, vc ViaConfig, a, b Segment, layer common.Layer, endExt int) []Segment {
	if vc.CutW == 0 || vc.CutH == 0 {
		return segs
	}
	x0 := max(a.LowerLeft.X, b.LowerLeft.X) + endExt
	y0 := max(a.LowerLeft.Y, b.LowerLeft.Y) + endExt
	x1 := min(a.UpperRight.X, b.UpperRight.X) - endExt
	y1 := min(a.UpperRight.Y, b.UpperRight.Y) - endExt
	if x0 >= x1 || y0 >= y1 {
		return segs
	}
	w, h := x1-x0, y1-y0
	cols := max(1, (w+vc.SpaceX)/(vc.CutW+vc.SpaceX))
	rows := max(1, (h+vc.SpaceY)/(vc.CutH+vc.SpaceY))
	startX := (x0+x1)/2 - (cols*vc.CutW+(cols-1)*vc.SpaceX)/2
	startY := (y0+y1)/2 - (rows*vc.CutH+(rows-1)*vc.SpaceY)/2
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			llx := startX + c*(vc.CutW+vc.SpaceX)
			lly := startY + r*(vc.CutH+vc.SpaceY)
			segs = append(segs, Segment{
				LowerLeft:  Point{X: llx, Y: lly},
				UpperRight: Point{X: llx + vc.CutW, Y: lly + vc.CutH},
				Layer:      layer,
				NetID:      a.NetID,
			})
		}
	}
	return segs
}

func pinBBox(pin RoutingPin) Segment {
	return Segment{
		LowerLeft:  Point{X: pin.XLow, Y: pin.YLow},
		UpperRight: Point{X: pin.XHigh, Y: pin.YHigh},
	}
}

func extendM2ToCoverPin(m2 Segment, pin RoutingPin, endExt int) Segment {
	return Segment{
		LowerLeft:  Point{X: m2.LowerLeft.X, Y: min(m2.LowerLeft.Y, pin.YLow-endExt)},
		UpperRight: Point{X: m2.UpperRight.X, Y: max(m2.UpperRight.Y, pin.YHigh+endExt)},
		NetID:      m2.NetID,
	}
}
