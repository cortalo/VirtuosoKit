package canvas

import (
	"autorouter/common"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFTC creates a FullTrackCanvas: 1000x1000, M2=10nm wide (100 vertical tracks),
// M3=100nm wide (10 horizontal tracks).
func newFTC() *FullTrackCanvas {
	return &FullTrackCanvas{
		LowerLeft:  Point{0, 0},
		UpperRight: Point{1000, 1000},
		M2Storage:  NewTrackSegmentStorage(100, 10),
		M3Storage:  NewTrackSegmentStorage(10, 100),
		M2Dir:      common.Vertical,
	}
}

// mkFTM2Seg builds a factory-compatible M2 Segment for the test FullTrackCanvas.
// M2 is vertical: TrackID from X (width=10), Start/End are Y coordinates.
func mkFTM2Seg(trackID, start, end, netID int) Segment {
	return Segment{
		LowerLeft:    Point{trackID * 10, start},
		UpperRight:   Point{(trackID + 1) * 10, end},
		Layer:        common.M2,
		NetID:        netID,
		CanvasOrigin: Point{0, 0},
		Dir:          common.Vertical,
	}
}

// --- Inbound ---

func TestFTC_Inbound_InsideBounds(t *testing.T) {
	c := newFTC()
	assert.True(t, c.Inbound(Point{0, 0}))
	assert.True(t, c.Inbound(Point{500, 500}))
	assert.True(t, c.Inbound(Point{1000, 1000}))
}

func TestFTC_Inbound_OutsideBounds(t *testing.T) {
	c := newFTC()
	assert.False(t, c.Inbound(Point{-1, 0}))
	assert.False(t, c.Inbound(Point{0, -1}))
	assert.False(t, c.Inbound(Point{1001, 0}))
	assert.False(t, c.Inbound(Point{0, 1001}))
}

// --- IsPassible M2 ---

func TestFTC_IsPassibleM2_EmptyCanvas_Passable(t *testing.T) {
	c := newFTC()
	assert.True(t, c.IsPassible(mkFTM2Seg(0, 0, 500, 1)))
}

func TestFTC_IsPassibleM2_AfterOccupy_NotPassable(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkFTM2Seg(0, 100, 500, 1)))
	assert.False(t, c.IsPassible(mkFTM2Seg(0, 200, 600, 2)))
}

func TestFTC_IsPassibleM2_SameNet_Passable(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkFTM2Seg(0, 100, 500, 1)))
	assert.True(t, c.IsPassible(mkFTM2Seg(0, 100, 500, 1)))
}

func TestFTC_IsPassibleM2_DifferentTrack_Passable(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkFTM2Seg(0, 100, 500, 1)))
	assert.True(t, c.IsPassible(mkFTM2Seg(1, 100, 500, 2)))
}

// --- IsPassible M3 (reuses mkM3Seg from two_layer_canvas_test.go) ---

func TestFTC_IsPassibleM3_EmptyCanvas_Passable(t *testing.T) {
	c := newFTC()
	assert.True(t, c.IsPassible(mkM3Seg(0, 0, 500, 1)))
}

func TestFTC_IsPassibleM3_AfterOccupy_NotPassable(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.False(t, c.IsPassible(mkM3Seg(0, 200, 600, 2)))
}

func TestFTC_IsPassibleM3_SameNet_Passable(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.True(t, c.IsPassible(mkM3Seg(0, 100, 500, 1)))
}

func TestFTC_IsPassibleM3_DifferentTrack_Passable(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.True(t, c.IsPassible(mkM3Seg(1, 100, 500, 2)))
}

// --- Occupy M2 ---

func TestFTC_OccupyM2_Basic_Succeeds(t *testing.T) {
	c := newFTC()
	assert.NoError(t, c.Occupy(mkFTM2Seg(0, 100, 500, 1)))
}

func TestFTC_OccupyM2_Overlap_DifferentNet_ReturnsError(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkFTM2Seg(0, 100, 500, 1)))
	assert.ErrorIs(t, c.Occupy(mkFTM2Seg(0, 200, 600, 2)), ErrOverlap)
}

func TestFTC_OccupyM2_OutOfRangeTrack_ReturnsError(t *testing.T) {
	c := newFTC()
	assert.ErrorIs(t, c.Occupy(mkFTM2Seg(100, 0, 100, 1)), ErrInvalidTrackID)
}

// --- Occupy M3 ---

func TestFTC_OccupyM3_Basic_Succeeds(t *testing.T) {
	c := newFTC()
	assert.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
}

func TestFTC_OccupyM3_Overlap_DifferentNet_ReturnsError(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.ErrorIs(t, c.Occupy(mkM3Seg(0, 200, 600, 2)), ErrOverlap)
}

func TestFTC_OccupyM3_OutOfRangeTrack_ReturnsError(t *testing.T) {
	c := newFTC()
	assert.ErrorIs(t, c.Occupy(mkM3Seg(10, 0, 100, 1)), ErrInvalidTrackID)
}

// --- GetTrackWidth ---

func TestFTC_GetTrackWidth_M2(t *testing.T) {
	c := newFTC()
	assert.Equal(t, 10, c.GetTrackWidth(common.M2))
}

func TestFTC_GetTrackWidth_M3(t *testing.T) {
	c := newFTC()
	assert.Equal(t, 100, c.GetTrackWidth(common.M3))
}

// --- NewTrack ---

func TestFTC_NewTrackM2_Basic(t *testing.T) {
	c := newFTC()
	ts, err := c.NewTrack(common.M2, 5, 200, 800, 1)
	require.NoError(t, err)
	assert.Equal(t, 5, ts.TrackID)
	assert.Equal(t, 10, ts.Width)
	assert.Equal(t, common.Vertical, ts.Dir)
	assert.Equal(t, 100, ts.NumTracks)
}

func TestFTC_NewTrackM2_OutOfRange_ReturnsError(t *testing.T) {
	c := newFTC()
	_, err := c.NewTrack(common.M2, 100, 0, 100, 1)
	assert.ErrorIs(t, err, ErrInvalidTrackID)
}

func TestFTC_NewTrackM3_Basic(t *testing.T) {
	c := newFTC()
	ts, err := c.NewTrack(common.M3, 3, 0, 500, 1)
	require.NoError(t, err)
	assert.Equal(t, 3, ts.TrackID)
	assert.Equal(t, 100, ts.Width)
	assert.Equal(t, common.Horizontal, ts.Dir)
	assert.Equal(t, 10, ts.NumTracks)
}

func TestFTC_NewTrackM3_OutOfRange_ReturnsError(t *testing.T) {
	c := newFTC()
	_, err := c.NewTrack(common.M3, 10, 0, 100, 1)
	assert.ErrorIs(t, err, ErrInvalidTrackID)
}
