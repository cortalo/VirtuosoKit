package canvas

import (
	"autorouter/common"

	"github.com/samber/lo"
)

type TwoLayerCanvas struct {
	LowerLeft  Point
	UpperRight Point
	M3Storage  TrackSegmentStorage
	M2Storage  SegmentStorage
}

func (c *TwoLayerCanvas) Inbound(p Point) bool {
	return p.X >= c.LowerLeft.X && p.X <= c.UpperRight.X &&
		p.Y >= c.LowerLeft.Y && p.Y <= c.UpperRight.Y
}

func (c *TwoLayerCanvas) IsPassible(seg Segment) bool {
	switch seg.Layer {
	case common.M2:
		return c.M2Storage.IsPassible(seg)
	case common.M3:
		return c.M3Storage.IsPassible(lo.Must(seg.ToTrack(c.M3Storage.GetTrackWidth())))
	default:
		panic(ErrUnknownLayer)
		return false
	}
}

func (c *TwoLayerCanvas) IsOccupied(seg Segment) bool {
	switch seg.Layer {
	case common.M3:
		return c.M3Storage.IsOccupied(lo.Must(seg.ToTrack(c.M3Storage.GetTrackWidth())))
	default:
		panic(ErrUnknownLayer)
		return false
	}
}

func (c *TwoLayerCanvas) Occupy(seg Segment) error {
	switch seg.Layer {
	case common.M2:
		return c.M2Storage.Occupy(seg)
	case common.M3:
		ts, err := seg.ToTrack(c.M3Storage.GetTrackWidth())
		if err != nil {
			return err
		}
		return c.M3Storage.Occupy(ts)
	case common.M1:
		return nil
	default:
		return ErrUnknownLayer
	}
}

func (c *TwoLayerCanvas) dirForLayer(layer common.Layer) (common.Direction, error) {
	switch layer {
	case common.M3:
		return common.Horizontal, nil
	case common.M2:
		return common.Vertical, nil
	case common.M1:
		return common.UnknownDirection, nil
	default:
		panic(ErrUnknownLayer)
		return 0, ErrUnknownLayer
	}
}

func (c *TwoLayerCanvas) trackWidth(layer common.Layer) (int, error) {
	switch layer {
	case common.M3:
		return c.M3Storage.GetTrackWidth(), nil
	default:
		panic(ErrUnknownLayer)
		return 0, ErrUnknownLayer
	}
}

func (c *TwoLayerCanvas) NewTrack(layer common.Layer, trackID, start, end, netID int) (TrackSegment, error) {
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

func (c *TwoLayerCanvas) NewSeg(layer common.Layer, ll, ur Point, netID int) (Segment, error) {
	return Segment{
		LowerLeft:    ll,
		UpperRight:   ur,
		NetID:        netID,
		Layer:        layer,
		CanvasOrigin: c.LowerLeft,
		Dir:          lo.Must(c.dirForLayer(layer)),
	}, nil
}

func (c *TwoLayerCanvas) GetLowerLeft() Point {
	return c.LowerLeft
}

func (c *TwoLayerCanvas) GetUpperRight() Point {
	return c.UpperRight
}

func (c *TwoLayerCanvas) GetTrackWidth(layer common.Layer) int {
	if layer != common.M3 {
		panic(ErrUnknownLayer)
	}
	return c.M3Storage.GetTrackWidth()
}
