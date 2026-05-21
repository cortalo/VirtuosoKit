package canvas

import (
	"autorouter/common"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCanvas() *Canvas {
	return &Canvas{
		LowerLeft:  Point{0, 0},
		UpperRight: Point{1000, 1000},
		M2Storage:  NewSegmentStore(Point{0, 0}, Point{1000, 1000}),
		M3Storage:  NewTrackSegmentStorage(10, 100),
	}
}

func mkSeg(x1, y1, x2, y2, netID int) Segment {
	return Segment{LowerLeft: Point{x1, y1}, UpperRight: Point{x2, y2}, NetID: netID, Layer: common.M2}
}

// mkM3Seg builds a factory-compatible M3 Segment (Dir and CanvasOrigin set)
// for the test canvas (LowerLeft={0,0}, trackWidth=100).
func mkM3Seg(trackID, start, end, netID int) Segment {
	return Segment{
		LowerLeft:    Point{start, trackID * 100},
		UpperRight:   Point{end, (trackID + 1) * 100},
		Layer:        common.M3,
		NetID:        netID,
		CanvasOrigin: Point{0, 0},
		Dir:          common.Horizontal,
	}
}

// --- Inbound ---

func TestCanvas_Inbound_InsideBounds(t *testing.T) {
	c := newCanvas()
	assert.True(t, c.Inbound(Point{0, 0}))
	assert.True(t, c.Inbound(Point{500, 500}))
	assert.True(t, c.Inbound(Point{1000, 1000}))
}

func TestCanvas_Inbound_OutsideBounds(t *testing.T) {
	c := newCanvas()
	assert.False(t, c.Inbound(Point{-1, 0}))
	assert.False(t, c.Inbound(Point{0, -1}))
	assert.False(t, c.Inbound(Point{1001, 0}))
	assert.False(t, c.Inbound(Point{0, 1001}))
}

// --- IsPassible M2 ---

func TestCanvas_IsPassibleM2_EmptyCanvas_Passable(t *testing.T) {
	c := newCanvas()
	assert.True(t, c.IsPassible(mkSeg(10, 10, 20, 100, 1)))
}

func TestCanvas_IsPassibleM2_AfterOccupy_NotPassable(t *testing.T) {
	c := newCanvas()
	require.NoError(t, c.Occupy(mkSeg(10, 10, 20, 100, 1)))
	assert.False(t, c.IsPassible(mkSeg(15, 10, 25, 100, 2)))
}

func TestCanvas_IsPassibleM2_SameNet_Passable(t *testing.T) {
	c := newCanvas()
	require.NoError(t, c.Occupy(mkSeg(10, 10, 20, 100, 1)))
	assert.True(t, c.IsPassible(mkSeg(10, 10, 20, 100, 1)))
}

func TestCanvas_IsPassibleM2_NonOverlapping_Passable(t *testing.T) {
	c := newCanvas()
	require.NoError(t, c.Occupy(mkSeg(10, 10, 20, 50, 1)))
	assert.True(t, c.IsPassible(mkSeg(20, 10, 30, 50, 2)))  // adjacent
	assert.True(t, c.IsPassible(mkSeg(10, 50, 20, 100, 2))) // above
}

// --- IsPassible M3 ---

func TestCanvas_IsPassibleM3_EmptyCanvas_Passable(t *testing.T) {
	c := newCanvas()
	assert.True(t, c.IsPassible(mkM3Seg(0, 0, 500, 1)))
}

func TestCanvas_IsPassibleM3_AfterOccupy_NotPassable(t *testing.T) {
	c := newCanvas()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.False(t, c.IsPassible(mkM3Seg(0, 200, 600, 2)))
}

func TestCanvas_IsPassibleM3_SameNet_Passable(t *testing.T) {
	c := newCanvas()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.True(t, c.IsPassible(mkM3Seg(0, 100, 500, 1)))
}

func TestCanvas_IsPassibleM3_DifferentTrack_Passable(t *testing.T) {
	c := newCanvas()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.True(t, c.IsPassible(mkM3Seg(1, 100, 500, 2)))
}

// --- Occupy M2 ---

func TestCanvas_OccupyM2_Basic_Succeeds(t *testing.T) {
	c := newCanvas()
	assert.NoError(t, c.Occupy(mkSeg(10, 10, 20, 100, 1)))
}

func TestCanvas_OccupyM2_Overlap_DifferentNet_ReturnsError(t *testing.T) {
	c := newCanvas()
	require.NoError(t, c.Occupy(mkSeg(10, 10, 20, 100, 1)))
	assert.ErrorIs(t, c.Occupy(mkSeg(15, 10, 25, 100, 2)), ErrOverlap)
}

// --- Occupy M3 ---

func TestCanvas_OccupyM3_Basic_Succeeds(t *testing.T) {
	c := newCanvas()
	assert.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
}

func TestCanvas_OccupyM3_OutOfRangeTrack_ReturnsInvalidTrackIDError(t *testing.T) {
	c := newCanvas()
	// mkM3Seg(10,...) → TrackID=10, storage only has tracks 0-9 → ErrInvalidTrackID from storage
	assert.ErrorIs(t, c.Occupy(mkM3Seg(10, 0, 100, 1)), ErrInvalidTrackID)
}

func TestCanvas_OccupyM3_Overlap_DifferentNet_ReturnsError(t *testing.T) {
	c := newCanvas()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.ErrorIs(t, c.Occupy(mkM3Seg(0, 200, 600, 2)), ErrOverlap)
}

// --- GetM3TrackWidth ---

func TestCanvas_GetM3TrackWidth_ReturnsCorrectWidth(t *testing.T) {
	c := newCanvas()
	assert.Equal(t, 100, c.GetM3TrackWidth())
}
