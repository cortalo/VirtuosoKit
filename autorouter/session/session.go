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
	canvas      Canvas
	router      Router
	powerRouter Router
	powerNets   map[string]bool
	netlist     *Netlist
	via12       ViaConfig
	via23       ViaConfig
	m2DRC       DRCSpec
	m3DRC       DRCSpec
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

func (s *Session) SetPowerRouter(r Router, netNames ...string) {
	s.powerRouter = r
	s.powerNets = make(map[string]bool, len(netNames))
	for _, name := range netNames {
		s.powerNets[name] = true
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
	nets := make([]*Net, 0, len(s.netlist.Nets))
	for _, net := range s.netlist.Nets {
		if s.powerNets[net.Name] {
			nets = append(nets, net)
		}
	}
	for _, net := range s.netlist.Nets {
		if !s.powerNets[net.Name] {
			nets = append(nets, net)
		}
	}

	results := make([]NetResult, len(nets))

	for i, net := range nets {
		r := s.router
		if s.powerNets[net.Name] {
			if s.powerRouter == nil {
				panic("power net " + net.Name + " requires powerRouter but none is set via SetPowerRouter")
			}
			r = s.powerRouter
		}
		// The first len(net.Pins) segments connect each pin's M1 bbox to M2.
		segs, err := r.Route(net.Pins, net.ID)
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
			if shapes[j].NoVia {
				continue
			}
			shapes = s.appendViaCuts(shapes, shapes[j], pinBBox(pin))
		}
		for ii := range N {
			if shapes[ii].NoVia {
				continue
			}
			for jj := ii + 1; jj < N; jj++ {
				if shapes[jj].NoVia {
					continue
				}
				shapes = s.appendViaCuts(shapes, shapes[ii], shapes[jj])
			}
		}

		results[i] = NetResult{NetID: net.ID, NetName: net.Name, Shapes: shapes}
	}
	pinShapes := lo.Map(s.netlist.Pins, func(pin *RoutingPin, _ int) Shape {
		layer := pin.Layer
		if layer == 0 {
			layer = common.M1
		}
		return Shape{
			LowerLeft:  Point{X: pin.XLow, Y: pin.YLow},
			UpperRight: Point{X: pin.XHigh, Y: pin.YHigh},
			Layer:      layer,
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
	la, lb := a.Layer, b.Layer
	switch {
	case (la == common.M1 && lb == common.M2) || (la == common.M2 && lb == common.M1):
		return common.PlaceViaCuts(shapes, a, b, s.via12, common.Via12, s.m2DRC.ViaEnclosure())
	case (la == common.M2 && lb == common.M3) || (la == common.M3 && lb == common.M2):
		return common.PlaceViaCuts(shapes, a, b, s.via23, common.Via23, max(s.m2DRC.ViaEnclosure(), s.m3DRC.ViaEnclosure()))
	default:
		return shapes
	}
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
