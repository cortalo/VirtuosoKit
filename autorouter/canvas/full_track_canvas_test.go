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

// --- IsOccupied M2 ---

func TestFTC_IsOccupiedM2_EmptyCanvas_NotOccupied(t *testing.T) {
	c := newFTC()
	assert.False(t, c.IsOccupied(mkFTM2Seg(0, 0, 500, 1)))
}

func TestFTC_IsOccupiedM2_AfterOccupy_SameNet_IsOccupied(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkFTM2Seg(0, 100, 500, 1)))
	assert.True(t, c.IsOccupied(mkFTM2Seg(0, 100, 500, 1)))
}

func TestFTC_IsOccupiedM2_AfterOccupy_DifferentNet_IsOccupied(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkFTM2Seg(0, 100, 500, 1)))
	// different net: IsPassible returns false but IsOccupied also returns true
	assert.True(t, c.IsOccupied(mkFTM2Seg(0, 200, 600, 2)))
}

func TestFTC_IsOccupiedM2_DifferentTrack_NotOccupied(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkFTM2Seg(0, 100, 500, 1)))
	assert.False(t, c.IsOccupied(mkFTM2Seg(1, 100, 500, 2)))
}

// --- IsOccupied M3 ---

func TestFTC_IsOccupiedM3_EmptyCanvas_NotOccupied(t *testing.T) {
	c := newFTC()
	assert.False(t, c.IsOccupied(mkM3Seg(0, 0, 500, 1)))
}

func TestFTC_IsOccupiedM3_AfterOccupy_SameNet_IsOccupied(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.True(t, c.IsOccupied(mkM3Seg(0, 100, 500, 1)))
}

func TestFTC_IsOccupiedM3_AfterOccupy_DifferentNet_IsOccupied(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.True(t, c.IsOccupied(mkM3Seg(0, 200, 600, 2)))
}

func TestFTC_IsOccupiedM3_DifferentTrack_NotOccupied(t *testing.T) {
	c := newFTC()
	require.NoError(t, c.Occupy(mkM3Seg(0, 100, 500, 1)))
	assert.False(t, c.IsOccupied(mkM3Seg(1, 100, 500, 2)))
}

// --- ParseOrient ---

func TestParseOrient_AllValidOrients(t *testing.T) {
	cases := []struct {
		s    string
		want Orient
	}{
		{"R0", R0},
		{"R90", R90},
		{"R180", R180},
		{"R270", R270},
		{"MX", MX},
		{"MY", MY},
		{"MXR90", MXR90},
		{"MYR90", MYR90},
	}
	for _, tc := range cases {
		got, err := ParseOrient(tc.s)
		require.NoError(t, err, tc.s)
		assert.Equal(t, tc.want, got, tc.s)
	}
}

func TestParseOrient_Unknown_ReturnsError(t *testing.T) {
	_, err := ParseOrient("R45")
	assert.Error(t, err)
}

// --- AbsoluteMetals ---

// baseShape is a cell-relative M2 rectangle used across orient tests.
// xLow=10 xHigh=30 yLow=20 yHigh=40, instance origin at (100,200).
func baseInstance(o Orient) Instance {
	return Instance{
		XY:     Point{100, 200},
		Orient: o,
		Metals: []common.Shape{{
			LowerLeft:  Point{10, 20},
			UpperRight: Point{30, 40},
			Layer:      common.M2,
		}},
	}
}

func TestAbsoluteMetals_R0(t *testing.T) {
	s := baseInstance(R0).AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, Point{110, 220}, s[0].LowerLeft)
	assert.Equal(t, Point{130, 240}, s[0].UpperRight)
}

func TestAbsoluteMetals_R90(t *testing.T) {
	// (x,y)→(-y,x): xLow=-40 xHigh=-20 yLow=10 yHigh=30 + origin
	s := baseInstance(R90).AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, Point{60, 210}, s[0].LowerLeft)
	assert.Equal(t, Point{80, 230}, s[0].UpperRight)
}

func TestAbsoluteMetals_R180(t *testing.T) {
	// (x,y)→(-x,-y): xLow=-30 xHigh=-10 yLow=-40 yHigh=-20 + origin
	s := baseInstance(R180).AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, Point{70, 160}, s[0].LowerLeft)
	assert.Equal(t, Point{90, 180}, s[0].UpperRight)
}

