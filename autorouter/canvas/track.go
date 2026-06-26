package canvas

import (
	"autorouter/common"
	"errors"

	"github.com/google/btree"
)

var ErrOverlap = errors.New("segment overlaps existing occupation")

type Track interface {
	IsPassible(netID int, start, end common.Nm) bool
	IsOccupied(start, end common.Nm) bool
	Occupy(netID int, start, end common.Nm) error
}

type interval struct {
	start, end common.Nm
	netID      int
}
type TrackImpl struct {
	occupied *btree.BTreeG[interval]
}

func NewTrackImpl() *TrackImpl {
	return &TrackImpl{
		occupied: btree.NewG[interval](32, func(a, b interval) bool {
			return a.start < b.start
		}),
	}
}

func (t *TrackImpl) IsPassible(netID int, start, end common.Nm) bool {
	passable := true
	// start from the first interval whose start >= start
	t.occupied.AscendGreaterOrEqual(interval{start: start}, func(iv interval) bool {
		if iv.start >= end {
			return false
		}
		if iv.netID != netID {
			passable = false
			return false
		}
		return true
	})
	// also check the interval just before start, it might extend into [start,end)
	t.occupied.DescendLessOrEqual(interval{start: start - 1}, func(iv interval) bool {
		if iv.end > start && iv.netID != netID {
			passable = false
		}
		return false
	})

	return passable

}

func (t *TrackImpl) IsOccupied(start, end common.Nm) bool {
	occupied := false
	t.occupied.AscendGreaterOrEqual(interval{start: start}, func(iv interval) bool {
		if iv.start >= end {
			return false
		}
		occupied = true
		return false
	})
	if !occupied {
		t.occupied.DescendLessOrEqual(interval{start: start - 1}, func(iv interval) bool {
			if iv.end > start {
				occupied = true
			}
			return false
		})
	}
	return occupied
}

func (t *TrackImpl) Occupy(netID int, start, end common.Nm) error {
	if !t.IsPassible(netID, start, end) {
		return ErrOverlap
	}
	mergedStart, mergedEnd := start, end
	var toDelete []interval
	t.occupied.AscendGreaterOrEqual(interval{start: start}, func(iv interval) bool {
		if iv.netID == netID && iv.start <= end {
			mergedStart = min(iv.start, mergedStart)
			mergedEnd = max(iv.end, mergedEnd)
			toDelete = append(toDelete, iv)
			return true
		}
		return false
	})
	t.occupied.DescendLessOrEqual(interval{start: start - 1}, func(iv interval) bool {
		if iv.netID == netID && iv.end >= start {
			mergedStart = min(iv.start, mergedStart)
			mergedEnd = max(iv.end, mergedEnd)
			toDelete = append(toDelete, iv)
			return true
		}
		return false
	})
	for _, iv := range toDelete {
		t.occupied.Delete(iv)
	}
	t.occupied.ReplaceOrInsert(interval{start: mergedStart, end: mergedEnd, netID: netID})
	return nil
}
