package integration_test

import (
	"autorouter/canvas"
	"autorouter/common"
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

	nets, err := netlist.BuildNets(
		"testdata/inv_layout.json",
		"testdata/inv_schematic.json",
		db,
	)
	require.NoError(t, err)
	require.Len(t, nets, 1)

	c := &canvas.Canvas{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 1000_000, Y: 1000_000},
		M2Storage:  canvas.NewSegmentStore(common.Point{X: 0, Y: 0}, common.Point{X: 1000_000, Y: 1000_000}),
		M3Storage:  canvas.NewTrackSegmentStorage(10_000, 100),
	}
	r := router.NewTwoLayerRouter(c)
	s := session.NewSession(c, r, nets)

	results := s.Route()

	require.Len(t, results, 1)
	for i, res := range results {
		assert.NoError(t, res.Err, "net index %d should route without error", i)
	}
}
