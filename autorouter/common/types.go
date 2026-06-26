package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// Nm represents a physical distance in nanometres.
type Nm int

// Micron represents a physical distance in micrometres.
type Micron float64

func (m Micron) ToNm() Nm     { return Nm(math.Round(float64(m) * 1000)) }
func (n Nm) ToMicron() Micron { return Micron(float64(n) / 1000) }

var (
	ErrTrackMisaligned    = errors.New("segment is not aligned to track grid")
	ErrTrackWidthMismatch = errors.New("segment width does not match track width")
	ErrUnknownDirection   = errors.New("segment direction not set")
	ErrPinNotFound        = errors.New("pin not found")
)

type Point struct {
	X Nm `json:"x"`
	Y Nm `json:"y"`
}

type Layer int

const (
	PC Layer = iota + 1
	M1
	M2
	M3
	M4
	M5
	M6
	Contact // poly contact cut (CC/CNT) between PC and M1
	Via12
	Via23
)

func (l Layer) MarshalJSON() ([]byte, error) {
	switch l {
	case PC:
		return json.Marshal("PC")
	case M1:
		return json.Marshal("M1")
	case M2:
		return json.Marshal("M2")
	case M3:
		return json.Marshal("M3")
	case M4:
		return json.Marshal("M4")
	case M5:
		return json.Marshal("M5")
	case M6:
		return json.Marshal("M6")
	case Contact:
		return json.Marshal("Contact")
	case Via12:
		return json.Marshal("Via12")
	case Via23:
		return json.Marshal("Via23")
	default:
		return nil, fmt.Errorf("unknown layer: %d", int(l))
	}
}

func ParseLayer(s string) (Layer, error) {
	switch s {
	case "PC":
		return PC, nil
	case "M1", "METAL1":
		return M1, nil
	case "M2", "METAL2":
		return M2, nil
	case "M3", "METAL3":
		return M3, nil
	case "M4", "METAL4":
		return M4, nil
	case "M5", "METAL5":
		return M5, nil
	case "M6", "METAL6":
		return M6, nil
	case "Contact":
		return Contact, nil
	case "Via12":
		return Via12, nil
	case "Via23":
		return Via23, nil
	default:
		return 0, fmt.Errorf("unknown layer: %q", s)
	}
}

func (l *Layer) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	layer, err := ParseLayer(s)
	if err != nil {
		return err
	}
	*l = layer
	return nil
}

type Direction int

const (
	UnknownDirection Direction = iota // zero value — means Dir was never set
	Horizontal
	Vertical
)

func (d Direction) Perpendicular() Direction {
	switch d {
	case Horizontal:
		return Vertical
	case Vertical:
		return Horizontal
	default:
		panic(ErrUnknownDirection)
	}
}

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
	CutW   Nm     // cut width
	CutH   Nm     // cut height
	SpaceX Nm     // cut-to-cut spacing in X
	SpaceY Nm     // cut-to-cut spacing in Y
}

type Shape struct {
	LowerLeft  Point   `json:"lower_left"`
	UpperRight Point   `json:"upper_right"`
	NetID      int     `json:"net_id"`
	Layer      Layer   `json:"layer"`
	Purpose    Purpose `json:"purpose"`
	Name       string  `json:"name,omitempty"`
	NoViaUp    bool    `json:"-"`
	NoViaDown  bool    `json:"-"`
}

type Segment struct {
	LowerLeft    Point     `json:"lower_left"`
	UpperRight   Point     `json:"upper_right"`
	NetID        int       `json:"net_id"`
	Layer        Layer     `json:"layer"`
	CanvasOrigin Point     `json:"-"`
	Dir          Direction `json:"-"`
	NoViaUp      bool      `json:"-"`
	NoViaDown    bool      `json:"-"`
}

func (seg Segment) ToShape() Shape {
	return Shape{
		LowerLeft:  seg.LowerLeft,
		UpperRight: seg.UpperRight,
		NetID:      seg.NetID,
		Layer:      seg.Layer,
		Purpose:    Drawing,
		NoViaUp:    seg.NoViaUp,
		NoViaDown:  seg.NoViaDown,
	}
}

func (s Segment) GetArea() int {
	return int(s.UpperRight.X-s.LowerLeft.X) * int(s.UpperRight.Y-s.LowerLeft.Y)
}

