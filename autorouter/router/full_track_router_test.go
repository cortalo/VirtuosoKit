package router_test

import (
	"autorouter/canvas"
	"autorouter/common"
	"autorouter/router"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFTCanvas creates a FullTrackCanvas: width x height, M2 vertical (m2tw wide),
// M3 horizontal (m3tw wide).
func newFTCanvas(width, height, m2tw, m3tw int) *canvas.FullTrackCanvas {
	return &canvas.FullTrackCanvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: width, Y: height},
		M2Storage:  canvas.NewTrackSegmentStorage(width/m2tw, m2tw),
		M3Storage:  canvas.NewTrackSegmentStorage(height/m3tw, m3tw),
		M2Dir:      common.Vertical,
	}
}

func newFTRouter(c *canvas.FullTrackCanvas) *router.FullTrackRouter {
	return router.NewFullTrackRouter(c, c.M2Dir, common.NoDRC{}, common.NoDRC{})
}

// ftPins builds RoutingPin slices with explicit X and Y ranges.
func ftPins(coords ...[4]int) []common.RoutingPin {
	ps := make([]common.RoutingPin, len(coords))
	for i, c := range coords {
		ps[i] = common.RoutingPin{XLow: c[0], XHigh: c[1], YLow: c[2], YHigh: c[3]}
	}
	return ps
}

// occupyFTM3Track marks a full-width M3 track as occupied on a FullTrackCanvas.
func occupyFTM3Track(c *canvas.FullTrackCanvas, trackID, start, end, netID, tw int) error {
	return c.Occupy(common.Segment{
		LowerLeft:    common.Point{X: start, Y: trackID * tw},
		UpperRight:   common.Point{X: end, Y: (trackID + 1) * tw},
		NetID:        netID,
		Layer:        common.M3,
		CanvasOrigin: common.Point{X: 0, Y: 0},
		Dir:          common.Horizontal,
	})
}

// occupyFTM2Track marks a single M2 track segment as occupied on a FullTrackCanvas.
func occupyFTM2Track(c *canvas.FullTrackCanvas, trackID, start, end, netID, tw int) error {
	return c.Occupy(common.Segment{
		LowerLeft:    common.Point{X: trackID * tw, Y: start},
		UpperRight:   common.Point{X: (trackID + 1) * tw, Y: end},
		NetID:        netID,
		Layer:        common.M2,
		CanvasOrigin: common.Point{X: 0, Y: 0},
		Dir:          common.Vertical,
	})
}

// --- basic routing ---

// Canvas 1000x1000, m2tw=10, m3tw=100.
// Pin1 at X=[0,10] Y=[100,200] → M2 track 0.
// Pin2 at X=[500,510] Y=[900,1000] → M2 track 50.
// midY=(100+900)/2=500 → midTrack=5 (Y=[500,600]).
func TestFTRoute_ClearCanvas_FindsMidTrack(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	r := newFTRouter(c)

	segs, err := r.Route(ftPins([4]int{0, 10, 100, 200}, [4]int{500, 510, 900, 1000}), 1)

	require.NoError(t, err)
	assert.Equal(t, 5, m3Track(segs, 100))
}

// Both pins at same Y → midTrack = YLow / m3tw.
func TestFTRoute_SameY_FindsMidTrack(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	r := newFTRouter(c)

	segs, err := r.Route(ftPins([4]int{0, 10, 200, 300}, [4]int{500, 510, 200, 300}), 1)

	require.NoError(t, err)
	assert.Equal(t, 2, m3Track(segs, 100))
}

func TestFTRoute_OutOfBounds_ReturnsError(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	r := newFTRouter(c)

	_, err := r.Route(ftPins([4]int{-1, 10, 0, 100}, [4]int{500, 510, 200, 300}), 1)
	assert.ErrorIs(t, err, router.ErrOutOfBound)
}

// Pin with XHigh == XLow → no M2 track overlaps → ErrPinMisaligned.
func TestFTRoute_ZeroWidthPin_ReturnsErrPinMisaligned(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	r := newFTRouter(c)

	_, err := r.Route(ftPins([4]int{10, 10, 100, 200}, [4]int{500, 510, 100, 200}), 1)
	assert.ErrorIs(t, err, router.ErrPinMisaligned)
}

