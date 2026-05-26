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

func (r *TwoLayerRouter) tryTrack(pins []RoutingPin, netID, trackID int) ([]Segment, bool) {

	xLows := lo.Map(pins, func(p RoutingPin, _ int) int { return p.XLow })
	minX := lo.Min(xLows)
	maxX := lo.Max(xLows)

	m3Lo, m3Hi := r.m3DRC.ApplyEndExtension(minX, maxX+r.m2Width)
	m3, err := r.canvas.NewTrack(common.M3, trackID, m3Lo, m3Hi, netID)
	if err != nil {
		return nil, false
	}
	m3Space := m3
	m3Space.Start, m3Space.End = r.m3DRC.ApplyMinSpaceExtension(m3.Start, m3.End)
	if !r.canvas.IsPassible(m3Space.ToSeg()) ||
		(!m3.IsFirstTrack() && r.canvas.IsOccupied(m3.PrevTrack().ToSeg())) ||
		(!m3.IsLastTrack() && r.canvas.IsOccupied(m3.NextTrack().ToSeg())) {
		return nil, false
	}

	m2Segs := make([]Segment, len(pins))
	for i, pin := range pins {
		m2Lo, m2Hi := r.m2DRC.ApplyEndExtension(r.pinM2Bounds(pin, m3))
		m2Segs[i], err = r.canvas.NewSeg(
			common.M2,
			Point{X: pin.XLow, Y: m2Lo},
			Point{X: pin.XLow + r.m2Width, Y: m2Hi},
			netID,
		)
		m2Space := m2Segs[i]
		m2Space.LowerLeft.Y, m2Space.UpperRight.Y = r.m2DRC.ApplyMinSpaceExtension(m2Segs[i].LowerLeft.Y, m2Segs[i].UpperRight.Y)
		if err != nil || !r.canvas.IsPassible(m2Space) || !r.m2DRC.SatisfiesMinArea(m2Segs[i]) {
			return nil, false
		}
	}

	if !r.m3DRC.SatisfiesMinArea(m3.ToSeg()) {
		for i, pin := range pins {
			m2Lo, m2Hi := r.m2DRC.ApplyEndExtension(r.pinM2Bounds(pin, m3))
			m2Segs[i], err = r.canvas.NewSeg(
				common.M2,
				Point{X: pin.XLow, Y: m2Lo},
				Point{X: pin.XLow + r.m2Width, Y: m2Hi},
				netID,
			)
			if err != nil {
				return nil, false
			}
		}
		m2Horiz, err := r.canvas.NewSeg(
			common.M2,
			Point{X: minX, Y: m3.GetLower()},
			Point{X: maxX + r.m2Width, Y: m3.GetUpper()},
			netID,
		)
		if err != nil {
			return nil, false
		}
		m2HorizSpace := m2Horiz
		m2HorizSpace.LowerLeft.X, m2HorizSpace.UpperRight.X = r.m2DRC.ApplyMinSpaceExtension(m2Horiz.LowerLeft.X, m2Horiz.UpperRight.X)
		if !r.canvas.IsPassible(m2HorizSpace) {
			return nil, false
		}
		return append(m2Segs, m2Horiz), true
	}

	return append(m2Segs, m3.ToSeg()), true
}

// pinM2Bounds returns the raw (before end-extension) Y range for the M2 stub
// connecting pin to m3. When pin.MinOverlap is set and MinPinOverlap() > 0,
// M2 enters the pin bbox by only MinPinOverlap() nm from the nearest edge;
// otherwise the full pin height is covered.
func (r *TwoLayerRouter) pinM2Bounds(pin RoutingPin, m3 TrackSegment) (lo, hi int) {
	minOv := r.m2DRC.MinPinOverlap()
	if pin.MinOverlap && minOv > 0 {
		pinCenterY := (pin.YLow + pin.YHigh) / 2
		m3CenterY := (m3.GetLower() + m3.GetUpper()) / 2
		if pinCenterY <= m3CenterY {
			// pin center is below M3 center: enter from the near (top) edge of the pin.
			// Cap the entry point at m3.GetUpper() so the overlap is measured against the
			// actual M2 top, not a pin.YHigh that extends past the M3 track.
			// Also clamp lo to at most m3.GetLower() so M2 always covers the full M3 track.
			return min(m3.GetLower(), max(pin.YLow, min(pin.YHigh, m3.GetUpper())-minOv)), m3.GetUpper()
		}
		// pin center is above M3 center: enter from the near (bottom) edge of the pin.
		// Cap the entry point at m3.GetLower() symmetrically.
		// Also clamp hi to at least m3.GetUpper() so M2 always covers the full M3 track.
		return m3.GetLower(), max(m3.GetUpper(), min(pin.YHigh, max(pin.YLow, m3.GetLower())+minOv))
	}
	return min(pin.YLow, m3.GetLower()), max(pin.YHigh, m3.GetUpper())
}
