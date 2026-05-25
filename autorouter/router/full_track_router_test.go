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

// endExtDRC is a DRCSpec with a fixed symmetric end extension and no min-area constraint.
type endExtDRC struct{ ext int }

func (d endExtDRC) SatisfiesMinArea(_ common.Segment) bool       { return true }
func (d endExtDRC) ApplyEndExtension(lo, hi int) (int, int)      { return lo - d.ext, hi + d.ext }
func (d endExtDRC) ViaEnclosure() int                            { return d.ext }
func (d endExtDRC) ViaTrackSpacing() int                         { return 1 }
func (d endExtDRC) ApplyMinSpaceExtension(lo, hi int) (int, int) { return lo, hi }
func (d endExtDRC) MinPinOverlap() int                           { return 0 }

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
// No M3 is needed; the router returns one M2 per pin plus one full-range M2 connector.
func TestFTRoute_SameM2Track_NoM3(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := newFTRouter(c)

	segs, err := r.Route(ftPins(
		[4]int{0, 100, 100, 200},
		[4]int{0, 100, 500, 600},
	), 1)

	require.NoError(t, err)
	assert.Equal(t, -1, m3Track(segs, 100), "no M3 when all pins on same M2 track")
	require.Len(t, segs, 3, "one M2 per pin plus one full-range connector, no M3")
	assert.Equal(t, common.M2, segs[0].Layer)
	assert.Equal(t, common.M2, segs[1].Layer)
	assert.Equal(t, common.M2, segs[2].Layer)
	// pin0 M2 covers its own Y range
	assert.Equal(t, 100, segs[0].LowerLeft.Y)
	assert.Equal(t, 200, segs[0].UpperRight.Y)
	// pin1 M2 covers its own Y range
	assert.Equal(t, 500, segs[1].LowerLeft.Y)
	assert.Equal(t, 600, segs[1].UpperRight.Y)
	// connector M2 spans full range, no DRC extension
	assert.Equal(t, 0, segs[2].LowerLeft.X, "connector on same M2 track 0")
	assert.Equal(t, 100, segs[2].UpperRight.X)
	assert.Equal(t, 100, segs[2].LowerLeft.Y, "connector starts at min pin start")
	assert.Equal(t, 600, segs[2].UpperRight.Y, "connector ends at max pin end")
}

