package canvas

import (
	"autorouter/common"
	"fmt"

	"github.com/samber/lo"
)

type Orient int

const (
	R0 Orient = iota
	R90
	R180
	R270
	MX
	MY
	MXR90
	MYR90
)

func ParseOrient(s string) (Orient, error) {
	switch s {
	case "R0":
		return R0, nil
	case "R90":
		return R90, nil
	case "R180":
		return R180, nil
	case "R270":
		return R270, nil
	case "MX":
		return MX, nil
	case "MY":
		return MY, nil
	case "MXR90":
		return MXR90, nil
	case "MYR90":
		return MYR90, nil
	default:
		return 0, fmt.Errorf("unknown orient: %q", s)
	}
}

type Instance struct {
	XY     Point
	Orient Orient
	Metals []common.Shape // cell-relative coordinates
}

func (inst Instance) AbsoluteMetals() []common.Shape {
	result := make([]common.Shape, len(inst.Metals))
	for i, m := range inst.Metals {
		xLow, xHigh := m.LowerLeft.X, m.UpperRight.X
		yLow, yHigh := m.LowerLeft.Y, m.UpperRight.Y
		var txLow, txHigh, tyLow, tyHigh common.Nm
		switch inst.Orient {
		case R90: // (x,y) → (-y, x)
			txLow, txHigh, tyLow, tyHigh = -yHigh, -yLow, xLow, xHigh
		case R180: // (x,y) → (-x,-y)
			txLow, txHigh, tyLow, tyHigh = -xHigh, -xLow, -yHigh, -yLow
		case R270: // (x,y) → (y,-x)
			txLow, txHigh, tyLow, tyHigh = yLow, yHigh, -xHigh, -xLow
		case MX: // (x,y) → (x,-y)
			txLow, txHigh, tyLow, tyHigh = xLow, xHigh, -yHigh, -yLow
		case MY: // (x,y) → (-x,y)
			txLow, txHigh, tyLow, tyHigh = -xHigh, -xLow, yLow, yHigh
		case MXR90: // (x,y) → (y,x)
			txLow, txHigh, tyLow, tyHigh = yLow, yHigh, xLow, xHigh
		case MYR90: // (x,y) → (-y,-x)
			txLow, txHigh, tyLow, tyHigh = -yHigh, -yLow, -xHigh, -xLow
		case R0:
			txLow, txHigh, tyLow, tyHigh = xLow, xHigh, yLow, yHigh
		default:
			panic("unknown orient")
		}
		result[i] = common.Shape{
			LowerLeft:  Point{X: inst.XY.X + txLow, Y: inst.XY.Y + tyLow},
			UpperRight: Point{X: inst.XY.X + txHigh, Y: inst.XY.Y + tyHigh},
			Layer:      m.Layer,
		}
	}
	return result
}

func NewFullTrackCanvas(
	lowerLeft, upperRight Point,
	m2Storage, m3Storage TrackSegmentStorage,
	m2Dir common.Direction,
	instances []Instance,
) (*FullTrackCanvas, error) {
	c := &FullTrackCanvas{
		LowerLeft:  lowerLeft,
		UpperRight: upperRight,
		M2Storage:  m2Storage,
		M3Storage:  m3Storage,
		M2Dir:      m2Dir,
	}
	for _, inst := range instances {
		for _, shape := range inst.AbsoluteMetals() {
			seg, err := c.NewSeg(shape.Layer, shape.LowerLeft, shape.UpperRight, -1)
			if err != nil {
				return nil, err
			}
			if err := c.Occupy(seg); err != nil {
				return nil, err
			}
		}
	}
	return c, nil
}

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
	ts, err := seg.ToTrack(st.GetTrackWidth())
	if err != nil {
		return err
	}
	return st.Occupy(ts)
}

func (c *FullTrackCanvas) NewTrack(layer common.Layer, trackID, netID int, start, end common.Nm) (TrackSegment, error) {
	st := c.storageFor(layer)
	tw := st.GetTrackWidth()
	dir := lo.Must(c.dirForLayer(layer))
	var numTracks int
	switch dir {
	case common.Horizontal:
		numTracks = int((c.UpperRight.Y - c.LowerLeft.Y) / tw)
	case common.Vertical:
		numTracks = int((c.UpperRight.X - c.LowerLeft.X) / tw)
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

func (c *FullTrackCanvas) GetTrackWidth(layer common.Layer) common.Nm {
	return c.storageFor(layer).GetTrackWidth()
}
