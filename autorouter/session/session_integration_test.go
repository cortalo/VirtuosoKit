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
	c := &canvas.Canvas{
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
	for _, seg := range res.Segments {
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
	for _, seg := range results[0].Segments {
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
