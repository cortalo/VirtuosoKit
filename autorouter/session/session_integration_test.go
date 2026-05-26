package session_test

import (
	"autorouter/canvas"
	"autorouter/common"
	"autorouter/router"
	"autorouter/session"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newIntegrationSession(nets []*common.Net) *session.Session {
	c := &canvas.TwoLayerCanvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 1000, Y: 1000},
		M2Storage:  canvas.NewSegmentStore(common.Point{X: 0, Y: 0}, common.Point{X: 1000, Y: 1000}),
		M3Storage:  canvas.NewTrackSegmentStorage(10, 100),
	}
	r := router.NewTwoLayerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
	nl := &common.Netlist{Nets: nets}
	return session.NewSession(c, r, nl, common.ViaConfig{}, common.ViaConfig{}, common.NoDRC{}, common.NoDRC{})
}

// trackIDFromResult finds the M3 segment by layer and returns its track ID.
func trackIDFromResult(res session.NetResult, trackWidth int) int {
	for _, seg := range res.Shapes {
		if seg.Layer == common.M3 {
			return seg.LowerLeft.Y / trackWidth
		}
	}
	panic("no M3 segment in result")
}

func pin(x, y int) common.RoutingPin {
	return common.RoutingPin{XLow: x, YLow: y, YHigh: y}
}

func TestIntegration_SingleNet_RouteSucceeds(t *testing.T) {
	nets := []*common.Net{{ID: 1, Pins: []common.RoutingPin{pin(100, 100), pin(900, 900)}}}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, 5, trackIDFromResult(results[0], 100))
}

func TestIntegration_MultipleNets_DoNotConflict(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, Pins: []common.RoutingPin{pin(0, 500), pin(900, 500)}},
		{ID: 2, Pins: []common.RoutingPin{pin(0, 500), pin(900, 500)}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.NoError(t, results[1].Err)
	assert.Equal(t, 5, trackIDFromResult(results[0], 100))
	assert.Equal(t, 3, trackIDFromResult(results[1], 100))
}

func TestIntegration_OutOfBoundsNet_ReturnsError(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, Pins: []common.RoutingPin{pin(-1, 0), pin(900, 900)}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	assert.ErrorIs(t, results[0].Err, router.ErrOutOfBound)
}

func TestIntegration_MixedNets_SuccessAndError(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, Pins: []common.RoutingPin{pin(100, 100), pin(900, 900)}},
		{ID: 2, Pins: []common.RoutingPin{pin(-1, 0), pin(900, 900)}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.ErrorIs(t, results[1].Err, router.ErrOutOfBound)
}

// TestIntegration_PinBBoxExtension_M2OverlapPanic reproduces a bug where the
// session extends M2 to cover pin.YHigh but the router only checked passibility
// for the un-extended M2 (based on pin.YLow). The grown M2 can overlap a
// different net's already-occupied M2, causing a panic via lo.Must0.
//
// Net 1 pin at YLow=300,YHigh=1000: router M2 Y=[300,400], session extends to [300,1000].
// Net 2 pin at YLow=100,YHigh=400: router M2 Y=[100,200], touches 300 but no
// strict overlap → router accepts. Session extends to [100,400] → overlaps [300,1000] → panic.
func TestIntegration_PinBBoxExtension_M2OverlapPanic(t *testing.T) {
	c := &canvas.TwoLayerCanvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 1000, Y: 1200},
		M2Storage:  canvas.NewSegmentStore(common.Point{X: 0, Y: 0}, common.Point{X: 1000, Y: 1200}),
		M3Storage:  canvas.NewTrackSegmentStorage(12, 100),
	}
	r := router.NewTwoLayerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
	nets := []*common.Net{
		{ID: 1, Pins: []common.RoutingPin{
			{XLow: 100, YLow: 300, YHigh: 1000},
			{XLow: 900, YLow: 300, YHigh: 1000},
		}},
		{ID: 2, Pins: []common.RoutingPin{
			{XLow: 100, YLow: 100, YHigh: 400},
			{XLow: 900, YLow: 100, YHigh: 400},
		}},
	}
	s := session.NewSession(c, r, &common.Netlist{Nets: nets}, common.ViaConfig{}, common.ViaConfig{}, common.NoDRC{}, common.NoDRC{})

	assert.NotPanics(t, func() { s.Route() })
}

// hasM2Bus reports whether shapes contains the full-height M2 bus produced by PowerRouter.
func hasM2Bus(shapes []session.Shape, canvasHeight int) bool {
	for _, sh := range shapes {
		if sh.Layer == common.M2 && sh.LowerLeft.X == 0 && sh.UpperRight.Y == canvasHeight {
			return true
		}
	}
	return false
}

func newIntegrationSessionWithPower(nets []*common.Net, powerNetNames ...string) *session.Session {
	c := &canvas.TwoLayerCanvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 1000, Y: 1000},
		M2Storage:  canvas.NewSegmentStore(common.Point{X: 0, Y: 0}, common.Point{X: 1000, Y: 1000}),
		M3Storage:  canvas.NewTrackSegmentStorage(10, 100),
	}
	r := router.NewTwoLayerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
	pr := router.NewPowerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
	nl := &common.Netlist{Nets: nets}
	s := session.NewSession(c, r, nl, common.ViaConfig{}, common.ViaConfig{}, common.NoDRC{}, common.NoDRC{})
	s.SetPowerRouter(pr, powerNetNames...)
	return s
}

// TestIntegration_PowerNet_UsesPowerRouter: VDD net gets M2 bus; signal net does not.
func TestIntegration_PowerNet_UsesPowerRouter(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, Name: "VDD", Pins: []common.RoutingPin{pin(300, 800), pin(700, 200)}},
		{ID: 2, Name: "sig", Pins: []common.RoutingPin{pin(100, 100), pin(900, 500)}},
	}
	s := newIntegrationSessionWithPower(nets, "VDD")

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.NoError(t, results[1].Err)
	assert.True(t, hasM2Bus(results[0].Shapes, 1000), "VDD net must have M2 bus from PowerRouter")
	assert.False(t, hasM2Bus(results[1].Shapes, 1000), "signal net must not have M2 bus")
}

// TestIntegration_PowerNet_NoPowerRouterSet_FallsBackToRegularRouter: if SetPowerRouter
// was not called, power-named nets are routed by the regular router (no M2 bus).
func TestIntegration_PowerNet_NoPowerRouterSet_FallsBackToRegularRouter(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, Name: "VDD", Pins: []common.RoutingPin{pin(100, 100), pin(900, 900)}},
	}
	s := newIntegrationSession(nets) // no SetPowerRouter

	results := s.Route()

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.False(t, hasM2Bus(results[0].Shapes, 1000), "without SetPowerRouter, VDD uses regular router")
}

func TestIntegration_ThreePinNet_RouteSucceeds(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, Pins: []common.RoutingPin{pin(100, 100), pin(500, 900), pin(900, 200)}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	// 3 M2 stubs + 1 M3
	m2Count := 0
	m3Count := 0
	for _, seg := range results[0].Shapes {
		switch seg.Layer {
		case common.M2:
			m2Count++
		case common.M3:
			m3Count++
		}
	}
	assert.Equal(t, 3, m2Count)
	assert.Equal(t, 1, m3Count)
}
