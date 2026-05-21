package canvas

import (
	"autorouter/common"
	"errors"

	"github.com/samber/lo"
)

type Point = common.Point
type Segment = common.Segment
type TrackSegment = common.TrackSegment

var (
	ErrInvalidTrackID = errors.New("invalid m3 track ID")
	ErrUnknownLayer   = errors.New("cannot occupy segment with unknown layer")
)

type TrackSegmentStorage interface {
	IsPassible(seg TrackSegment) bool
	Occupy(seg TrackSegment) error
	GetM3TrackWidth() int
}

type SegmentStorage interface {
	IsPassible(seg Segment) bool
	Occupy(seg Segment) error
}

type Canvas struct {
	LowerLeft  Point
	UpperRight Point
	M3Storage  TrackSegmentStorage
	M2Storage  SegmentStorage
}

func (c *Canvas) Inbound(p Point) bool {
	return p.X >= c.LowerLeft.X && p.X <= c.UpperRight.X &&
		p.Y >= c.LowerLeft.Y && p.Y <= c.UpperRight.Y
}

func (c *Canvas) IsPassible(seg Segment) bool {
	switch seg.Layer {
	case common.M2:
		return c.M2Storage.IsPassible(seg)
	case common.M3:
		return c.M3Storage.IsPassible(lo.Must(seg.ToTrack(c.M3Storage.GetM3TrackWidth())))
	default:
		panic(ErrUnknownLayer)
		return false
	}
}

func (c *Canvas) Occupy(seg Segment) error {
	switch seg.Layer {
	case common.M2:
		return c.M2Storage.Occupy(seg)
	case common.M3:
		return c.M3Storage.Occupy(lo.Must(seg.ToTrack(c.M3Storage.GetM3TrackWidth())))
	default:
		panic(ErrUnknownLayer)
		return ErrUnknownLayer
	}
}

func (c *Canvas) dirForLayer(layer common.Layer) (common.Direction, error) {
	switch layer {
	case common.M3:
		return common.Horizontal, nil
	case common.M2:
		return common.Vertical, nil
	default:
		panic(ErrUnknownLayer)
		return 0, ErrUnknownLayer
	}
}

func (c *Canvas) trackWidth(layer common.Layer) (int, error) {
	switch layer {
	case common.M3:
		return c.M3Storage.GetM3TrackWidth(), nil
	default:
		panic(ErrUnknownLayer)
		return 0, ErrUnknownLayer
	}
}

func (c *Canvas) NewTrack(layer common.Layer, trackID, start, end, netID int) (TrackSegment, error) {
	tw := lo.Must(c.trackWidth(layer))
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

func (c *Canvas) NewSeg(layer common.Layer, ll, ur Point, netID int) (Segment, error) {
	return Segment{
		LowerLeft:    ll,
		UpperRight:   ur,
		NetID:        netID,
		Layer:        layer,
		CanvasOrigin: c.LowerLeft,
		Dir:          lo.Must(c.dirForLayer(layer)),
	}, nil
}

func (c *Canvas) GetLowerLeft() Point {
	return c.LowerLeft
}

func (c *Canvas) GetUpperRight() Point {
	return c.UpperRight
}

func (c *Canvas) GetM3TrackWidth() int {
	return c.M3Storage.GetM3TrackWidth()
}
