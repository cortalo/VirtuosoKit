package router

import (
	"autorouter/common"
)

type Point = common.Point
type Segment = common.Segment
type TrackSegment = common.TrackSegment
type RoutingPin = common.RoutingPin

type Canvas interface {
	Inbound(p Point) bool
	IsPassibleM2(seg Segment) bool
	IsPassibleM3(seg TrackSegment) bool
	GetLowerLeft() Point
	GetUpperRight() Point
	GetM3TrackWidth() int
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

func (r *TwoLayerRouter) Route(pins []RoutingPin, netID int) ([]Segment, TrackSegment, error) {
	for _, pin := range pins {
		if !r.canvas.Inbound(Point{X: pin.XLow, Y: pin.YLow}) {
			return nil, TrackSegment{}, ErrOutOfBound
		}
	}

	sumY := 0
	for _, pin := range pins {
		sumY += pin.YLow
	}
	midY := sumY / len(pins)

	lowerLeft := r.canvas.GetLowerLeft()
	upperRight := r.canvas.GetUpperRight()
	midTrack := (midY - lowerLeft.Y) / r.canvas.GetM3TrackWidth()
	maxTrack := (upperRight.Y-lowerLeft.Y)/r.canvas.GetM3TrackWidth() - 1

	for delta := 0; (midTrack+delta <= maxTrack) || (midTrack-delta >= 0); delta++ {
		if m2Segs, m3, ok := r.tryTrack(pins, netID, midTrack+delta); ok {
			return m2Segs, m3, nil
		}
		if delta > 0 {
			if m2Segs, m3, ok := r.tryTrack(pins, netID, midTrack-delta); ok {
				return m2Segs, m3, nil
			}
		}
	}
	return nil, TrackSegment{}, ErrNoPath
}

func (r *TwoLayerRouter) tryTrack(pins []RoutingPin, netID, trackID int) ([]Segment, TrackSegment, bool) {
	lowerLeft := r.canvas.GetLowerLeft()
	upperRight := r.canvas.GetUpperRight()
	maxTrack := (upperRight.Y-lowerLeft.Y)/r.canvas.GetM3TrackWidth() - 1
	if trackID < 0 || trackID > maxTrack {
		return nil, TrackSegment{}, false
	}

	trackYLower := lowerLeft.Y + trackID*r.canvas.GetM3TrackWidth()
	trackYUpper := lowerLeft.Y + (trackID+1)*r.canvas.GetM3TrackWidth()
	m2Ext := r.m2DRC.EndExtension()
	m3Ext := r.m3DRC.EndExtension()

	m2Segs := make([]Segment, len(pins))
	minX, maxX := pins[0].XLow, pins[0].XLow
	for i, pin := range pins {
		m2Segs[i] = Segment{
			LowerLeft:  Point{X: pin.XLow, Y: min(pin.YLow, trackYLower) - m2Ext},
			UpperRight: Point{X: pin.XLow + r.m2Width, Y: max(pin.YLow, trackYUpper) + m2Ext},
			NetID:      netID,
		}
		if pin.XLow < minX {
			minX = pin.XLow
		}
		if pin.XLow > maxX {
			maxX = pin.XLow
		}
	}

	m3 := TrackSegment{
		TrackID: trackID,
		Start:   minX - m3Ext,
		End:     maxX + r.m2Width + m3Ext,
		NetID:   netID,
	}

	spacingOK := true
	if trackID > 0 {
		spacingOK = r.canvas.IsPassibleM3(TrackSegment{TrackID: trackID - 1, Start: m3.Start, End: m3.End, NetID: netID})
	}
	if trackID < maxTrack {
		spacingOK = spacingOK && r.canvas.IsPassibleM3(TrackSegment{TrackID: trackID + 1, Start: m3.Start, End: m3.End, NetID: netID})
	}
	if !spacingOK || !r.canvas.IsPassibleM3(m3) {
		return nil, TrackSegment{}, false
	}

	m3Area := (m3.End - m3.Start) * r.canvas.GetM3TrackWidth()
	if m3Area < r.m3DRC.MinArea() {
		return nil, TrackSegment{}, false
	}

	for _, m2 := range m2Segs {
		if !r.canvas.IsPassibleM2(m2) {
			return nil, TrackSegment{}, false
		}
		m2Area := (m2.UpperRight.X - m2.LowerLeft.X) * (m2.UpperRight.Y - m2.LowerLeft.Y)
		if m2Area < r.m2DRC.MinArea() {
			return nil, TrackSegment{}, false
		}
	}

	return m2Segs, m3, true
}
