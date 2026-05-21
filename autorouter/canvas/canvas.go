package canvas

import (
	"autorouter/common"
	"errors"
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
	GetTrackWidth() int
}

type SegmentStorage interface {
	IsPassible(seg Segment) bool
	Occupy(seg Segment) error
}
