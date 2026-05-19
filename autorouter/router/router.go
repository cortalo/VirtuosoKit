package router

import (
	"autorouter/common"
)

type Point = common.Point
type Segment = common.Segment
type TrackSegment = common.TrackSegment

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

func (r *TwoLayerRouter) Route(from, to Point, netID int) (Segment, Segment, TrackSegment, error) {
	if !r.canvas.Inbound(from) || !r.canvas.Inbound(to) {
		return Segment{}, Segment{}, TrackSegment{}, ErrOutOfBound
	}
	midY := (from.Y + to.Y) / 2
	lowerLeft := r.canvas.GetLowerLeft()
	upperRight := r.canvas.GetUpperRight()
	midTrack := (midY - lowerLeft.Y) / r.canvas.GetM3TrackWidth()
	maxTrack := (upperRight.Y-lowerLeft.Y)/r.canvas.GetM3TrackWidth() - 1
	for delta := 0; (midTrack+delta <= maxTrack) || (midTrack-delta >= 0); delta++ {
		m2From, m2To, m3, success := r.tryTrack(from, to, netID, midTrack+delta)
		if success {
			return m2From, m2To, m3, nil
		}
		m2From, m2To, m3, success = r.tryTrack(from, to, netID, midTrack-delta)
		if success {
			return m2From, m2To, m3, nil
		}
	}
	return Segment{}, Segment{}, TrackSegment{}, ErrNoPath
}

func (r *TwoLayerRouter) tryTrack(from, to Point, netID, trackID int) (Segment, Segment, TrackSegment, bool) {
	lowerLeft := r.canvas.GetLowerLeft()
	upperRight := r.canvas.GetUpperRight()
	maxTrack := (upperRight.Y-lowerLeft.Y)/r.canvas.GetM3TrackWidth() - 1
	if trackID < 0 || trackID > maxTrack {
		return Segment{}, Segment{}, TrackSegment{}, false
	}

	trackYLower := lowerLeft.Y + trackID*r.canvas.GetM3TrackWidth()
	trackYUpper := lowerLeft.Y + (trackID+1)*r.canvas.GetM3TrackWidth()
	m2From := Segment{
		LowerLeft:  Point{X: from.X, Y: min(from.Y, trackYLower)},
		UpperRight: Point{X: from.X + r.m2Width, Y: max(from.Y, trackYUpper)},
		NetID:      netID,
	}
	m2To := Segment{
		LowerLeft:  Point{X: to.X, Y: min(to.Y, trackYLower)},
		UpperRight: Point{X: to.X + r.m2Width, Y: max(to.Y, trackYUpper)},
		NetID:      netID,
	}
	m3 := TrackSegment{
		TrackID: trackID,
		Start:   min(from.X, to.X),
		End:     max(from.X, to.X) + r.m2Width,
		NetID:   netID,
	}

	spacingOK := true
	if trackID > 0 {
		spacingOK = r.canvas.IsPassibleM3(TrackSegment{TrackID: trackID - 1, Start: m3.Start, End: m3.End, NetID: netID})
	}
	if trackID < maxTrack {
		spacingOK = spacingOK && r.canvas.IsPassibleM3(TrackSegment{TrackID: trackID + 1, Start: m3.Start, End: m3.End, NetID: netID})
	}

	m2FromArea := (m2From.UpperRight.X - m2From.LowerLeft.X) * (m2From.UpperRight.Y - m2From.LowerLeft.Y)
	m2ToArea := (m2To.UpperRight.X - m2To.LowerLeft.X) * (m2To.UpperRight.Y - m2To.LowerLeft.Y)
	m3Area := (m3.End - m3.Start) * r.canvas.GetM3TrackWidth()

	return m2From, m2To, m3, r.canvas.IsPassibleM2(m2From) &&
		r.canvas.IsPassibleM2(m2To) &&
		r.canvas.IsPassibleM3(m3) &&
		spacingOK &&
		m2FromArea >= r.m2DRC.MinArea() &&
		m2ToArea >= r.m2DRC.MinArea() &&
		m3Area >= r.m3DRC.MinArea()
}
