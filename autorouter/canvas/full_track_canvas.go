package canvas

import (
	"autorouter/common"

	"github.com/samber/lo"
)

type FullTrackCanvas struct {
	LowerLeft  Point
	UpperRight Point
	M2Storage  TrackSegmentStorage
	M3Storage  TrackSegmentStorage
	M2Dir      common.Direction
}

func (c *FullTrackCanvas) Inbound(p Point) bool {
	return p.X >= c.LowerLeft.X && p.X <= c.UpperRight.X &&
		p.Y >= c.LowerLeft.Y && p.Y <= c.UpperRight.Y
}

func (c *FullTrackCanvas) dirForLayer(layer common.Layer) (common.Direction, error) {
	switch layer {
	case common.M2:
		return c.M2Dir, nil
	case common.M3:
		return c.M2Dir.Perpendicular(), nil
	default:
		panic(ErrUnknownLayer)
		return 0, ErrUnknownLayer
	}
}

func (c *FullTrackCanvas) storageFor(layer common.Layer) TrackSegmentStorage {
	switch layer {
	case common.M2:
		return c.M2Storage
	case common.M3:
		return c.M3Storage
	default:
		panic(ErrUnknownLayer)
	}
}

func (c *FullTrackCanvas) IsPassible(seg Segment) bool {
	st := c.storageFor(seg.Layer)
	return st.IsPassible(lo.Must(seg.ToTrack(st.GetTrackWidth())))
}

func (c *FullTrackCanvas) IsOccupied(seg Segment) bool {
	st := c.storageFor(seg.Layer)
	return st.IsOccupied(lo.Must(seg.ToTrack(st.GetTrackWidth())))
}

func (c *FullTrackCanvas) Occupy(seg Segment) error {
	st := c.storageFor(seg.Layer)
	return st.Occupy(lo.Must(seg.ToTrack(st.GetTrackWidth())))
}

func (c *FullTrackCanvas) NewTrack(layer common.Layer, trackID, start, end, netID int) (TrackSegment, error) {
	st := c.storageFor(layer)
	tw := st.GetTrackWidth()
	dir := lo.Must(c.dirForLayer(layer))
	var numTracks int
	switch dir {
	case common.Horizontal:
		numTracks = (c.UpperRight.Y - c.LowerLeft.Y) / tw
	case common.Vertical:
		numTracks = (c.UpperRight.X - c.LowerLeft.X) / tw
	default:
		panic(ErrUnknownLayer)
	}
	if trackID < 0 || trackID >= numTracks {
		return TrackSegment{}, ErrInvalidTrackID
	}
	return TrackSegment{
		TrackID:      trackID,
		Start:        start,
		End:          end,
		NetID:        netID,
		Layer:        layer,
		CanvasOrigin: c.LowerLeft,
		Width:        tw,
		NumTracks:    numTracks,
		Dir:          dir,
	}, nil
}

func (c *FullTrackCanvas) NewSeg(layer common.Layer, ll, ur Point, netID int) (Segment, error) {
	return Segment{
		LowerLeft:    ll,
		UpperRight:   ur,
		NetID:        netID,
		Layer:        layer,
		CanvasOrigin: c.LowerLeft,
		Dir:          lo.Must(c.dirForLayer(layer)),
	}, nil
}

func (c *FullTrackCanvas) GetLowerLeft() Point {
	return c.LowerLeft
}

func (c *FullTrackCanvas) GetUpperRight() Point {
	return c.UpperRight
}

func (c *FullTrackCanvas) GetTrackWidth(layer common.Layer) int {
	return c.storageFor(layer).GetTrackWidth()
}
