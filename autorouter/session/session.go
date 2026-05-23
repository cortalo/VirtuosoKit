package session

import (
	"autorouter/common"
	"encoding/json"

	"github.com/samber/lo"
)

type Point = common.Point
type Shape = common.Shape
type Segment = common.Segment
type TrackSegment = common.TrackSegment
type RoutingPin = common.RoutingPin
type Net = common.Net
type Netlist = common.Netlist
type ViaConfig = common.ViaConfig
type DRCSpec = common.DRCSpec

type Canvas interface {
	Occupy(seg Segment) error
}

type Router interface {
	Route(pins []RoutingPin, netID int) ([]Segment, error)
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
	NetID   int     `json:"net_id"`
	NetName string  `json:"net_name,omitempty"`
	Shapes  []Shape `json:"shapes"`
	Err     error   `json:"-"`
}

func (r NetResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		NetID   int     `json:"net_id"`
		NetName string  `json:"net_name,omitempty"`
		Shapes  []Shape `json:"shapes,omitempty"`
		Error   string  `json:"error,omitempty"`
	}
	w := wire{NetID: r.NetID, NetName: r.NetName, Shapes: r.Shapes}
	if r.Err != nil {
		w.Error = r.Err.Error()
	}
	return json.Marshal(w)
}

func (s *Session) Route() []NetResult {
	results := make([]NetResult, len(s.netlist.Nets))

	for i, net := range s.netlist.Nets {
		// The first len(net.Pins) segments connect each pin's M1 bbox to M2.
		segs, err := s.router.Route(net.Pins, net.ID)
		if err != nil {
			results[i] = NetResult{NetID: net.ID, NetName: net.Name, Err: err}
			continue
		}
		for _, seg := range segs {
			lo.Must0(s.canvas.Occupy(seg))
		}
		shapes := lo.Map(segs, func(seg Segment, _ int) Shape { return seg.ToShape() })
		N := len(shapes)

		for j, pin := range net.Pins {
			shapes = s.appendViaCuts(shapes, shapes[j], pinBBox(pin))
		}
		for ii := range N {
			for jj := ii + 1; jj < N; jj++ {
				shapes = s.appendViaCuts(shapes, shapes[ii], shapes[jj])
			}
		}

		results[i] = NetResult{NetID: net.ID, NetName: net.Name, Shapes: shapes}
	}
	pinShapes := lo.Map(s.netlist.Pins, func(pin *RoutingPin, _ int) Shape {
		return Shape{
			LowerLeft:  Point{X: pin.XLow, Y: pin.YLow},
			UpperRight: Point{X: pin.XHigh, Y: pin.YHigh},
			Layer:      common.M1,
			Purpose:    common.Pin,
			Name:       pin.Name,
		}
	})
	if len(pinShapes) > 0 {
		results = append(results, NetResult{Shapes: pinShapes})
	}
	return results
}

// appendViaCuts detects the via type from a.Layer/b.Layer and places via cuts
// in the overlap region. Unrecognised layer combinations are a no-op.
func (s *Session) appendViaCuts(shapes []Shape, a, b Shape) []Shape {
	var vc ViaConfig
	var layer common.Layer
	var endExt int

	la, lb := a.Layer, b.Layer
	switch {
	case (la == common.M1 && lb == common.M2) || (la == common.M2 && lb == common.M1):
		vc, layer, endExt = s.via12, common.Via12, s.m2DRC.ViaEnclosure()
	case (la == common.M2 && lb == common.M3) || (la == common.M3 && lb == common.M2):
		vc, layer, endExt = s.via23, common.Via23, max(s.m2DRC.ViaEnclosure(), s.m3DRC.ViaEnclosure())
	default:
		return shapes
	}

	if vc.CutW == 0 || vc.CutH == 0 {
		return shapes
	}
	x0 := max(a.LowerLeft.X, b.LowerLeft.X) + endExt
	y0 := max(a.LowerLeft.Y, b.LowerLeft.Y) + endExt
	x1 := min(a.UpperRight.X, b.UpperRight.X) - endExt
	y1 := min(a.UpperRight.Y, b.UpperRight.Y) - endExt
	if x0 >= x1 || y0 >= y1 {
		return shapes
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
			shapes = append(shapes, Shape{
				LowerLeft:  Point{X: llx, Y: lly},
				UpperRight: Point{X: llx + vc.CutW, Y: lly + vc.CutH},
				Layer:      layer,
				NetID:      a.NetID,
			})
		}
	}
	return shapes
}

func pinBBox(pin RoutingPin) Shape {
	layer := pin.Layer
	if layer == 0 {
		layer = common.M1
	}
	return Shape{
		LowerLeft:  Point{X: pin.XLow, Y: pin.YLow},
		UpperRight: Point{X: pin.XHigh, Y: pin.YHigh},
		Layer:      layer,
	}
}
