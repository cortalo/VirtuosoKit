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
	r := router.NewTwoLayerRouter(c, 1)
	return session.NewSession(c, r, nets)
}

func TestIntegration_SingleNet_RouteSucceeds(t *testing.T) {
	nets := []*common.Net{{ID: 1, From: common.Point{X: 100, Y: 100}, To: common.Point{X: 900, Y: 900}}}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, 5, results[0].M3.TrackID)
}

func TestIntegration_MultipleNets_DoNotConflict(t *testing.T) {
	// Both nets share the same endpoints at Y=500 (midTrack=5).
	// Net 1 occupies track 5. Net 2's M2 at track 5 overlaps, but track 4's
	// upper edge (500) equals net 1's lower edge (500) — strict inequality means
	// no overlap — so net 2 is forced to track 4.
	nets := []*common.Net{
		{ID: 1, From: common.Point{X: 0, Y: 500}, To: common.Point{X: 900, Y: 500}},
		{ID: 2, From: common.Point{X: 0, Y: 500}, To: common.Point{X: 900, Y: 500}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.NoError(t, results[1].Err)
	assert.Equal(t, 5, results[0].M3.TrackID)
	assert.Equal(t, 4, results[1].M3.TrackID)
}

func TestIntegration_OutOfBoundsNet_ReturnsError(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, From: common.Point{X: -1, Y: 0}, To: common.Point{X: 900, Y: 900}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 1)
	assert.ErrorIs(t, results[0].Err, router.ErrOutOfBound)
}

func TestIntegration_MixedNets_SuccessAndError(t *testing.T) {
	nets := []*common.Net{
		{ID: 1, From: common.Point{X: 100, Y: 100}, To: common.Point{X: 900, Y: 900}},
		{ID: 2, From: common.Point{X: -1, Y: 0}, To: common.Point{X: 900, Y: 900}},
	}
	s := newIntegrationSession(nets)

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.ErrorIs(t, results[1].Err, router.ErrOutOfBound)
}