// Three pins all on the same M2 track; connector must span the full extent.
// Pin0: Y=[50,150], Pin1: Y=[300,400], Pin2: Y=[700,800] → connector Y=[50,800].
func TestFTRoute_SameM2Track_ThreePins_ConnectorSpansAll(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := newFTRouter(c)

	segs, err := r.Route(ftPins(
		[4]int{0, 100, 50, 150},
		[4]int{0, 100, 300, 400},
		[4]int{0, 100, 700, 800},
	), 1)

	require.NoError(t, err)
	assert.Equal(t, -1, m3Track(segs, 100), "no M3 when all pins on same M2 track")
	require.Len(t, segs, 4, "one M2 per pin plus one full-range connector")

	// Find the connector: longest M2 segment.
	var connector common.Segment
	for _, s := range segs {
		if s.Layer == common.M2 && (s.UpperRight.Y-s.LowerLeft.Y) > (connector.UpperRight.Y-connector.LowerLeft.Y) {
			connector = s
		}
	}
	assert.Equal(t, 50, connector.LowerLeft.Y, "connector starts at min pin start")
	assert.Equal(t, 800, connector.UpperRight.Y, "connector ends at max pin end")
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

// --- ViaTrackSpacing ---

// viaSpacingDRC is a NoDRC variant that only overrides ViaTrackSpacing.
type viaSpacingDRC struct {
	common.NoDRC
	spacing int
}

func (d viaSpacingDRC) ViaTrackSpacing() int { return d.spacing }

// Canvas 1000x1000, m2tw=100, m3tw=100. Pin1→track 0, Pin2→track 2 only.
// With spacing=1 (default): |diff|=2 > 1 → succeeds.
// With spacing=3: |diff|=2 <= 3 → rejected; no other candidates → ErrNoPath.
func TestFTRoute_ViaTrackSpacingDefault_AllowsGapOfTwo(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := newFTRouter(c)

	pins := []common.RoutingPin{
		{XLow: 0, XHigh: 100, YLow: 100, YHigh: 200},
		{XLow: 200, XHigh: 300, YLow: 100, YHigh: 200},
	}
	segs, err := r.Route(pins, 1)
	require.NoError(t, err)

	var m2Tracks []int
	for _, s := range segs {
		if s.Layer == common.M2 {
			m2Tracks = append(m2Tracks, s.LowerLeft.X/100)
		}
	}
	require.Len(t, m2Tracks, 2)
	diff := m2Tracks[0] - m2Tracks[1]
	if diff < 0 {
		diff = -diff
	}
	assert.Equal(t, 2, diff, "with default spacing=1, tracks 0 and 2 (|diff|=2) should be chosen")
}

func TestFTRoute_ViaTrackSpacing_RejectsCloseConfiguration(t *testing.T) {
	// spacing=3: pin1→track 0, pin2→track 2 only. |diff|=2 ≤ 3 → ErrNoPath.
	c := newFTCanvas(1000, 1000, 100, 100)
	r := router.NewFullTrackRouter(c, common.Vertical, viaSpacingDRC{spacing: 3}, common.NoDRC{})

	pins := []common.RoutingPin{
		{XLow: 0, XHigh: 100, YLow: 100, YHigh: 200},
		{XLow: 200, XHigh: 300, YLow: 100, YHigh: 200},
	}
	_, err := r.Route(pins, 1)
	assert.ErrorIs(t, err, router.ErrNoPath)
}

func TestFTRoute_ViaTrackSpacing_RouterPicksFarTrack(t *testing.T) {
	// spacing=3: pin1 spans tracks 0-1, pin2 spans tracks 4-5.
	// Closest pair would be (1,4) with |diff|=3 → rejected.
	// Router must find (0,4) or (1,5) with |diff|=4 → accepted.
	c := newFTCanvas(1000, 1000, 100, 100)
	r := router.NewFullTrackRouter(c, common.Vertical, viaSpacingDRC{spacing: 3}, common.NoDRC{})

	pins := []common.RoutingPin{
		{XLow: 0, XHigh: 200, YLow: 100, YHigh: 200},   // candidates: tracks 0,1
		{XLow: 400, XHigh: 600, YLow: 100, YHigh: 200}, // candidates: tracks 4,5
	}
	segs, err := r.Route(pins, 1)
	require.NoError(t, err)

	var m2Tracks []int
	for _, s := range segs {
		if s.Layer == common.M2 {
			m2Tracks = append(m2Tracks, s.LowerLeft.X/100)
		}
	}
	require.Len(t, m2Tracks, 2)
	diff := m2Tracks[0] - m2Tracks[1]
	if diff < 0 {
		diff = -diff
	}
	assert.Greater(t, diff, 3, "chosen M2 tracks must be more than 3 apart (spacing=3)")
}

// --- MinSpace ---

// minSpaceDRC is a NoDRC variant that only overrides ApplyMinSpaceExtension.
type minSpaceDRC struct {
	common.NoDRC
	space int
}

func (d minSpaceDRC) ApplyMinSpaceExtension(lo, hi int) (int, int) { return lo - d.space, hi + d.space }

// Canvas 1000×1000, m2tw=100, m3tw=100, m2DRC.minSpace=50.
// Occupied M2 track 0: Y=[0,400]. Pin1 on track 0 only.
//
// Rejection: pin1 Y=[440,540].
//   For any M3 track t, m2Start = min(440, t*100) ≤ 440.
//   Space-extended start ≤ 440-50=390 < 400 → always overlaps [0,400] → ErrNoPath.
//
// Acceptance: pin1 Y=[460,560].
//   M3 track 5 (m3Lower=500): m2Start=min(460,500)=460, extended_start=410 > 400
//   → no overlap → routing succeeds.
func TestFTRoute_M2MinSpace_RejectsSegmentTooClose(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	require.NoError(t, occupyFTM2Track(c, 0, 0, 400, -1, 100))

	r := router.NewFullTrackRouter(c, common.Vertical, minSpaceDRC{space: 50}, common.NoDRC{})

	pins := []common.RoutingPin{
		{XLow: 0, XHigh: 100, YLow: 440, YHigh: 540},   // track 0 only; gap=40 < 50
		{XLow: 500, XHigh: 600, YLow: 440, YHigh: 540}, // track 5, unobstructed
	}
	_, err := r.Route(pins, 1)
	assert.ErrorIs(t, err, router.ErrNoPath, "gap < minSpace must be rejected")
}

func TestFTRoute_M2MinSpace_AcceptsSegmentFarEnough(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	require.NoError(t, occupyFTM2Track(c, 0, 0, 400, -1, 100))

	r := router.NewFullTrackRouter(c, common.Vertical, minSpaceDRC{space: 50}, common.NoDRC{})

	pins := []common.RoutingPin{
		{XLow: 0, XHigh: 100, YLow: 460, YHigh: 560},   // track 0; M3 track 5 gives gap=60 > 50
		{XLow: 500, XHigh: 600, YLow: 460, YHigh: 560}, // track 5, unobstructed
	}
	_, err := r.Route(pins, 1)
	assert.NoError(t, err, "gap >= minSpace must be accepted")
}

// --- M2-layer RoutingPin ---

// An M2-layer pin should not have ApplyEndExtension applied.
// Canvas 1000x1000, m2tw=100, m3tw=100, NoDRC (no-op end extension).
// Pin1: M1 at X=[0,100] Y=[100,200]  → M2 track 0.
// Pin2: M2 at X=[500,600] Y=[100,200] → M2 track 5.
// With NoDRC both behave the same — test just verifies routing succeeds and
// produces the expected M2 segments with no panics or errors.
func TestFTRoute_M2LayerPin_RoutesSuccessfully(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := newFTRouter(c)

	pins := []common.RoutingPin{
		{Layer: common.M1, XLow: 0, XHigh: 100, YLow: 100, YHigh: 200},
		{Layer: common.M2, XLow: 500, XHigh: 600, YLow: 100, YHigh: 200},
	}
	segs, err := r.Route(pins, 1)

	require.NoError(t, err)
	require.Len(t, segs, 3, "one M2 per pin plus one M3")
	assert.Equal(t, common.M2, segs[0].Layer)
	assert.Equal(t, common.M2, segs[1].Layer)
	assert.Equal(t, common.M3, segs[2].Layer)
}

// All-same-M2-track path with an M2-layer pin: no M3, just per-pin M2s + connector.
func TestFTRoute_M2LayerPin_SameTrack(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := newFTRouter(c)

	// Both pins on M2 track 3 (X=[300,400]).
	pins := []common.RoutingPin{
		{Layer: common.M1, XLow: 300, XHigh: 400, YLow: 100, YHigh: 200},
		{Layer: common.M2, XLow: 300, XHigh: 400, YLow: 600, YHigh: 700},
	}
	segs, err := r.Route(pins, 1)

	require.NoError(t, err)
	assert.Equal(t, -1, m3Track(segs, 100), "no M3 when all pins on same M2 track")
	require.Len(t, segs, 3, "one M2 per pin plus connector")
}

// TestFTRoute_M2Pin_EndExtensionApplied exposes the bug where an M2-layer pin
// gets no DRC end extension, leaving the M2 segment too short to satisfy Via23
// enclosure at the M3 connection.
//
// Canvas 1000×1000, m2tw=100, m3tw=100, M2=Vertical, m2DRC.endExtension=50.
// Pin1 (M1): X=[0,100]   Y=[520,580] → M2 track 0.
// Pin2 (M2): X=[200,300] Y=[520,580] → M2 track 2. Both pins inside M3 band.
// midY=(520+520)/2=520 → M3 track 5 (Y=[500,600]).
//
// Correct M2 for pin2: ApplyEndExtension(m3Lower=500, m3Upper=600) = [450,650].
// Bug: extension skipped entirely → [500,600].
func TestFTRoute_M2Pin_EndExtensionApplied(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := router.NewFullTrackRouter(c, common.Vertical, endExtDRC{50}, common.NoDRC{})

	pins := []common.RoutingPin{
		{Layer: common.M1, XLow: 0, XHigh: 100, YLow: 520, YHigh: 580},
		{Layer: common.M2, XLow: 200, XHigh: 300, YLow: 520, YHigh: 580},
	}
	segs, err := r.Route(pins, 1)
	require.NoError(t, err)

	var m2Pin2 common.Segment
	for _, s := range segs {
		if s.Layer == common.M2 && s.LowerLeft.X == 200 {
			m2Pin2 = s
			break
		}
	}
	assert.Equal(t, 450, m2Pin2.LowerLeft.Y, "M2 pin segment must extend past M3 lower bound (bug: no DRC extension)")
	assert.Equal(t, 650, m2Pin2.UpperRight.Y, "M2 pin segment must extend past M3 upper bound (bug: no DRC extension)")
}

// TestFTRoute_M2Pin_PinLargerThanExtendedM3 verifies that when an M2-layer
// pin already extends beyond the extended M3 band, the full pin range is used.
//
// Canvas 1000×1000, m2tw=100, m3tw=100, M2=Vertical, m2DRC.endExtension=50.
// Pin1 (M1): X=[0,100]   Y=[520,580] → M2 track 0.
// Pin2 (M2): X=[200,300] Y=[400,700] → M2 track 2; pin extends beyond M3 band.
// midY=(520+400)/2=460 → M3 track 4 (Y=[400,500]).
// Extended M3 band: [350,550]. Pin2 range [400,700] exceeds the upper end.
//
// Correct M2 for pin2: m2Start=min(350,400)=350, m2End=max(550,700)=700 → [350,700].
// Bug: lower bound not extended → m2Start=400.
func TestFTRoute_M2Pin_PinLargerThanExtendedM3(t *testing.T) {
	c := newFTCanvas(1000, 1000, 100, 100)
	r := router.NewFullTrackRouter(c, common.Vertical, endExtDRC{50}, common.NoDRC{})

	pins := []common.RoutingPin{
		{Layer: common.M1, XLow: 0, XHigh: 100, YLow: 520, YHigh: 580},
		{Layer: common.M2, XLow: 200, XHigh: 300, YLow: 400, YHigh: 700},
	}
	segs, err := r.Route(pins, 1)
	require.NoError(t, err)

	var m2Pin2 common.Segment
	for _, s := range segs {
		if s.Layer == common.M2 && s.LowerLeft.X == 200 {
			m2Pin2 = s
			break
		}
	}
	assert.Equal(t, 350, m2Pin2.LowerLeft.Y, "M2 pin lower end must extend to M3 extension even when pin is large")
	assert.Equal(t, 700, m2Pin2.UpperRight.Y, "M2 pin upper end must use pin range when larger than extension")
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
