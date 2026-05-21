package common

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrTrackMisaligned    = errors.New("segment is not aligned to track grid")
	ErrTrackWidthMismatch = errors.New("segment width does not match track width")
	ErrUnknownDirection   = errors.New("segment direction not set")
)

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Layer int

const (
	M1 Layer = iota + 1
	M2
	M3
	Via12
	Via23
)

func (l Layer) MarshalJSON() ([]byte, error) {
	switch l {
	case M1:
		return json.Marshal("M1")
	case M2:
		return json.Marshal("M2")
	case M3:
		return json.Marshal("M3")
	case Via12:
		return json.Marshal("Via12")
	case Via23:
		return json.Marshal("Via23")
	default:
		return nil, fmt.Errorf("unknown layer: %d", int(l))
	}
}

func (l *Layer) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "M1":
		*l = M1
	case "M2":
		*l = M2
	case "M3":
		*l = M3
	case "Via12":
		*l = Via12
	case "Via23":
		*l = Via23
	default:
		return fmt.Errorf("unknown layer: %q", s)
	}
	return nil
}

type Direction int

const (
	UnknownDirection Direction = iota // zero value — means Dir was never set
	Horizontal
	Vertical
)

type Purpose int

const (
	Drawing Purpose = iota
	Pin
)

func (p Purpose) MarshalJSON() ([]byte, error) {
	switch p {
	case Drawing:
		return json.Marshal("drawing")
	case Pin:
		return json.Marshal("pin")
	default:
		return nil, fmt.Errorf("unknown purpose: %d", int(p))
	}
}

func (p *Purpose) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "drawing":
		*p = Drawing
	case "pin":
		*p = Pin
	default:
		return fmt.Errorf("unknown purpose: %q", s)
	}
	return nil
}

// ViaConfig holds DRC parameters for a single via type.
// All dimensions are in nm.
type ViaConfig struct {
	ViaDef string // via definition name passed to the layout tool (e.g. "M3_M2")
	CutW   int    // cut width
	CutH   int    // cut height
	SpaceX int    // cut-to-cut spacing in X
	SpaceY int    // cut-to-cut spacing in Y
}

type Shape struct {
	LowerLeft  Point   `json:"lower_left"`
	UpperRight Point   `json:"upper_right"`
	NetID      int     `json:"net_id"`
	Layer      Layer   `json:"layer"`
	Purpose    Purpose `json:"purpose"`
}

type Segment struct {
	LowerLeft    Point     `json:"lower_left"`
	UpperRight   Point     `json:"upper_right"`
	NetID        int       `json:"net_id"`
	Layer        Layer     `json:"layer"`
	CanvasOrigin Point     `json:"-"`
	Dir          Direction `json:"-"`
}

func (seg Segment) ToShape() Shape {
	return Shape{
		LowerLeft:  seg.LowerLeft,
		UpperRight: seg.UpperRight,
		NetID:      seg.NetID,
		Layer:      seg.Layer,
		Purpose:    Drawing,
	}
}

func (s Segment) GetArea() int {
	return (s.UpperRight.X - s.LowerLeft.X) * (s.UpperRight.Y - s.LowerLeft.Y)
}

func (s Segment) Overlap(other Segment) bool {
	return s.LowerLeft.X < other.UpperRight.X && s.UpperRight.X > other.LowerLeft.X &&
		s.LowerLeft.Y < other.UpperRight.Y && s.UpperRight.Y > other.LowerLeft.Y
}

type TrackSegment struct {
	TrackID      int       `json:"track_id"`
	Start        int       `json:"start"`
	End          int       `json:"end"`
	NetID        int       `json:"net_id"`
	Layer        Layer     `json:"-"`
	CanvasOrigin Point     `json:"-"`
	Width        int       `json:"-"`
	NumTracks    int       `json:"-"`
	Dir          Direction `json:"-"`
}

func (ts TrackSegment) IsFirstTrack() bool { return ts.TrackID == 0 }
func (ts TrackSegment) IsLastTrack() bool  { return ts.TrackID == ts.NumTracks-1 }

func (ts TrackSegment) PrevTrack() TrackSegment {
	ts.TrackID--
	return ts
}

func (ts TrackSegment) NextTrack() TrackSegment {
	ts.TrackID++
	return ts
}

