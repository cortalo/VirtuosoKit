package integration_test

import (
	"autorouter/canvas"
	"autorouter/common"
	"autorouter/router"
	"autorouter/session"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFTSession creates a Session backed by FullTrackCanvas + FullTrackRouter.
// Canvas: 1000x1000, M2=100nm vertical (10 tracks), M3=100nm horizontal (10 tracks).
func newFTSession(nets []*common.Net) *session.Session {
	c := &canvas.FullTrackCanvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 1000, Y: 1000},
		M2Storage:  canvas.NewTrackSegmentStorage(10, common.Nm(100)),
		M3Storage:  canvas.NewTrackSegmentStorage(10, common.Nm(100)),
		M2Dir:      common.Vertical,
	}
	r := router.NewFullTrackRouter(c, common.Vertical, common.NoDRC{}, common.NoDRC{})
	nl := &common.Netlist{Nets: nets}
	return session.NewSession(c, r, nl, common.ViaConfig{}, common.ViaConfig{}, common.NoDRC{}, common.NoDRC{})
}

func ftTrackID(res session.NetResult, trackWidth int) int {
	for _, seg := range res.Shapes {
		if seg.Layer == common.M3 {
			return int(seg.LowerLeft.Y / common.Nm(trackWidth))
		}
	}
	panic("no M3 segment in result")
}

// --- single net ---

// Pin1: X=[0,100] Y=[100,200] → M2 track 0.
// Pin2: X=[900,1000] Y=[700,800] → M2 track 9.
// midY=(100+700)/2=400 → M3 track 4 (Y=[400,500]).
func TestFTIntegration_SingleNet_RouteSucceeds(t *testing.T) {
	nets := []*common.Net{{
		ID: 1,
		Pins: []common.RoutingPin{
			{XLow: 0, XHigh: 100, YLow: 100, YHigh: 200},
			{XLow: 900, XHigh: 1000, YLow: 700, YHigh: 800},
		},
	}}
	s := newFTSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	require.NoError(t, results[0].Err)

	var m2Count, m3Count int
	for _, seg := range results[0].Shapes {
		switch seg.Layer {
		case common.M2:
			m2Count++
		case common.M3:
			m3Count++
		}
	}
	assert.Equal(t, 2, m2Count, "one M2 stub per pin")
	assert.Equal(t, 1, m3Count, "one M3 track segment")
	assert.Equal(t, 4, ftTrackID(results[0], 100))
}

// --- two nets, sequential conflict avoidance ---

// Net1: Pin1 X=[0,100] Y=[100,200], Pin2 X=[900,1000] Y=[700,800].
//
//	→ M3 track 4 (midY=400). M3 X=[0,1000] fully spans canvas.
//
// Net2: Pin1 X=[200,300] Y=[600,700], Pin2 X=[600,700] Y=[200,300].
//
//	→ midY=400 → track 4 blocked; tracks 3 and 5 also blocked (adjacent spacing).
//	→ first valid: track 6 (delta=2, prev=5 passible).
func TestFTIntegration_TwoNets_ConflictAvoidance(t *testing.T) {
	nets := []*common.Net{
		{
			ID: 1,
			Pins: []common.RoutingPin{
				{XLow: 0, XHigh: 100, YLow: 100, YHigh: 200},
				{XLow: 900, XHigh: 1000, YLow: 700, YHigh: 800},
			},
		},
		{
			ID: 2,
			Pins: []common.RoutingPin{
				{XLow: 200, XHigh: 300, YLow: 600, YHigh: 700},
				{XLow: 600, XHigh: 700, YLow: 200, YHigh: 300},
			},
		},
	}
	s := newFTSession(nets)

	results := s.Route()

	require.Len(t, results, 2)
	require.NoError(t, results[0].Err, "net 1 should route")
	require.NoError(t, results[1].Err, "net 2 should route")

	track1 := ftTrackID(results[0], 100)
	track2 := ftTrackID(results[1], 100)
	assert.Equal(t, 4, track1, "net1 routes on midTrack")
	assert.Equal(t, 6, track2, "net2 skips blocked track 4 and adjacent 3,5 → track 6")
	assert.NotEqual(t, track1, track2, "nets must use different M3 tracks")
}

