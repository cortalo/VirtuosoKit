package integration_test

import (
	"autorouter/canvas"
	"autorouter/netlist"
	"autorouter/pindb"
	"autorouter/router"
	"autorouter/session"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Canvas: 1000x1000, trackWidth=100 (10 tracks).
// Pin coordinates in testdata/pins.toml are chosen so the 4 inv nets
// land on non-conflicting tracks:
//
//	VDD  (sorted first)  → track 6
//	VIN                  → track 3
//	VOUT                 → track 3, different X range (no conflict)
//	VSS  (sorted last)   → track 1
func TestIntegration_InvLayout_AllNetsRoute(t *testing.T) {
	db, err := pindb.Load("testdata/pins.toml")
	require.NoError(t, err)

	const m3TrackWidth = 100

	ll, ur, nets, err := netlist.BuildNets(
		"testdata/inv_layout.json",
		"testdata/inv_schematic.json",
		db,
		nil, nil,
	)
	require.NoError(t, err)
	require.Len(t, nets, 1)

	m3TrackCount := (ur.Y - ll.Y) / m3TrackWidth
	c := &canvas.Canvas{
		LowerLeft:  ll,
		UpperRight: ur,
		M2Storage:  canvas.NewSegmentStore(ll, ur),
		M3Storage:  canvas.NewTrackSegmentStorage(m3TrackCount, m3TrackWidth),
	}
	r := router.NewTwoLayerRouter(c, 1)
	s := session.NewSession(c, r, nets)

	results := s.Route()

	require.Len(t, results, 1)
	for i, res := range results {
		assert.NoError(t, res.Err, "net index %d should route without error", i)
	}
}
