package router_test

import (
	"autorouter/canvas"
	"autorouter/common"
	"autorouter/router"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCanvas(width, height, trackWidth int) *canvas.TwoLayerCanvas {
	trackCount := height / trackWidth
	return &canvas.TwoLayerCanvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: width, Y: height},
		M2Storage:  canvas.NewSegmentStore(common.Point{X: 0, Y: 0}, common.Point{X: width, Y: height}),
		M3Storage:  canvas.NewTrackSegmentStorage(trackCount, trackWidth),
	}
}

func newRouter(c *canvas.TwoLayerCanvas) *router.TwoLayerRouter {
	return router.NewTwoLayerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
}

type m3MinAreaDRC struct{ area int }

func (d m3MinAreaDRC) SatisfiesMinArea(seg common.Segment) bool { return seg.GetArea() >= d.area }
func (d m3MinAreaDRC) ApplyEndExtension(lo, hi int) (int, int)  { return lo, hi }
func (d m3MinAreaDRC) ViaEnclosure() int                        { return 0 }
func (d m3MinAreaDRC) ViaTrackSpacing() int                     { return 1 }
func (d m3MinAreaDRC) ApplyMinSpaceExtension(lo, hi int) (int, int) { return lo, hi }

func pins(coords ...[2]int) []common.RoutingPin {
	ps := make([]common.RoutingPin, len(coords))
	for i, c := range coords {
		ps[i] = common.RoutingPin{XLow: c[0], YLow: c[1]}
	}
	return ps
}

// m3Track finds the M3 segment in segs and returns its track ID (LowerLeft.Y / tw).
// Returns -1 when no M3 segment is present (M2-only routing case).
func m3Track(segs []common.Segment, tw int) int {
	for _, s := range segs {
		if s.Layer == common.M3 {
			return s.LowerLeft.Y / tw
		}
	}
	return -1
}

// occupyM3Track marks a single M3 track on the canvas as occupied.
func occupyM3Track(c *canvas.TwoLayerCanvas, trackID, start, end, netID, tw int) error {
	return c.Occupy(common.Segment{
		LowerLeft:    common.Point{X: start, Y: trackID * tw},
		UpperRight:   common.Point{X: end, Y: (trackID + 1) * tw},
		NetID:        netID,
		Layer:        common.M3,
		CanvasOrigin: common.Point{X: 0, Y: 0},
		Dir:          common.Horizontal,
	})
}

// --- basic routing ---

func TestRoute_ClearCanvas_FindsMidTrack(t *testing.T) {
	// canvas 1000x1000, trackWidth=100 → 10 tracks (0-9)
	// from=(100,100) to=(900,900), midY=500 → midTrack=5
	c := newCanvas(1000, 1000, 100)
	r := newRouter(c)

	segs, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	require.NoError(t, err)
	assert.Equal(t, 5, m3Track(segs, 100))
}

func TestRoute_SameY_FindsMidTrack(t *testing.T) {
	// from and to at same Y=200, midY=200 → midTrack=2
	c := newCanvas(1000, 1000, 100)
	r := newRouter(c)

	segs, err := r.Route(pins([2]int{100, 200}, [2]int{900, 200}), 1)

	require.NoError(t, err)
	assert.Equal(t, 2, m3Track(segs, 100))
}

func TestRoute_OutOfBounds_ReturnsError(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := newRouter(c)

	_, err := r.Route(pins([2]int{-1, 0}, [2]int{900, 900}), 1)
	assert.ErrorIs(t, err, router.ErrOutOfBound)

	_, err = r.Route(pins([2]int{100, 100}, [2]int{1001, 900}), 1)
	assert.ErrorIs(t, err, router.ErrOutOfBound)
}

// --- obstacle avoidance ---

func TestRoute_MidTrackM3Blocked_FallsBackToNeighbor(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	require.NoError(t, occupyM3Track(c, 5, 0, 1000, 99, 100))
	r := newRouter(c)

	segs, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	require.NoError(t, err)
	// spacing rule: must be at least 2 tracks away from blocked track 5
	trackID := m3Track(segs, 100)
	assert.True(t, trackID == 3 || trackID == 7)
}

func TestRoute_M2FromBlocked_SkipsTrack(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	// block M2 at from.X=100, overlapping track 5's Y range [500,600]
	require.NoError(t, c.Occupy(common.Segment{Layer: common.M2,
		LowerLeft:  common.Point{X: 100, Y: 500},
		UpperRight: common.Point{X: 101, Y: 600},
		NetID:      99,
	}))
	r := newRouter(c)

	segs, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	require.NoError(t, err)
	assert.NotEqual(t, 5, m3Track(segs, 100))
}

