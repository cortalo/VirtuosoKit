package canvas

import "autorouter/common"

type TrackSegmentStorage interface {
	IsPassible(seg TrackSegment) bool
	IsOccupied(seg TrackSegment) bool
	Occupy(seg TrackSegment) error
	GetTrackWidth() common.Nm
}

type TrackSegmentStorageImpl struct {
	TrackWidth common.Nm
	Tracks     []Track
}

func NewTrackSegmentStorage(trackCount int, trackWidth common.Nm) *TrackSegmentStorageImpl {
	tracks := make([]Track, trackCount)
	for i := range tracks {
		tracks[i] = NewTrackImpl()
	}
	return &TrackSegmentStorageImpl{
		TrackWidth: trackWidth,
		Tracks:     tracks,
	}
}

func (tss *TrackSegmentStorageImpl) IsPassible(seg TrackSegment) bool {
	if seg.TrackID < 0 || seg.TrackID >= len(tss.Tracks) {
		return false
	}
	return tss.Tracks[seg.TrackID].IsPassible(seg.NetID, seg.Start, seg.End)
}

func (tss *TrackSegmentStorageImpl) IsOccupied(seg TrackSegment) bool {
	if seg.TrackID < 0 || seg.TrackID >= len(tss.Tracks) {
		return false
	}
	return tss.Tracks[seg.TrackID].IsOccupied(seg.Start, seg.End)
}

func (tss *TrackSegmentStorageImpl) Occupy(seg TrackSegment) error {
	if seg.TrackID < 0 || seg.TrackID >= len(tss.Tracks) {
		return ErrInvalidTrackID
	}
	return tss.Tracks[seg.TrackID].Occupy(seg.NetID, seg.Start, seg.End)
}

func (tss *TrackSegmentStorageImpl) GetTrackWidth() common.Nm {
	return tss.TrackWidth
}