// GetLower returns the lower perpendicular coordinate of the track
// (Y for horizontal tracks, X for vertical tracks).
func (ts TrackSegment) GetLower() int {
	switch ts.Dir {
	case Horizontal:
		return ts.CanvasOrigin.Y + ts.TrackID*ts.Width
	case Vertical:
		return ts.CanvasOrigin.X + ts.TrackID*ts.Width
	default:
		panic(ErrUnknownDirection)
	}
}

func (ts TrackSegment) GetUpper() int { return ts.GetLower() + ts.Width }

func (ts TrackSegment) GetArea() int { return (ts.End - ts.Start) * ts.Width }

// RoutingPin is a physical pin access point from the router's perspective.
// XLow/YLow is the bottom-left corner of the pin bbox; XHigh/YHigh is the top-right.
// The session extends M2 to cover the full Y range and computes M1-M2 vias from the bbox.
// Name is non-empty only for top-level schematic pins (ports of the cell being designed).
type RoutingPin struct {
	Name  string
	XLow  int
	XHigh int
	YLow  int
	YHigh int
}

type Net struct {
	ID   int
	Name string
	Pins []RoutingPin
}

// Netlist holds everything the router needs: the internal nets to route and
// the top-level port pins that need to be placed in the layout but are not
// routed (single-pin nets from the schematic's pins section).
type Netlist struct {
	Nets []*Net
	Pins []*RoutingPin
}

type DRCSpec interface {
	MinArea() int
	EndExtension() int
	ViaEnclosure() int
}

// NoDRC is a DRCSpec with no constraints, used when DRC rules are not configured.
type NoDRC struct{}

func (NoDRC) MinArea() int      { return 0 }
func (NoDRC) EndExtension() int { return 0 }
func (NoDRC) ViaEnclosure() int { return 0 }

// ToTrack converts a Segment to a TrackSegment using seg.Dir and seg.CanvasOrigin.
// Horizontal: TrackID from Y, Start/End are X coordinates.
// Vertical:   TrackID from X, Start/End are Y coordinates.
// Returns ErrTrackMisaligned if the segment is not on the track grid.
func (seg Segment) ToTrack(tw int) (TrackSegment, error) {
	var offset, start, end, segWidth int
	switch seg.Dir {
	case Horizontal:
		offset = seg.LowerLeft.Y - seg.CanvasOrigin.Y
		start, end = seg.LowerLeft.X, seg.UpperRight.X
		segWidth = seg.UpperRight.Y - seg.LowerLeft.Y
	case Vertical:
		offset = seg.LowerLeft.X - seg.CanvasOrigin.X
		start, end = seg.LowerLeft.Y, seg.UpperRight.Y
		segWidth = seg.UpperRight.X - seg.LowerLeft.X
	default:
		return TrackSegment{}, ErrUnknownDirection
	}
	if offset < 0 || offset%tw != 0 {
		return TrackSegment{}, ErrTrackMisaligned
	}
	if segWidth != tw {
		return TrackSegment{}, ErrTrackWidthMismatch
	}
	return TrackSegment{
		TrackID:      offset / tw,
		Start:        start,
		End:          end,
		NetID:        seg.NetID,
		Layer:        seg.Layer,
		CanvasOrigin: seg.CanvasOrigin,
		Width:        tw,
		Dir:          seg.Dir,
	}, nil
}

// ToSeg converts a TrackSegment back to a Segment using ts.Dir, ts.CanvasOrigin, ts.Width, and ts.Layer.
// Horizontal: Y from TrackID, X from Start/End.
// Vertical:   X from TrackID, Y from Start/End.
func (ts TrackSegment) ToSeg() Segment {
	var ll, ur Point
	switch ts.Dir {
	case Horizontal:
		yLow := ts.CanvasOrigin.Y + ts.TrackID*ts.Width
		ll = Point{X: ts.Start, Y: yLow}
		ur = Point{X: ts.End, Y: yLow + ts.Width}
	case Vertical:
		xLow := ts.CanvasOrigin.X + ts.TrackID*ts.Width
		ll = Point{X: xLow, Y: ts.Start}
		ur = Point{X: xLow + ts.Width, Y: ts.End}
	case UnknownDirection:
		panic(ErrUnknownDirection)
	default:
		panic(ErrUnknownDirection)
	}
	return Segment{
		LowerLeft:    ll,
		UpperRight:   ur,
		NetID:        ts.NetID,
		Layer:        ts.Layer,
		CanvasOrigin: ts.CanvasOrigin,
		Dir:          ts.Dir,
	}
}