// --- obstacle avoidance ---

// Block M3 track 5. Tracks 4 and 6 also fail (adjacent to blocked).
// delta=2: track 7 is first valid (prev=6 passible).
func TestFTRoute_MidM3TrackBlocked_FallsBack(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	require.NoError(t, occupyFTM3Track(c, 5, 0, 1000, 99, 100))
	r := newFTRouter(c)

	segs, err := r.Route(ftPins([4]int{0, 10, 100, 200}, [4]int{500, 510, 900, 1000}), 1)

	require.NoError(t, err)
	trackID := m3Track(segs, 100)
	assert.True(t, trackID == 3 || trackID == 7, "got track %d", trackID)
}

// Block M3 tracks 5 and 6. Adjacent spacing forces router past tracks 4 and 7.
// delta=2: track 7 fails (prev=6 blocked); track 3 passes (next=4 passible).
func TestFTRoute_TwoAdjacentM3TracksBlocked_SkipsBoth(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	require.NoError(t, occupyFTM3Track(c, 5, 0, 1000, 99, 100))
	require.NoError(t, occupyFTM3Track(c, 6, 0, 1000, 99, 100))
	r := newFTRouter(c)

	segs, err := r.Route(ftPins([4]int{0, 10, 100, 200}, [4]int{500, 510, 900, 1000}), 1)

	require.NoError(t, err)
	assert.Equal(t, 3, m3Track(segs, 100))
}

// Block M2 track 0 for the Y range where pin1 would connect.
// Pin1 spans X=[0,25]: track 0 blocked; track 1 fails adjacent check (PrevTrack=0 blocked);
// track 2's PrevTrack is track 1 (empty) → passes.
func TestFTRoute_PreferredM2TrackBlocked_UsesNextCandidate(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	// midY=(100+100)/2=100 → midTrack=1 → M3 Y=[100,200]
	// M2 extent for pin1: Y=[100,200]; block track 0 there.
	require.NoError(t, occupyFTM2Track(c, 0, 100, 200, 99, 10))
	r := newFTRouter(c)

	// Pin1: X=[0,25] → candidates [0,1,2] sorted by proximity → [1,0,2].
	// Track 0 blocked; track 1 rejected (PrevTrack=0 blocked); track 2 succeeds.
	segs, err := r.Route(ftPins([4]int{0, 25, 100, 200}, [4]int{500, 510, 100, 200}), 1)

	require.NoError(t, err)
	// M2 for pin1 should use track 2 (X=20).
	assert.Equal(t, 20, segs[0].LowerLeft.X, "pin1 M2 should use track 2 (X=20)")
}

// Two pins whose natural M2 track assignment would be adjacent.
// Pin1: X=[0,100]   → only track 0.
// Pin2: X=[100,280] → candidates [track1, track2]; track1 is closer but adjacent
// to track0 → must be rejected; track2 (X=[200,300]) should be chosen instead.
func TestFTRoute_IntraPinAdjacentM2_UsesNonAdjacentTrack(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := newFTRouter(c)

	segs, err := r.Route(ftPins(
		[4]int{0, 100, 100, 200},
		[4]int{100, 280, 100, 200},
	), 1)

	require.NoError(t, err)
	require.Len(t, segs, 3)
	// segs[0]=pin1 M2, segs[1]=pin2 M2, segs[2]=M3.
	assert.Equal(t, 0, segs[0].LowerLeft.X, "pin1 M2 should be track 0 (X=0)")
	assert.Equal(t, 200, segs[1].LowerLeft.X, "pin2 M2 should skip adjacent track 1 and use track 2 (X=200)")
}

// Both pins span X=[0,100] with m2tw=100 → both land on M2 track 0.
// No M3 is needed; the router should return a single merged M2 spanning both pins.
func TestFTRoute_SameM2Track_NoM3(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := newFTRouter(c)

	segs, err := r.Route(ftPins(
		[4]int{0, 100, 100, 200},
		[4]int{0, 100, 500, 600},
	), 1)

	require.NoError(t, err)
	assert.Equal(t, -1, m3Track(segs, 100), "no M3 when all pins on same M2 track")
	require.Len(t, segs, 1, "one merged M2, no M3")
	assert.Equal(t, common.M2, segs[0].Layer)
	assert.Equal(t, 100, segs[0].LowerLeft.Y, "M2 starts at lower pin YLow")
	assert.Equal(t, 600, segs[0].UpperRight.Y, "M2 ends at upper pin YHigh")
}