// --- instance metals block routing ---

// Instance metal covers M3 track 4 (Y=[400,500]).
// Net midY=400 → ideal track 4, but it's occupied by instance metal (netID=-1).
// Adjacent tracks 3 and 5 are also blocked. Router should land on track 6.
func TestFTIntegration_InstanceMetal_BlocksM3Track(t *testing.T) {
	inst := canvas.Instance{
		XY:     common.Point{X: 0, Y: 0},
		Orient: canvas.R0,
		Metals: []common.Shape{{
			LowerLeft:  common.Point{X: 0, Y: 400},
			UpperRight: common.Point{X: 1000, Y: 500},
			Layer:      common.M3,
		}},
	}
	c, err := canvas.NewFullTrackCanvas(
		common.Point{X: 0, Y: 0},
		common.Point{X: 1000, Y: 1000},
		canvas.NewTrackSegmentStorage(10, common.Nm(100)),
		canvas.NewTrackSegmentStorage(10, common.Nm(100)),
		common.Vertical,
		[]canvas.Instance{inst},
	)
	require.NoError(t, err)

	r := router.NewFullTrackRouter(c, common.Vertical, common.NoDRC{}, common.NoDRC{})
	nl := &common.Netlist{Nets: []*common.Net{{
		ID: 1,
		Pins: []common.RoutingPin{
			{XLow: 0, XHigh: 100, YLow: 100, YHigh: 200},
			{XLow: 900, XHigh: 1000, YLow: 700, YHigh: 800},
		},
	}}}
	s := session.NewSession(c, r, nl, common.ViaConfig{}, common.ViaConfig{}, common.NoDRC{}, common.NoDRC{})

	results := s.Route()

	require.Len(t, results, 1)
	require.NoError(t, results[0].Err)
	assert.Equal(t, 6, ftTrackID(results[0], 100), "track 4 blocked by instance metal → routed on track 6")
}

// --- all M3 tracks blocked → second net fails ---

// Net1 fills track 4. Then pre-occupy all remaining M3 tracks via a dummy routing session.
// Net2 should return ErrNoPath.
func TestFTIntegration_SecondNet_NoPath_WhenAllTracksBlocked(t *testing.T) {
	c := &canvas.FullTrackCanvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 1000, Y: 1000},
		M2Storage:  canvas.NewTrackSegmentStorage(10, common.Nm(100)),
		M3Storage:  canvas.NewTrackSegmentStorage(10, common.Nm(100)),
		M2Dir:      common.Vertical,
	}
	// Block all 10 M3 tracks with netID=99.
	for i := 0; i < 10; i++ {
		require.NoError(t, c.Occupy(common.Segment{
			LowerLeft:    common.Point{X: 0, Y: common.Nm(i * 100)},
			UpperRight:   common.Point{X: 1000, Y: common.Nm((i + 1) * 100)},
			NetID:        99,
			Layer:        common.M3,
			CanvasOrigin: common.Point{X: 0, Y: 0},
			Dir:          common.Horizontal,
		}))
	}

	r := router.NewFullTrackRouter(c, common.Vertical, common.NoDRC{}, common.NoDRC{})
	nl := &common.Netlist{Nets: []*common.Net{{
		ID: 1,
		Pins: []common.RoutingPin{
			{XLow: 0, XHigh: 100, YLow: 100, YHigh: 200},
			{XLow: 900, XHigh: 1000, YLow: 700, YHigh: 800},
		},
	}}}
	s := session.NewSession(c, r, nl, common.ViaConfig{}, common.ViaConfig{}, common.NoDRC{}, common.NoDRC{})

	results := s.Route()

	require.Len(t, results, 1)
	assert.ErrorIs(t, results[0].Err, router.ErrNoPath)
}
