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
	M2 Layer = iota + 1
	M3
	Via12 // M1-M2 contact
	Via23 // M2-M3 via
)

func (l Layer) MarshalJSON() ([]byte, error) {
	switch l {
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

type Segment struct {
	LowerLeft  Point `json:"lower_left"`
	UpperRight Point `json:"upper_right"`
	NetID      int   `json:"net_id"`
	Layer      Layer `json:"layer"`
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
type RoutingPin struct {
	XLow  int
	XHigh int
	YLow  int
	YHigh int
}

type Net struct {
	ID   int
	From RoutingPin
	To   RoutingPin
}

type DRCSpec interface {
	MinArea() int
}

// NoDRC is a DRCSpec with no constraints, used when DRC rules are not configured.
type NoDRC struct{}

func (NoDRC) MinArea() int { return 0 }
