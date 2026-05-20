package common

import (
	"encoding/json"
	"fmt"
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

type Segment struct {
	LowerLeft  Point   `json:"lower_left"`
	UpperRight Point   `json:"upper_right"`
	NetID      int     `json:"net_id"`
	Layer      Layer   `json:"layer"`
	Purpose    Purpose `json:"purpose"`
	Name       string  `json:"name,omitempty"`
}

func (s Segment) Overlap(other Segment) bool {
	return s.LowerLeft.X < other.UpperRight.X && s.UpperRight.X > other.LowerLeft.X &&
		s.LowerLeft.Y < other.UpperRight.Y && s.UpperRight.Y > other.LowerLeft.Y
}

type TrackSegment struct {
	TrackID int `json:"track_id"`
	Start   int `json:"start"`
	End     int `json:"end"`
	NetID   int `json:"net_id"`
}

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
}

// NoDRC is a DRCSpec with no constraints, used when DRC rules are not configured.
type NoDRC struct{}

func (NoDRC) MinArea() int      { return 0 }
func (NoDRC) EndExtension() int { return 0 }