func (s Segment) Overlap(other Segment) bool {
	return s.LowerLeft.X < other.UpperRight.X && s.UpperRight.X > other.LowerLeft.X &&
		s.LowerLeft.Y < other.UpperRight.Y && s.UpperRight.Y > other.LowerLeft.Y
}

type TrackSegment struct {
	TrackID      int       `json:"track_id"`
	Start        Nm        `json:"start"`
	End          Nm        `json:"end"`
	NetID        int       `json:"net_id"`
	Layer        Layer     `json:"-"`
	CanvasOrigin Point     `json:"-"`
	Width        Nm        `json:"-"`
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
func (ts TrackSegment) GetLower() Nm {
	switch ts.Dir {
	case Horizontal:
		return ts.CanvasOrigin.Y + Nm(ts.TrackID)*ts.Width
	case Vertical:
		return ts.CanvasOrigin.X + Nm(ts.TrackID)*ts.Width
	default:
		panic(ErrUnknownDirection)
	}
}

func (ts TrackSegment) GetUpper() Nm { return ts.GetLower() + ts.Width }

func (ts TrackSegment) GetArea() int { return int(ts.End-ts.Start) * int(ts.Width) }

// RoutingPin is a physical pin access point from the router's perspective.
// XLow/YLow is the bottom-left corner of the pin bbox; XHigh/YHigh is the top-right.
// The session extends M2 to cover the full Y range and computes M1-M2 vias from the bbox.
// Name is non-empty only for top-level schematic pins (ports of the cell being designed).
// MinOverlap, when true, instructs the router to enter the pin bbox by only
// m2DRC.MinPinOverlap() nm instead of covering the full pin height.
type RoutingPin struct {
	Name       string
	Layer      Layer
	XLow       Nm
	XHigh      Nm
	YLow       Nm
	YHigh      Nm
	MinOverlap bool
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
	SatisfiesMinArea(seg Segment) bool
	ApplyEndExtension(lo, hi Nm) (Nm, Nm)
	ViaEnclosure() Nm
	// ViaTrackSpacing returns the minimum number of tracks that must separate
	// two M2 tracks that each carry a via to M3, to satisfy via spacing DRC.
	// Default is 1 (one empty track between any two via-bearing M2 tracks).
	ViaTrackSpacing() int
	// ApplyMinSpaceExtension extends [lo, hi] by the min_space rule in both
	// directions, returning the spacing-check range. No-op when min_space=0.
	ApplyMinSpaceExtension(lo, hi Nm) (Nm, Nm)
	// MinPinOverlap returns the minimum nm M2 must extend into the pin bbox
	// when routing in min-overlap mode. Zero means full-overlap (default).
	MinPinOverlap() Nm
}

// NoDRC is a DRCSpec with no constraints, used when DRC rules are not configured.
type NoDRC struct{}

func (NoDRC) SatisfiesMinArea(_ Segment) bool           { return true }
func (NoDRC) ApplyEndExtension(lo, hi Nm) (Nm, Nm)      { return lo, hi }
func (NoDRC) ViaEnclosure() Nm                          { return 0 }
func (NoDRC) ViaTrackSpacing() int                      { return 1 }
func (NoDRC) ApplyMinSpaceExtension(lo, hi Nm) (Nm, Nm) { return lo, hi }
func (NoDRC) MinPinOverlap() Nm                         { return 0 }

// ToTrack converts a Segment to a TrackSegment using seg.Dir and seg.CanvasOrigin.
// Horizontal: TrackID from Y, Start/End are X coordinates.
// Vertical:   TrackID from X, Start/End are Y coordinates.
// Returns ErrTrackMisaligned if the segment is not on the track grid.
func (seg Segment) ToTrack(tw Nm) (TrackSegment, error) {
	var offset, start, end, segWidth Nm
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
		TrackID:      int(offset / tw),
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
		yLow := ts.CanvasOrigin.Y + Nm(ts.TrackID)*ts.Width
		ll = Point{X: ts.Start, Y: yLow}
		ur = Point{X: ts.End, Y: yLow + ts.Width}
	case Vertical:
		xLow := ts.CanvasOrigin.X + Nm(ts.TrackID)*ts.Width
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
