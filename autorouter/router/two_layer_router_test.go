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
	ll := common.Point{X: 0, Y: 0}
	ur := common.Point{X: common.Nm(width), Y: common.Nm(height)}
	return &canvas.TwoLayerCanvas{
		LowerLeft:  ll,
		UpperRight: ur,
		M2Storage:  canvas.NewSegmentStore(ll, ur),
		M3Storage:  canvas.NewTrackSegmentStorage(trackCount, common.Nm(trackWidth)),
	}
}

func newRouter(c *canvas.TwoLayerCanvas) *router.TwoLayerRouter {
	return router.NewTwoLayerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
}

type m3MinAreaDRC struct{ area int }

func (d m3MinAreaDRC) SatisfiesMinArea(seg common.Segment) bool                  { return seg.GetArea() >= d.area }
func (d m3MinAreaDRC) ApplyEndExtension(lo, hi common.Nm) (common.Nm, common.Nm) { return lo, hi }
func (d m3MinAreaDRC) ViaEnclosure() common.Nm                                   { return 0 }
func (d m3MinAreaDRC) ViaTrackSpacing() int                                      { return 1 }
func (d m3MinAreaDRC) ApplyMinSpaceExtension(lo, hi common.Nm) (common.Nm, common.Nm) {
	return lo, hi
}
func (d m3MinAreaDRC) MinPinOverlap() common.Nm { return 0 }

func pins(coords ...[2]int) []common.RoutingPin {
	ps := make([]common.RoutingPin, len(coords))
	for i, c := range coords {
		ps[i] = common.RoutingPin{XLow: common.Nm(c[0]), YLow: common.Nm(c[1])}
	}
	return ps
}

// m3Track finds the M3 segment in segs and returns its track ID (LowerLeft.Y / tw).
// Returns -1 when no M3 segment is present (M2-only routing case).
func m3Track(segs []common.Segment, tw int) int {
	for _, s := range segs {
		if s.Layer == common.M3 {
			return int(s.LowerLeft.Y / common.Nm(tw))
		}
	}
	return -1
}

// occupyM3Track marks a single M3 track on the canvas as occupied.
func occupyM3Track(c *canvas.TwoLayerCanvas, trackID, netID, tw int, start, end common.Nm) error {
	return c.Occupy(common.Segment{
		LowerLeft:    common.Point{X: start, Y: common.Nm(trackID * tw)},
		UpperRight:   common.Point{X: end, Y: common.Nm((trackID + 1) * tw)},
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
	require.NoError(t, occupyM3Track(c, 5, 99, 100, 0, 1000))
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
		require.NoError(t, occupyM3Track(c, i, 99, 100, 0, 1000))
	}
	r := newRouter(c)

	_, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	assert.ErrorIs(t, err, router.ErrNoPath)
}

func TestRoute_SameNetID_IgnoresOwnBlocks(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	require.NoError(t, occupyM3Track(c, 5, 1, 100, 0, 1000))
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
	require.NoError(t, occupyM3Track(c, 5, 99, 100, 0, 1000))
	require.NoError(t, occupyM3Track(c, 6, 99, 100, 0, 1000))
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
	require.NoError(t, occupyM3Track(c, trackID1, 1, 100, 100, 900))

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
		assert.Equal(t, common.Nm(100), segs[i].LowerLeft.X, "stub[%d] XLow", i)
		assert.Equal(t, common.Nm(101), segs[i].UpperRight.X, "stub[%d] XHigh", i)
		assert.Equal(t, common.Nm(400), segs[i].LowerLeft.Y, "stub[%d] YLow", i)
		assert.Equal(t, common.Nm(500), segs[i].UpperRight.Y, "stub[%d] YHigh", i)
	}

	m2Horiz := segs[2]
	assert.Equal(t, common.Nm(100), m2Horiz.LowerLeft.X)
	assert.Equal(t, common.Nm(101), m2Horiz.UpperRight.X)
	assert.Equal(t, common.Nm(400), m2Horiz.LowerLeft.Y)
	assert.Equal(t, common.Nm(500), m2Horiz.UpperRight.Y)
}

// --- min pin overlap ---

// minPinOverlapDRC is a NoDRC variant that only overrides MinPinOverlap.
type minPinOverlapDRC struct {
	common.NoDRC
	overlap common.Nm
}

