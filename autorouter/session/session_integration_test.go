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
	return session.NewSession(c, r, nets)
}

func TestIntegration_SingleNet_RouteSucceeds(t *testing.T) {
	nets := []*common.Net{{ID: 1, From: common.RoutingPin{XLow: 100, YLow: 100, YHigh: 100}, To: common.RoutingPin{XLow: 900, YLow: 900, YHigh: 900}}}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, 5, results[0].M3.TrackID)
}

func TestIntegration_MultipleNets_DoNotConflict(t *testing.T) {
	// Both nets share the same endpoints at Y=500 (midTrack=5).
	// Net 1 occupies track 5. Spacing rule forbids tracks 4 and 6 (adjacent to 5),
	// so net 2 is forced to track 3.
	nets := []*common.Net{
		{ID: 1, From: common.RoutingPin{XLow: 0, YLow: 500, YHigh: 500}, To: common.RoutingPin{XLow: 900, YLow: 500, YHigh: 500}},
		{ID: 2, From: common.RoutingPin{XLow: 0, YLow: 500, YHigh: 500}, To: common.RoutingPin{XLow: 900, YLow: 500, YHigh: 500}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.NoError(t, results[1].Err)
	assert.Equal(t, 5, results[0].M3.TrackID)
	assert.Equal(t, 3, results[1].M3.TrackID)
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
