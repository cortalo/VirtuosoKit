package common_test

import (
	"autorouter/canvas"
	"autorouter/common"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCanvas returns a canvas with LowerLeft={0,0} and M3 trackWidth=100 (10 tracks).
func testCanvas() *canvas.Canvas {
	return &canvas.Canvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 1000, Y: 1000},
		M3Storage:  canvas.NewTrackSegmentStorage(10, 100),
		M2Storage:  canvas.NewSegmentStore(common.Point{X: 0, Y: 0}, common.Point{X: 1000, Y: 1000}),
	}
}

// --- ToTrack ---

func TestToTrack_HorizontalM3_CorrectTrackID(t *testing.T) {
	c := testCanvas()
	// Track 3 occupies Y=[300,400); segment must exactly cover that band.
	seg, err := c.NewSeg(common.M3, common.Point{X: 50, Y: 300}, common.Point{X: 500, Y: 400}, 1)
	require.NoError(t, err)

	ts, err := seg.ToTrack(100)

	require.NoError(t, err)
	assert.Equal(t, 3, ts.TrackID)
	assert.Equal(t, 50, ts.Start)
	assert.Equal(t, 500, ts.End)
	assert.Equal(t, 1, ts.NetID)
	assert.Equal(t, common.Horizontal, ts.Dir)
	assert.Equal(t, common.Point{X: 0, Y: 0}, ts.CanvasOrigin)
	assert.Equal(t, 100, ts.Width)
}

func TestToTrack_VerticalM2_CorrectTrackID(t *testing.T) {
	// Construct a vertical segment manually (base Canvas has no vertical track storage).
	seg := common.Segment{
		LowerLeft:    common.Point{X: 200, Y: 50},
		UpperRight:   common.Point{X: 300, Y: 600},
		NetID:        2,
		Layer:        common.M2,
		CanvasOrigin: common.Point{X: 0, Y: 0},
		Dir:          common.Vertical,
	}

	ts, err := seg.ToTrack(100)

	require.NoError(t, err)
	assert.Equal(t, 2, ts.TrackID) // X offset=200, 200/100=2
	assert.Equal(t, 50, ts.Start)
	assert.Equal(t, 600, ts.End)
	assert.Equal(t, common.Vertical, ts.Dir)
}

func TestToTrack_Misaligned_ReturnsMisalignedError(t *testing.T) {
	c := testCanvas()
	// Y offset=250 → 250%100=50, not aligned to grid.
	seg, err := c.NewSeg(common.M3, common.Point{X: 50, Y: 250}, common.Point{X: 500, Y: 350}, 1)
	require.NoError(t, err)

	_, err = seg.ToTrack(100)

	assert.ErrorIs(t, err, common.ErrTrackMisaligned)
}

func TestToTrack_WidthMismatch_ReturnsWidthMismatchError(t *testing.T) {
	c := testCanvas()
	// Y range [300,380) → width=80, tw=100 → mismatch (alignment itself is fine).
	seg, err := c.NewSeg(common.M3, common.Point{X: 50, Y: 300}, common.Point{X: 500, Y: 380}, 1)
	require.NoError(t, err)

	_, err = seg.ToTrack(100)

	assert.ErrorIs(t, err, common.ErrTrackWidthMismatch)
}

func TestToTrack_NegativeOffset_ReturnsMisalignedError(t *testing.T) {
	seg := common.Segment{
		LowerLeft:    common.Point{X: 0, Y: -100},
		UpperRight:   common.Point{X: 500, Y: 0},
		Layer:        common.M3,
		CanvasOrigin: common.Point{X: 0, Y: 0},
		Dir:          common.Horizontal,
	}

	_, err := seg.ToTrack(100)

	assert.ErrorIs(t, err, common.ErrTrackMisaligned)
}

// --- ToSeg ---

func TestToSeg_HorizontalM3_CorrectCoordinates(t *testing.T) {
	c := testCanvas()
	ts, err := c.NewTrack(common.M3, 5, 100, 600, 2)
	require.NoError(t, err)

	seg := ts.ToSeg()

	// Track 5: yLow=5*100=500, yHigh=6*100=600.
	assert.Equal(t, common.Point{X: 100, Y: 500}, seg.LowerLeft)
	assert.Equal(t, common.Point{X: 600, Y: 600}, seg.UpperRight)
	assert.Equal(t, common.M3, seg.Layer)
	assert.Equal(t, 2, seg.NetID)
	assert.Equal(t, common.Horizontal, seg.Dir)
	assert.Equal(t, common.Point{X: 0, Y: 0}, seg.CanvasOrigin)
}

func TestToSeg_VerticalM2_CorrectCoordinates(t *testing.T) {
	ts := common.TrackSegment{
		TrackID:      3,
		Start:        50,
		End:          700,
		NetID:        1,
		Layer:        common.M2,
		CanvasOrigin: common.Point{X: 0, Y: 0},
		Width:        100,
		Dir:          common.Vertical,
	}

	seg := ts.ToSeg()

	// Track 3 vertical: xLow=3*100=300, xHigh=4*100=400.
	assert.Equal(t, common.Point{X: 300, Y: 50}, seg.LowerLeft)
	assert.Equal(t, common.Point{X: 400, Y: 700}, seg.UpperRight)
	assert.Equal(t, common.M2, seg.Layer)
}

func TestToSeg_NonZeroOrigin_CorrectCoordinates(t *testing.T) {
	ts := common.TrackSegment{
		TrackID:      2,
		Start:        0,
		End:          400,
		NetID:        1,
		Layer:        common.M3,
		CanvasOrigin: common.Point{X: 200, Y: 300},
		Width:        100,
		Dir:          common.Horizontal,
	}

	seg := ts.ToSeg()

	// yLow = 300 + 2*100 = 500, yHigh = 300 + 3*100 = 600.
	assert.Equal(t, common.Point{X: 0, Y: 500}, seg.LowerLeft)
	assert.Equal(t, common.Point{X: 400, Y: 600}, seg.UpperRight)
}

// --- Round-trip ---

func TestRoundTrip_HorizontalM3_TrackToSegToTrack(t *testing.T) {
	c := testCanvas()
	ts1, err := c.NewTrack(common.M3, 7, 200, 800, 3)
	require.NoError(t, err)

	seg := ts1.ToSeg()
	ts2, err := seg.ToTrack(100)

	require.NoError(t, err)
	assert.Equal(t, ts1.TrackID, ts2.TrackID)
	assert.Equal(t, ts1.Start, ts2.Start)
	assert.Equal(t, ts1.End, ts2.End)
	assert.Equal(t, ts1.NetID, ts2.NetID)
}

func TestRoundTrip_HorizontalM3_SegToTrackToSeg(t *testing.T) {
	c := testCanvas()
	// Track 2: Y=[200,300).
	seg1, err := c.NewSeg(common.M3, common.Point{X: 100, Y: 200}, common.Point{X: 900, Y: 300}, 4)
	require.NoError(t, err)

	ts, err := seg1.ToTrack(100)
	require.NoError(t, err)

	seg2 := ts.ToSeg()
	assert.Equal(t, seg1.LowerLeft, seg2.LowerLeft)
	assert.Equal(t, seg1.UpperRight, seg2.UpperRight)
	assert.Equal(t, seg1.Layer, seg2.Layer)
	assert.Equal(t, seg1.NetID, seg2.NetID)
}
