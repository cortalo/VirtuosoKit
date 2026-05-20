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

// TestIntegration_Inv2Layout_Orientation verifies that orientation-transformed pins
// (I0 with MX, I1 with MY) are correctly placed so that net2 (I0.ZN → I1.I) routes
// and that the output segments reflect the mirrored pin coordinates.
//
// Pin data (pins.toml nm coords, relative to cell origin):
//
//	I:  ll=[100,100] ur=[200,300]
//	ZN: ll=[800,100] ur=[900,300]
//
// After transforms:
//
//	I0 (MX, at 0,0):  ZN → XLow=800  XHigh=900  YLow=-300 YHigh=-100
//	I1 (MY, at 5,0µm): I → XLow=4800 XHigh=4900 YLow=100  YHigh=300
//
// Router selects track 99 (trackY=-100..0) — midY=(-300+100)/2=-100.
// Session extends M2 to cover pin YHigh, so I1.I M2 top reaches Y=300.
func TestIntegration_Inv2Layout_Orientation(t *testing.T) {
	db, err := pindb.Load("testdata/pins.toml")
	require.NoError(t, err)

	const m3TrackWidth = 100

	ll, ur, nl, err := netlist.BuildNets(
		"testdata/inv2_layout.json",
		"testdata/inv2_schematic.json",
		db,
		nil, nil,
	)
	require.NoError(t, err)
	require.Len(t, nl.Nets, 1, "only net2 has 2+ pins")

	// VIN → I0.I (MX at 0,0): I pin YLow/YHigh negated
	// VOUT → I1.ZN (MY at 5000,0): ZN pin X negated then shifted
	require.Len(t, nl.Pins, 2)
	assert.Equal(t, "VIN", nl.Pins[0].Name)
	assert.Equal(t, 100, nl.Pins[0].XLow)
	assert.Equal(t, 200, nl.Pins[0].XHigh)
	assert.Equal(t, -300, nl.Pins[0].YLow)
	assert.Equal(t, -100, nl.Pins[0].YHigh)
	assert.Equal(t, "VOUT", nl.Pins[1].Name)
	assert.Equal(t, 4100, nl.Pins[1].XLow)
	assert.Equal(t, 4200, nl.Pins[1].XHigh)
	assert.Equal(t, 100, nl.Pins[1].YLow)
	assert.Equal(t, 300, nl.Pins[1].YHigh)

	m3TrackCount := (ur.Y - ll.Y) / m3TrackWidth
	c := &canvas.Canvas{
		LowerLeft:  ll,
		UpperRight: ur,
		M2Storage:  canvas.NewSegmentStore(ll, ur),
		M3Storage:  canvas.NewTrackSegmentStorage(m3TrackCount, m3TrackWidth),
	}
	r := router.NewTwoLayerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
	s := session.NewSession(c, r, nl, common.ViaConfig{}, common.ViaConfig{}, common.NoDRC{}, common.NoDRC{})

	results := s.Route()

	// results[0] = routed net; results[1] = pin segments (NetID=0)
	require.Len(t, results, 2)
	require.NoError(t, results[0].Err, "net2 should route without error")

	segs := results[0].Segments

	// Collect segments by layer.
	var m2Segs, m3Segs []common.Segment
	for _, seg := range segs {
		switch seg.Layer {
		case common.M2:
			m2Segs = append(m2Segs, seg)
		case common.M3:
			m3Segs = append(m3Segs, seg)
		default:
			// Via12, Via23 — not checked here
		}
	}
	require.Len(t, m2Segs, 2, "one M2 stub per pin")
	require.Len(t, m3Segs, 1, "one M3 track segment")

	// Find the two M2 stubs by their X position so the assertions are
	// independent of ordering.
	var m2ZN, m2I common.Segment
	for _, seg := range m2Segs {
		if seg.LowerLeft.X == 800 {
			m2ZN = seg // I0.ZN (MX-transformed)
		} else if seg.LowerLeft.X == 4800 {
			m2I = seg // I1.I (MY-transformed)
		}
	}

	// I0 has MX orientation: pin ZN Y coords are negated relative to cell origin.
	// Without MX the stub would sit at Y≥100; with MX it sits at Y≤0.
	assert.Equal(t, 800, m2ZN.LowerLeft.X, "I0.ZN XLow unaffected by MX")
	assert.Equal(t, -300, m2ZN.LowerLeft.Y, "I0.ZN MX flips pin YLow: -yHigh=-300")
	assert.Equal(t, 0, m2ZN.UpperRight.Y, "I0.ZN M2 reaches track top (Y=0)")

	// I1 has MY orientation: pin I X coords are negated relative to cell origin,
	// then offset by instX=5000. Without MY XLow would be 5000+100=5100.
	assert.Equal(t, 4800, m2I.LowerLeft.X, "I1.I MY flips X: 5000-200=4800")
	assert.Equal(t, 300, m2I.UpperRight.Y, "I1.I M2 extended to cover pin YHigh=300")

	// M3 spans from the leftmost pin X (800) to the rightmost pin X + m2Width (4801).
	assert.Equal(t, 800, m3Segs[0].LowerLeft.X, "M3 starts at I0.ZN XLow")
	assert.Equal(t, 4801, m3Segs[0].UpperRight.X, "M3 ends at I1.I XLow + m2Width")
}

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

	ll, ur, nl, err := netlist.BuildNets(
		"testdata/inv_layout.json",
		"testdata/inv_schematic.json",
		db,
		nil, nil,
	)
	require.NoError(t, err)
	require.Len(t, nl.Nets, 1)

	m3TrackCount := (ur.Y - ll.Y) / m3TrackWidth
	c := &canvas.Canvas{
		LowerLeft:  ll,
		UpperRight: ur,
		M2Storage:  canvas.NewSegmentStore(ll, ur),
		M3Storage:  canvas.NewTrackSegmentStorage(m3TrackCount, m3TrackWidth),
	}
	r := router.NewTwoLayerRouter(c, 1, common.NoDRC{}, common.NoDRC{})
	s := session.NewSession(c, r, nl, common.ViaConfig{}, common.ViaConfig{}, common.NoDRC{}, common.NoDRC{})

	results := s.Route()

	require.Len(t, results, 1)
	for i, res := range results {
		assert.NoError(t, res.Err, "net index %d should route without error", i)
	}
}