func TestAbsoluteMetals_R270(t *testing.T) {
	// (x,y)→(y,-x): xLow=20 xHigh=40 yLow=-30 yHigh=-10 + origin
	s := baseInstance(R270).AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, Point{120, 170}, s[0].LowerLeft)
	assert.Equal(t, Point{140, 190}, s[0].UpperRight)
}

func TestAbsoluteMetals_MX(t *testing.T) {
	// (x,y)→(x,-y): xLow=10 xHigh=30 yLow=-40 yHigh=-20 + origin
	s := baseInstance(MX).AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, Point{110, 160}, s[0].LowerLeft)
	assert.Equal(t, Point{130, 180}, s[0].UpperRight)
}

func TestAbsoluteMetals_MY(t *testing.T) {
	// (x,y)→(-x,y): xLow=-30 xHigh=-10 yLow=20 yHigh=40 + origin
	s := baseInstance(MY).AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, Point{70, 220}, s[0].LowerLeft)
	assert.Equal(t, Point{90, 240}, s[0].UpperRight)
}

func TestAbsoluteMetals_MXR90(t *testing.T) {
	// (x,y)→(y,x): xLow=20 xHigh=40 yLow=10 yHigh=30 + origin
	s := baseInstance(MXR90).AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, Point{120, 210}, s[0].LowerLeft)
	assert.Equal(t, Point{140, 230}, s[0].UpperRight)
}

func TestAbsoluteMetals_MYR90(t *testing.T) {
	// (x,y)→(-y,-x): xLow=-40 xHigh=-20 yLow=-30 yHigh=-10 + origin
	s := baseInstance(MYR90).AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, Point{60, 170}, s[0].LowerLeft)
	assert.Equal(t, Point{80, 190}, s[0].UpperRight)
}

func TestAbsoluteMetals_LayerPreserved(t *testing.T) {
	inst := Instance{
		XY:     Point{0, 0},
		Orient: R0,
		Metals: []common.Shape{{LowerLeft: Point{0, 0}, UpperRight: Point{10, 10}, Layer: common.M3}},
	}
	s := inst.AbsoluteMetals()
	require.Len(t, s, 1)
	assert.Equal(t, common.M3, s[0].Layer)
}

func TestAbsoluteMetals_Empty(t *testing.T) {
	inst := Instance{XY: Point{0, 0}, Orient: R0}
	assert.Empty(t, inst.AbsoluteMetals())
}

// --- NewFullTrackCanvas ---

func newFTCInstances(instances []Instance) (*FullTrackCanvas, error) {
	return NewFullTrackCanvas(
		Point{0, 0}, Point{1000, 1000},
		NewTrackSegmentStorage(100, 10),
		NewTrackSegmentStorage(10, 100),
		common.Vertical,
		instances,
	)
}

func TestNewFullTrackCanvas_NoInstances_Empty(t *testing.T) {
	c, err := newFTCInstances(nil)
	require.NoError(t, err)
	assert.True(t, c.IsPassible(mkFTM2Seg(5, 0, 1000, 1)))
}

func TestNewFullTrackCanvas_M2Instance_TracksOccupied(t *testing.T) {
	// M2 is vertical width=10; shape at x=[50,60] lands on track 5.
	inst := Instance{
		XY:     Point{0, 0},
		Orient: R0,
		Metals: []common.Shape{{
			LowerLeft:  Point{50, 0},
			UpperRight: Point{60, 1000},
			Layer:      common.M2,
		}},
	}
	c, err := newFTCInstances([]Instance{inst})
	require.NoError(t, err)
	assert.False(t, c.IsPassible(mkFTM2Seg(5, 0, 500, 1)))
	assert.True(t, c.IsPassible(mkFTM2Seg(4, 0, 500, 1)))
}

func TestNewFullTrackCanvas_M3Instance_TrackOccupied(t *testing.T) {
	// M3 is horizontal width=100; shape at y=[300,400] lands on track 3.
	inst := Instance{
		XY:     Point{0, 0},
		Orient: R0,
		Metals: []common.Shape{{
			LowerLeft:  Point{0, 300},
			UpperRight: Point{1000, 400},
			Layer:      common.M3,
		}},
	}
	c, err := newFTCInstances([]Instance{inst})
	require.NoError(t, err)
	assert.False(t, c.IsPassible(mkM3Seg(3, 0, 500, 1)))
	assert.True(t, c.IsPassible(mkM3Seg(2, 0, 500, 1)))
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