func TestRoute_M2ToBlocked_SkipsTrack(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	// block M2 at to.X=900, overlapping track 5's Y range [500,600]
	require.NoError(t, c.Occupy(common.Segment{Layer: common.M2,
		LowerLeft:  common.Point{X: 900, Y: 500},
		UpperRight: common.Point{X: 901, Y: 600},
		NetID:      99,
	}))
	r := newRouter(c)

	segs, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	require.NoError(t, err)
	assert.NotEqual(t, 5, m3Track(segs, 100))
}

func TestRoute_AllTracksBlocked_ReturnsError(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	for i := 0; i < 10; i++ {
		require.NoError(t, occupyM3Track(c, i, 0, 1000, 99, 100))
	}
	r := newRouter(c)

	_, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	assert.ErrorIs(t, err, router.ErrNoPath)
}

func TestRoute_SameNetID_IgnoresOwnBlocks(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	require.NoError(t, occupyM3Track(c, 5, 0, 1000, 1, 100))
	r := newRouter(c)

	segs, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	require.NoError(t, err)
	assert.Equal(t, 5, m3Track(segs, 100))
}

// --- delta expansion ---

func TestRoute_MidTrackBlocked_ExpandsSymmetrically(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	// block tracks 5 and 6; spacing rule means 4 is also excluded (adjacent to 5)
	// and 7 is excluded (adjacent to 6), so the first valid track is 3
	require.NoError(t, occupyM3Track(c, 5, 0, 1000, 99, 100))
	require.NoError(t, occupyM3Track(c, 6, 0, 1000, 99, 100))
	r := newRouter(c)

	segs, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	require.NoError(t, err)
	assert.Equal(t, 3, m3Track(segs, 100))
}

func TestRoute_MultipleNets_DoNotConflict(t *testing.T) {
	// two nets routed sequentially, second should not overlap first
	c := newCanvas(1000, 1000, 100)
	r := newRouter(c)

	// route net1
	segs1, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)
	require.NoError(t, err)
	trackID1 := m3Track(segs1, 100)

	// mark net1 as occupied
	require.NoError(t, occupyM3Track(c, trackID1, 100, 900, 1, 100))

	// route net2 with same endpoints, should find different track
	segs2, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 2)
	require.NoError(t, err)
	assert.NotEqual(t, trackID1, m3Track(segs2, 100))
}

// --- M2-only fallback ---

// Two pins at the same X with fully overlapping Y ranges → M3 area = m2Width*trackWidth = 100.
// Setting M3 minArea=200 forces the M2-only fallback: two per-pin M2 stubs + one horizontal M2.
//
// Canvas: 1000x1000, trackWidth=100. Pins at X=100, Y=[400,500].
// midY=400 → midTrack=4 → trackYLower=400, trackYUpper=500.
// per-pin stubs: X=[100,101], Y=[400,500] (pin touches track exactly, no ext on either side).
// m2Horiz:       X=[100,101], Y=[400,500] (minX=maxX=100, no m3Ext).
func TestRoute_SameXPins_M3AreaTooSmall_FallsBackToM2Only(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := router.NewTwoLayerRouter(c, 1, common.NoDRC{}, m3MinAreaDRC{area: 200})

	ps := []common.RoutingPin{
		{XLow: 100, YLow: 400, YHigh: 500},
		{XLow: 100, YLow: 400, YHigh: 500},
	}

	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	assert.Equal(t, -1, m3Track(segs, 100), "M2-only sentinel")
	require.Len(t, segs, 3, "2 per-pin stubs + 1 m2Horiz")

	for i := 0; i < 2; i++ {
		assert.Equal(t, 100, segs[i].LowerLeft.X, "stub[%d] XLow", i)
		assert.Equal(t, 101, segs[i].UpperRight.X, "stub[%d] XHigh", i)
		assert.Equal(t, 400, segs[i].LowerLeft.Y, "stub[%d] YLow", i)
		assert.Equal(t, 500, segs[i].UpperRight.Y, "stub[%d] YHigh", i)
	}

	m2Horiz := segs[2]
	assert.Equal(t, 100, m2Horiz.LowerLeft.X)
	assert.Equal(t, 101, m2Horiz.UpperRight.X)
	assert.Equal(t, 400, m2Horiz.LowerLeft.Y)
	assert.Equal(t, 500, m2Horiz.UpperRight.Y)
}