func (d minPinOverlapDRC) MinPinOverlap() common.Nm { return d.overlap }

// Canvas 1000x1000, trackWidth=100. pin1 Y=[100,400], pin2 Y=[800,900].
// midY=(100+800)/2=450 → track 4 (Y=[400,500]).
// pin1Center=250 < m3Center=450 → pin1 is below M3 → enter from top of pin1.
//
// min overlap (150): lo=max(100, min(400,500)-150)=max(100,250)=250, hi=500 — saves 150nm at bottom.
// full overlap:      lo=min(100,400)=100, hi=max(400,500)=500.
func TestRoute_MinPinOverlap_ShortensM2Bottom(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	drc := minPinOverlapDRC{overlap: 150}
	r := router.NewTwoLayerRouter(c, 1, drc, common.NoDRC{})

	ps := []common.RoutingPin{
		{XLow: 100, YLow: 100, YHigh: 400, MinOverlap: true},
		{XLow: 900, YLow: 800, YHigh: 900},
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	assert.Equal(t, 4, m3Track(segs, 100))

	var m2Left common.Segment
	for _, s := range segs {
		if s.Layer == common.M2 && s.LowerLeft.X == 100 {
			m2Left = s
		}
	}
	assert.Equal(t, common.Nm(250), m2Left.LowerLeft.Y, "min-overlap: M2 bottom = min(pin.YHigh,m3.GetUpper())-overlap")
	assert.Equal(t, common.Nm(500), m2Left.UpperRight.Y, "M2 top at m3.GetUpper()")
}

// Canvas 1000x1000, trackWidth=100. Two pins at YLow=100 → midY=100 → track 1 (Y=[100,200]).
// pin1 Y=[100,400]: pinCenter=250 > m3Center=150 → pin extends above M3 → enter from bottom of pin.
//
// min overlap (150): lo=m3.GetLower()=100, hi=min(400, max(100,100)+150)=min(400,250)=250 — saves 150nm at top.
// full overlap:      lo=min(100,100)=100, hi=max(400,200)=400.
func TestRoute_MinPinOverlap_ShortensM2Top(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	drc := minPinOverlapDRC{overlap: 150}
	r := router.NewTwoLayerRouter(c, 1, drc, common.NoDRC{})

	ps := []common.RoutingPin{
		{XLow: 100, YLow: 100, YHigh: 400, MinOverlap: true},
		{XLow: 900, YLow: 100, YHigh: 400},
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	assert.Equal(t, 1, m3Track(segs, 100))

	var m2Left common.Segment
	for _, s := range segs {
		if s.Layer == common.M2 && s.LowerLeft.X == 100 {
			m2Left = s
		}
	}
	assert.Equal(t, common.Nm(100), m2Left.LowerLeft.Y, "M2 bottom at m3.GetLower()")
	assert.Equal(t, common.Nm(250), m2Left.UpperRight.Y, "min-overlap: M2 top = max(pin.YLow,m3.GetLower())+overlap")
}

// Bug: M2-only fallback (triggered when M3 area < minArea) rebuilds per-pin stubs from
// pin.YLow/pin.YHigh directly, bypassing pinM2Bounds and ignoring pin.MinOverlap.
//
// Canvas 1000x1000, trackWidth=100. Both pins at XLow=100 → M3 span = m2Width = 1,
// area = 100 < m3MinAreaDRC.area=200 → fallback triggered.
// midY=100 → track 1 (Y=[100,200]).
//
// pin1: MinOverlap=true, Y=[100,400]. pinCenter=250 > m3Center=150 → "above M3".
// pinM2Bounds: lo=100, hi=min(400, max(100,100)+150)=250.
// Fallback (buggy): yHigh = pinHi = 400 (pin.YHigh > m3.GetUpper()).
func TestRoute_MinPinOverlap_M2OnlyFallback_RespectsMinOverlap(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := router.NewTwoLayerRouter(c, 1, minPinOverlapDRC{overlap: 150}, m3MinAreaDRC{area: 200})

	ps := []common.RoutingPin{
		{XLow: 100, YLow: 100, YHigh: 400, MinOverlap: true},
		{XLow: 100, YLow: 100, YHigh: 400},
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	assert.Equal(t, -1, m3Track(segs, 100), "M2-only path (no M3 segment)")

	var m2Left common.Segment
	for _, s := range segs {
		if s.Layer == common.M2 && s.LowerLeft.X == 100 {
			m2Left = s
			break
		}
	}
	// MinOverlap=true: M2 top should be m3.GetLower()+minOv = 100+150 = 250, not pin.YHigh = 400.
	assert.Equal(t, common.Nm(250), m2Left.UpperRight.Y, "min-overlap must shorten M2 in M2-only fallback")
}

// Bug: pinM2Bounds "above M3" branch (pinCenterY > m3CenterY) can return hi < m3.GetUpper(),
// meaning M2 does not fully cover the M3 track.
//
// Canvas 1000x1000, trackWidth=100. pin1 Y=[360,380], pin2 Y=[300,400].
// midY=(360+300)/2=330 → track 3 (Y=[300,400]). m3Center=350.
// pin1Center=370 > 350 → "above M3": hi=min(380, max(360,300)+150)=min(380,510)=380 < 400. BUG.
func TestRoute_MinPinOverlap_AboveM3_M2CoversFullTrack(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := router.NewTwoLayerRouter(c, 1, minPinOverlapDRC{overlap: 150}, common.NoDRC{})

	ps := []common.RoutingPin{
		{XLow: 100, YLow: 360, YHigh: 380, MinOverlap: true},
		{XLow: 900, YLow: 300, YHigh: 400},
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	assert.Equal(t, 3, m3Track(segs, 100))

	var m2Left common.Segment
	for _, s := range segs {
		if s.Layer == common.M2 && s.LowerLeft.X == 100 {
			m2Left = s
		}
	}
	// M2 must cover the full M3 track: UpperRight.Y must be >= m3.GetUpper()=400.
	assert.Equal(t, common.Nm(400), m2Left.UpperRight.Y, "min-overlap M2 top must reach m3.GetUpper()")
}

// With MinOverlap=false the full pin height is covered regardless of minPinOverlap.
func TestRoute_FullOverlap_IgnoresMinPinOverlapRule(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	drc := minPinOverlapDRC{overlap: 150}
	r := router.NewTwoLayerRouter(c, 1, drc, common.NoDRC{})

	ps := []common.RoutingPin{
		{XLow: 100, YLow: 100, YHigh: 400, MinOverlap: false},
		{XLow: 900, YLow: 800, YHigh: 900},
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	var m2Left common.Segment
	for _, s := range segs {
		if s.Layer == common.M2 && s.LowerLeft.X == 100 {
			m2Left = s
		}
	}
	assert.Equal(t, common.Nm(100), m2Left.LowerLeft.Y, "full-overlap M2 bottom at pin.YLow")
	assert.Equal(t, common.Nm(500), m2Left.UpperRight.Y, "full-overlap M2 top at max(pin.YHigh, m3.GetUpper())")
}

// --- widen narrow pins ---

// TestRoute_WidenNarrowPins_Disabled_PinUnchanged: default behaviour — narrow pin is not
// widened and no M1 segment is appended.
func TestRoute_WidenNarrowPins_Disabled_PinUnchanged(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := router.NewTwoLayerRouter(c, 100, common.NoDRC{}, common.NoDRC{})

	ps := []common.RoutingPin{
		{XLow: 460, XHigh: 540, YLow: 100, YHigh: 200}, // width=80 < m2Width=100
		{XLow: 400, XHigh: 600, YLow: 800, YHigh: 900}, // width=200, wide enough
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	assert.Equal(t, common.Nm(460), ps[0].XLow, "pin must not be widened when flag is off")
	assert.Equal(t, common.Nm(540), ps[0].XHigh)
	for _, s := range segs {
		assert.NotEqual(t, common.M1, s.Layer, "no M1 segment expected")
	}
}

// TestRoute_WidenNarrowPins_NarrowPin_CenteredAndM1Appended: narrow pin is centred and
// widened to m2Width; a corresponding M1 segment is appended to the result.
//
// Canvas 1000x1000, m2Width=100. pin0: XLow=460, XHigh=540 (width=80, centre=500).
// After widening: XLow=450, XHigh=550. M1 appended at X=[450,550], Y=[100,200].
// pin1 is already wide (width=200) and must not be modified.
func TestRoute_WidenNarrowPins_NarrowPin_CenteredAndM1Appended(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := router.NewTwoLayerRouter(c, 100, common.NoDRC{}, common.NoDRC{})
	r.SetWidenNarrowPins(true)

	ps := []common.RoutingPin{
		{XLow: 460, XHigh: 540, YLow: 100, YHigh: 200}, // width=80 < 100, centre=500
		{XLow: 400, XHigh: 600, YLow: 800, YHigh: 900}, // width=200, unchanged
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)

	// pin widened in-place, centred on original centre=500
	assert.Equal(t, common.Nm(450), ps[0].XLow)
	assert.Equal(t, common.Nm(550), ps[0].XHigh)
	// wide pin untouched
	assert.Equal(t, common.Nm(400), ps[1].XLow)
	assert.Equal(t, common.Nm(600), ps[1].XHigh)

	// exactly one M1 segment appended, matching the widened pin bbox
	var m1s []common.Segment
	for _, s := range segs {
		if s.Layer == common.M1 {
			m1s = append(m1s, s)
		}
	}
	require.Len(t, m1s, 1)
	assert.Equal(t, common.Nm(450), m1s[0].LowerLeft.X)
	assert.Equal(t, common.Nm(550), m1s[0].UpperRight.X)
	assert.Equal(t, common.Nm(100), m1s[0].LowerLeft.Y)
	assert.Equal(t, common.Nm(200), m1s[0].UpperRight.Y)
}

// TestRoute_WidenNarrowPins_AlreadyWide_NotModified: pin whose width already equals or
// exceeds m2Width is not touched and no extra M1 is appended.
func TestRoute_WidenNarrowPins_AlreadyWide_NotModified(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := router.NewTwoLayerRouter(c, 100, common.NoDRC{}, common.NoDRC{})
	r.SetWidenNarrowPins(true)

	ps := []common.RoutingPin{
		{XLow: 400, XHigh: 500, YLow: 100, YHigh: 200}, // width=100 == m2Width, not widened
		{XLow: 400, XHigh: 600, YLow: 800, YHigh: 900}, // width=200 > m2Width, not widened
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	assert.Equal(t, common.Nm(400), ps[0].XLow)
	assert.Equal(t, common.Nm(500), ps[0].XHigh)
	for _, s := range segs {
		assert.NotEqual(t, common.M1, s.Layer)
	}
}

// --- min space ---

// Pre-occupy M3 at X=[0,55] on track 5. The M3 segment runs from X=100 to X=901,
// so without space extension there is no overlap (100 > 55). With space=50 the
// extended segment starts at 50 < 55, making track 5 not passible.
func TestRoute_M3MinSpace_BlocksMidTrack(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	require.NoError(t, c.Occupy(common.Segment{
		Layer:        common.M3,
		LowerLeft:    common.Point{X: 0, Y: 500},
		UpperRight:   common.Point{X: 55, Y: 600},
		NetID:        99,
		CanvasOrigin: common.Point{X: 0, Y: 0},
		Dir:          common.Horizontal,
	}))
	r := router.NewTwoLayerRouter(c, 1, common.NoDRC{}, minSpaceDRC{space: 50})

	segs, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	require.NoError(t, err)
	assert.NotEqual(t, 5, m3Track(segs, 100))
}

// Pre-occupy M2 at X=[100,101], Y=[600,660] — just above M3 track 5's upper bound of 600.
// The vertical M2 stub for the left pin ends at Y=600 (touching, no overlap without extension).
// With space=50 the extended stub reaches Y=650 > 600, blocking track 5.
// Track 4 (M2 ends at Y=500, extended to Y=550 < 600) is unaffected and becomes the fallback.
func TestRoute_M2MinSpace_BlocksMidTrack(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	require.NoError(t, c.Occupy(common.Segment{
		Layer:      common.M2,
		LowerLeft:  common.Point{X: 100, Y: 600},
		UpperRight: common.Point{X: 101, Y: 660},
		NetID:      99,
	}))
	r := router.NewTwoLayerRouter(c, 1, minSpaceDRC{space: 50}, common.NoDRC{})

	segs, err := r.Route(pins([2]int{100, 100}, [2]int{900, 900}), 1)

	require.NoError(t, err)
	assert.Equal(t, 4, m3Track(segs, 100))
}
