package router_test

import (
	"autorouter/common"
	"autorouter/router"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPowerRouter(c interface {
	router.Canvas
}) *router.PowerRouter {
	return router.NewPowerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
}

// m2BusInSegs returns the full-height M2 bus segment (starts at X=0, reaches canvasHeight).
func m2BusInSegs(segs []common.Segment, canvasHeight common.Nm) (common.Segment, bool) {
	for _, s := range segs {
		if s.Layer == common.M2 && s.LowerLeft.X == 0 && s.UpperRight.Y == canvasHeight {
			return s, true
		}
	}
	return common.Segment{}, false
}

// powerM3Segs returns all M3 segments from a power route result.
func powerM3Segs(segs []common.Segment) []common.Segment {
	var out []common.Segment
	for _, s := range segs {
		if s.Layer == common.M3 {
			out = append(out, s)
		}
	}
	return out
}

// TestPowerRoute_SinglePin_FindsNearestTrack: pin at YLow=300, tw=100 → midTrack=3.
func TestPowerRoute_SinglePin_FindsNearestTrack(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := newPowerRouter(c)

	segs, err := r.Route([]common.RoutingPin{{XLow: 500, YLow: 300}}, 1)

	require.NoError(t, err)
	m3s := powerM3Segs(segs)
	require.Len(t, m3s, 1)
	assert.Equal(t, common.Nm(3), m3s[0].LowerLeft.Y/100)
}

// TestPowerRoute_M3ExtendsToLeftEdge: M3 segment must start at X=0 to meet the M2 bus.
func TestPowerRoute_M3ExtendsToLeftEdge(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := newPowerRouter(c)

	segs, err := r.Route([]common.RoutingPin{{XLow: 500, YLow: 300}}, 1)

	require.NoError(t, err)
	m3s := powerM3Segs(segs)
	require.Len(t, m3s, 1)
	assert.Equal(t, common.Nm(0), m3s[0].LowerLeft.X, "M3 must reach left edge to connect to M2 bus")
}

// TestPowerRoute_M2BusSpansFullHeight: M2 bus at X=0 spans the full canvas height.
func TestPowerRoute_M2BusSpansFullHeight(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := newPowerRouter(c)

	segs, err := r.Route([]common.RoutingPin{{XLow: 500, YLow: 300}}, 1)

	require.NoError(t, err)
	bus, ok := m2BusInSegs(segs, 1000)
	require.True(t, ok, "M2 bus must be present")
	assert.Equal(t, common.Nm(0), bus.LowerLeft.Y)
	assert.Equal(t, common.Nm(1000), bus.UpperRight.Y)
}

// TestPowerRoute_MultiPins_EachGetOwnM3Track: pins at different Y get independent M3 tracks.
func TestPowerRoute_MultiPins_EachGetOwnM3Track(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := newPowerRouter(c)

	ps := []common.RoutingPin{
		{XLow: 100, YLow: 100}, // midTrack=1
		{XLow: 700, YLow: 700}, // midTrack=7
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	m3s := powerM3Segs(segs)
	require.Len(t, m3s, 2)
	trackIDs := map[int]bool{}
	for _, s := range m3s {
		trackIDs[int(s.LowerLeft.Y/100)] = true
	}
	assert.True(t, trackIDs[1], "pin at YLow=100 → track 1")
	assert.True(t, trackIDs[7], "pin at YLow=700 → track 7")
}

// TestPowerRoute_NearestTrackBlocked_FallsBack: blocked track forces adjacent track.
func TestPowerRoute_NearestTrackBlocked_FallsBack(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	require.NoError(t, occupyM3Track(c, 3, 99, 100, 0, 1000))
	r := newPowerRouter(c)

	segs, err := r.Route([]common.RoutingPin{{XLow: 500, YLow: 300}}, 1)

	require.NoError(t, err)
	m3s := powerM3Segs(segs)
	require.Len(t, m3s, 1)
	assert.NotEqual(t, common.Nm(3), m3s[0].LowerLeft.Y/100, "blocked track must be skipped")
}

// TestPowerRoute_FirstNSegs_AreM2StubsInPinOrder: segs[0..N-1] must be M2 stubs
// corresponding to pins[0..N-1] in order (identified by matching XLow).
func TestPowerRoute_FirstNSegs_AreM2StubsInPinOrder(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	r := newPowerRouter(c)

	ps := []common.RoutingPin{
		{XLow: 100, YLow: 100},
		{XLow: 500, YLow: 500},
		{XLow: 800, YLow: 800},
	}
	segs, err := r.Route(ps, 1)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(segs), len(ps))
	for i, pin := range ps {
		assert.Equal(t, common.M2, segs[i].Layer, "segs[%d] must be M2 stub", i)
		assert.Equal(t, pin.XLow, segs[i].LowerLeft.X, "segs[%d].X must match pin[%d].XLow", i, i)
	}
}

// TestPowerRoute_AllTracksBlocked_ReturnsErrNoPath: no valid M3 track → ErrNoPath.
func TestPowerRoute_AllTracksBlocked_ReturnsErrNoPath(t *testing.T) {
	c := newCanvas(1000, 1000, 100)
	for i := 0; i < 10; i++ {
		require.NoError(t, occupyM3Track(c, i, 99, 100, 0, 1000))
	}
	r := newPowerRouter(c)

	_, err := r.Route([]common.RoutingPin{{XLow: 500, YLow: 300}}, 1)

	assert.ErrorIs(t, err, router.ErrNoPath)
}