func TestFTRoute_AllM3TracksBlocked_ReturnsErrNoPath(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	for i := 0; i < 10; i++ {
		require.NoError(t, occupyFTM3Track(c, i, 0, 1000, 99, 100))
	}
	r := newFTRouter(c)

	_, err := r.Route(ftPins([4]int{0, 10, 100, 200}, [4]int{500, 510, 900, 1000}), 1)
	assert.ErrorIs(t, err, router.ErrNoPath)
}

func TestFTRoute_SameNetID_IgnoresOwnBlocks(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	require.NoError(t, occupyFTM3Track(c, 5, 0, 1000, 1, 100))
	r := newFTRouter(c)

	segs, err := r.Route(ftPins([4]int{0, 10, 100, 200}, [4]int{500, 510, 900, 1000}), 1)

	require.NoError(t, err)
	assert.Equal(t, 5, m3Track(segs, 100))
}

// --- M2=Horizontal regression ---

// With M2Dir=Horizontal, M2 tracks are indexed by Y and M3 tracks are vertical
// (indexed by X). FullTrackRouter hardcodes the assumption that M2 is vertical
// and M3 is horizontal, so it produces the wrong M3 track when M2Dir=Horizontal.
//
// Setup: 1000x1000, m2tw=m3tw=100, M2=Horizontal.
// Pin1: X=[200,300] Y=[0,100]   → correct M2 track: 0 (Y=[0,100])
// Pin2: X=[200,300] Y=[900,1000]→ correct M2 track: 9 (Y=[900,1000])
// Correct M3 track: midX=250 → track 2 (X=[200,300]); M3 Y=[0,1000].
//
// Buggy router uses midY=450 → M3 "track 4" in Y coords → M3 ends up at X=[400,500].
func TestFTRoute_HorizontalM2Dir_WrongM3Track(t *testing.T) {
	c := &canvas.FullTrackCanvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 1000, Y: 1000},
		M2Storage:  canvas.NewTrackSegmentStorage(10, 100),
		M3Storage:  canvas.NewTrackSegmentStorage(10, 100),
		M2Dir:      common.Horizontal,
	}
	r := router.NewFullTrackRouter(c, common.Horizontal, common.NoDRC{}, common.NoDRC{})

	segs, err := r.Route(ftPins(
		[4]int{200, 300, 0, 100},
		[4]int{200, 300, 900, 1000},
	), 1)

	require.NoError(t, err)

	var m3Seg common.Segment
	for _, s := range segs {
		if s.Layer == common.M3 {
			m3Seg = s
		}
	}

	// M3 is vertical: its X range identifies the track.
	// Expected: track 2 → X=[200,300]. Bug: router picks X=[400,500] (track 4 via midY).
	assert.Equal(t, 200, m3Seg.LowerLeft.X, "M3 should be at X=200 (track 2); bug produces X=400 (track 4 via midY)")
	assert.Equal(t, 300, m3Seg.UpperRight.X, "M3 UpperRight.X should be 300")

	// M3 should span the full Y range connecting the two M2 tracks.
	// Expected: Y=[0,1000]. Bug: M3 Y is wrong too (computed from X coords of wrong M2 tracks).
	assert.Equal(t, 0, m3Seg.LowerLeft.Y, "M3 should start at Y=0 (M2 track 0)")
	assert.Equal(t, 1000, m3Seg.UpperRight.Y, "M3 should end at Y=1000 (M2 track 9)")
}

// Route returns len(pins)+1 segments: one M2 per pin plus one M3.
func TestFTRoute_ResultShape_TwoPins(t *testing.T) {
	c := newFTCanvas(1000, 1000, 10, 100)
	r := newFTRouter(c)

	segs, err := r.Route(ftPins([4]int{0, 10, 100, 200}, [4]int{500, 510, 100, 200}), 1)

	require.NoError(t, err)
	require.Len(t, segs, 3)
	assert.Equal(t, common.M2, segs[0].Layer)
	assert.Equal(t, common.M2, segs[1].Layer)
	assert.Equal(t, common.M3, segs[2].Layer)
}
