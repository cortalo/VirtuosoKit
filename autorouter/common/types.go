package common

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Segment struct {
	LowerLeft  Point `json:"lower_left"`
	UpperRight Point `json:"upper_right"`
	NetID      int   `json:"net_id"`
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
// XLow/YLow is the bottom-left corner of the pin bbox (M2 anchor).
// YHigh is the top of the pin bbox, used by the session to extend M2 coverage.
type RoutingPin struct {
	XLow  int
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
