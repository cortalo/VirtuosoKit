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
	canvas  Canvas
	m2Width int
	m2DRC   DRCSpec
	m3DRC   DRCSpec
}

func NewTwoLayerRouter(c Canvas, m2Width int, m2DRC, m3DRC DRCSpec) *TwoLayerRouter {
	return &TwoLayerRouter{canvas: c, m2Width: m2Width, m2DRC: m2DRC, m3DRC: m3DRC}
}

func (r *TwoLayerRouter) Route(pins []RoutingPin, netID int) ([]Segment, error) {
	for _, pin := range pins {
		if !r.canvas.Inbound(Point{X: pin.XLow, Y: pin.YLow}) {
			return nil, ErrOutOfBound
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
			return segs, nil
		}
		if delta > 0 {
			if segs, ok := r.tryTrack(pins, netID, midTrack-delta); ok {
				return segs, nil
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
		m2Lo, m2Hi := r.m2DRC.ApplyEndExtension(min(pin.YLow, m3.GetLower()), max(pin.YHigh, m3.GetUpper()))
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
			pinLo, pinHi := r.m2DRC.ApplyEndExtension(pin.YLow, pin.YHigh)
			yLow := m3.GetLower()
			if pin.YLow < m3.GetLower() {
				yLow = pinLo
			}
			yHigh := m3.GetUpper()
			if pin.YHigh > m3.GetUpper() {
				yHigh = pinHi
			}
			m2Segs[i], err = r.canvas.NewSeg(
				common.M2,
				Point{X: pin.XLow, Y: yLow},
				Point{X: pin.XLow + r.m2Width, Y: yHigh},
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
