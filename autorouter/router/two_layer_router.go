package router

import (
	"autorouter/common"

	"github.com/samber/lo"
)

type Point = common.Point
type Segment = common.Segment
type TrackSegment = common.TrackSegment
type RoutingPin = common.RoutingPin

type Canvas interface {
	Inbound(p Point) bool
	IsPassible(seg Segment) bool
	IsOccupied(seg Segment) bool
	GetLowerLeft() Point
	GetUpperRight() Point
	GetTrackWidth(layer common.Layer) int
	NewTrack(layer common.Layer, trackID, start, end, netID int) (TrackSegment, error)
	NewSeg(layer common.Layer, ll, ur Point, netID int) (Segment, error)
}

type DRCSpec = common.DRCSpec

type TwoLayerRouter struct {
	canvas          Canvas
	m2Width         int
	m2DRC           DRCSpec
	m3DRC           DRCSpec
	widenNarrowPins bool
}

func NewTwoLayerRouter(c Canvas, m2Width int, m2DRC, m3DRC DRCSpec) *TwoLayerRouter {
	return &TwoLayerRouter{canvas: c, m2Width: m2Width, m2DRC: m2DRC, m3DRC: m3DRC}
}

func (r *TwoLayerRouter) SetWidenNarrowPins(v bool) {
	r.widenNarrowPins = v
}

func (r *TwoLayerRouter) Route(pins []RoutingPin, netID int) ([]Segment, error) {
	var widenedM1s []Segment
	for i, pin := range pins {
		if !r.canvas.Inbound(Point{X: pin.XLow, Y: pin.YLow}) {
			return nil, ErrOutOfBound
		}
		if r.widenNarrowPins && pin.XHigh-pin.XLow < r.m2Width {
			center := (pin.XLow + pin.XHigh) / 2
			pins[i].XLow = center - r.m2Width/2
			pins[i].XHigh = center + r.m2Width/2
			m1Seg, err := r.canvas.NewSeg(
				common.M1,
				Point{X: pins[i].XLow, Y: pin.YLow},
				Point{X: pins[i].XHigh, Y: pin.YHigh},
				netID,
			)
			if err != nil {
				return nil, err
			}
			m1Seg.NoVia = true
			widenedM1s = append(widenedM1s, m1Seg)
		}
	}

	sumY := 0
	for _, pin := range pins {
		sumY += pin.YLow
	}
	midY := sumY / len(pins)

	lowerLeft := r.canvas.GetLowerLeft()
	upperRight := r.canvas.GetUpperRight()
	tw := r.canvas.GetTrackWidth(common.M3)
	midTrack := (midY - lowerLeft.Y) / tw
	maxTrack := (upperRight.Y-lowerLeft.Y)/tw - 1

	for delta := 0; (midTrack+delta <= maxTrack) || (midTrack-delta >= 0); delta++ {
		if segs, ok := r.tryTrack(pins, netID, midTrack+delta); ok {
			return append(segs, widenedM1s...), nil
		}
		if delta > 0 {
			if segs, ok := r.tryTrack(pins, netID, midTrack-delta); ok {
				return append(segs, widenedM1s...), nil
			}
		}
	}
	return nil, ErrNoPath
}

func (r *TwoLayerRouter) m2Passible(seg Segment) bool {
	sp := seg
	sp.LowerLeft.X, sp.UpperRight.X = r.m2DRC.ApplyMinSpaceExtension(seg.LowerLeft.X, seg.UpperRight.X)
	sp.LowerLeft.Y, sp.UpperRight.Y = r.m2DRC.ApplyMinSpaceExtension(seg.LowerLeft.Y, seg.UpperRight.Y)
	return r.canvas.IsPassible(sp) && r.m2DRC.SatisfiesMinArea(seg)
}

func (r *TwoLayerRouter) m3Passible(ts TrackSegment) bool {
	sp := ts
	sp.Start, sp.End = r.m3DRC.ApplyMinSpaceExtension(ts.Start, ts.End)
	return r.canvas.IsPassible(sp.ToSeg()) &&
		(ts.IsFirstTrack() || !r.canvas.IsOccupied(ts.PrevTrack().ToSeg())) &&
		(ts.IsLastTrack() || !r.canvas.IsOccupied(ts.NextTrack().ToSeg()))
}

func (r *TwoLayerRouter) tryTrack(pins []RoutingPin, netID, trackID int) ([]Segment, bool) {
	minX := lo.Min(lo.Map(pins, func(p RoutingPin, _ int) int { return p.XLow }))
	maxXRight := lo.Max(lo.Map(pins, func(p RoutingPin, _ int) int { return max(p.XHigh, p.XLow+r.m2Width) }))

	m3Lo, m3Hi := r.m3DRC.ApplyEndExtension(minX, maxXRight)
	m3, err := r.canvas.NewTrack(common.M3, trackID, m3Lo, m3Hi, netID)
	if err != nil {
		return nil, false
	}
	if !r.m3Passible(m3) {
		return nil, false
	}

	step := r.m2Width / 2
	if step == 0 {
		step = 1
	}

	m2Segs := make([]Segment, len(pins))
	for i, pin := range pins {
		m2Lo, m2Hi, narrowErr := r.pinM2Bounds(pin, m3)
		m2Lo, m2Hi = r.m2DRC.ApplyEndExtension(m2Lo, m2Hi)

		xMax := pin.XLow // narrow pin: only try XLow
		if narrowErr == nil {
			xMax = pin.XHigh - r.m2Width
		}

		found := false
		for xLow := pin.XLow; xLow <= xMax; xLow += step {
			seg, segErr := r.canvas.NewSeg(
				common.M2,
				Point{X: xLow, Y: m2Lo},
				Point{X: xLow + r.m2Width, Y: m2Hi},
				netID,
			)
			if segErr != nil || !r.m2Passible(seg) {
				continue
			}
			m2Segs[i] = seg
			found = true
			break
		}
		if !found {
			return nil, false
		}
	}

	if !r.m3DRC.SatisfiesMinArea(m3.ToSeg()) {
		m2Horiz, err := r.canvas.NewSeg(
			common.M2,
			Point{X: minX, Y: m3.GetLower()},
			Point{X: maxXRight, Y: m3.GetUpper()},
			netID,
		)
		if err != nil || !r.m2Passible(m2Horiz) {
			return nil, false
		}
		return append(m2Segs, m2Horiz), true
	}

	return append(m2Segs, m3.ToSeg()), true
}

// pinM2Bounds returns the raw (before end-extension) Y range for the M2 stub
// connecting pin to m3, and errPinTooNarrow if pin.XHigh-pin.XLow < m2Width
// (caller should not attempt to slide the stub in that case).
func (r *TwoLayerRouter) pinM2Bounds(pin RoutingPin, m3 TrackSegment) (lo, hi int, err error) {
	if pin.XHigh-pin.XLow < r.m2Width {
		err = errPinTooNarrow
	}
	minOv := r.m2DRC.MinPinOverlap()
	if pin.MinOverlap && minOv > 0 {
		pinCenterY := (pin.YLow + pin.YHigh) / 2
		m3CenterY := (m3.GetLower() + m3.GetUpper()) / 2
		if pinCenterY <= m3CenterY {
			return min(m3.GetLower(), max(pin.YLow, min(pin.YHigh, m3.GetUpper())-minOv)), m3.GetUpper(), err
		}
		return m3.GetLower(), max(m3.GetUpper(), min(pin.YHigh, max(pin.YLow, m3.GetLower())+minOv)), err
	}
	return min(pin.YLow, m3.GetLower()), max(pin.YHigh, m3.GetUpper()), err
}
