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

type TwoLayerPath struct {
	M2Start Segment
	M2End   Segment
	M3      Segment
}

type Net struct {
	ID   int
	From Point
	To   Point
}

type PinDB interface {
	Query(lib, cell, pin string) (x, y int, err error)
}
