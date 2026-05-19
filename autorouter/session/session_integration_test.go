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
	return session.NewSession(c, r, nets, common.ViaConfig{}, common.ViaConfig{})
}

// trackIDFromResult extracts the M3 track ID from the middle segment of a NetResult.
// The session always returns segments in order [m2From, m3, m2To].
// trackWidth is the canvas M3 track width (100 in integration tests).
func trackIDFromResult(res session.NetResult, trackWidth int) int {
	m3Seg := res.Segments[1]
	return m3Seg.LowerLeft.Y / trackWidth
}

func TestIntegration_SingleNet_RouteSucceeds(t *testing.T) {
	nets := []*common.Net{{ID: 1, From: common.RoutingPin{XLow: 100, YLow: 100, YHigh: 100}, To: common.RoutingPin{XLow: 900, YLow: 900, YHigh: 900}}}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, 5, trackIDFromResult(results[0], 100))
}

func TestIntegration_MultipleNets_DoNotConflict(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, From: common.RoutingPin{XLow: 0, YLow: 500, YHigh: 500}, To: common.RoutingPin{XLow: 900, YLow: 500, YHigh: 500}},
		{ID: 2, From: common.RoutingPin{XLow: 0, YLow: 500, YHigh: 500}, To: common.RoutingPin{XLow: 900, YLow: 500, YHigh: 500}},
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
		{ID: 1, From: common.RoutingPin{XLow: -1, YLow: 0, YHigh: 0}, To: common.RoutingPin{XLow: 900, YLow: 900, YHigh: 900}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	assert.ErrorIs(t, results[0].Err, router.ErrOutOfBound)
}

func TestIntegration_MixedNets_SuccessAndError(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, From: common.RoutingPin{XLow: 100, YLow: 100, YHigh: 100}, To: common.RoutingPin{XLow: 900, YLow: 900, YHigh: 900}},
		{ID: 2, From: common.RoutingPin{XLow: -1, YLow: 0, YHigh: 0}, To: common.RoutingPin{XLow: 900, YLow: 900, YHigh: 900}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.ErrorIs(t, results[1].Err, router.ErrOutOfBound)
}
