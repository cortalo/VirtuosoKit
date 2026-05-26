package netlist

import (
	"testing"

	"autorouter/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countLayer counts shapes on the given layer.
func countLayer(shapes []common.Shape, layer common.Layer) int {
	n := 0
	for _, s := range shapes {
		if s.Layer == layer {
			n++
		}
	}
	return n
}

var testContactVC = common.ViaConfig{CutW: 80, CutH: 80, SpaceX: 20, SpaceY: 20}

func escapeLayout(instances []LayoutInstance) Layout {
	return Layout{Instances: instances}
}

// ── BuildEscapeShapes integration tests ──────────────────────────────────────

// TestBuildEscapeShapes_NonEscapeCell_NoShapes: cell not marked escape → nothing generated.
func TestBuildEscapeShapes_NonEscapeCell_NoShapes(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{"mylib/mycell/G": {0, 200, 0, 200, common.M1}},
		escape: map[string]bool{},
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "mylib", Cell: "mycell", XY: [2]float64{0, 0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"G": {Layer: "PC", Bbox: [2][2]float64{{0, 0}, {0.2, 0.2}}}}},
	})

	shapes, err := BuildEscapeShapes(layout, db, testContactVC)

	require.NoError(t, err)
	assert.Empty(t, shapes)
}

// TestBuildEscapeShapes_PCTerminal_GeneratesM1AndContacts: PC terminal contained
// within pin bbox → one M1 shape and at least one Contact cut.
func TestBuildEscapeShapes_PCTerminal_GeneratesM1AndContacts(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{"mylib/mycell/G": {0, 200, 0, 200, common.M1}},
		escape: map[string]bool{"mylib/mycell": true},
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "mylib", Cell: "mycell", XY: [2]float64{0, 0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"G": {Layer: "PC", Bbox: [2][2]float64{{0, 0}, {0.2, 0.2}}}}},
	})

	shapes, err := BuildEscapeShapes(layout, db, testContactVC)

	require.NoError(t, err)
	assert.Equal(t, 1, countLayer(shapes, common.M1))
	assert.Greater(t, countLayer(shapes, common.Contact), 0)
}

// TestBuildEscapeShapes_M1Terminal_GeneratesM1Only: non-PC terminal → M1 shape, no contacts.
func TestBuildEscapeShapes_M1Terminal_GeneratesM1Only(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{"mylib/mycell/S": {0, 200, 0, 200, common.M1}},
		escape: map[string]bool{"mylib/mycell": true},
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "mylib", Cell: "mycell", XY: [2]float64{0, 0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"S": {Layer: "M1", Bbox: [2][2]float64{{0, 0}, {0.2, 0.2}}}}},
	})

	shapes, err := BuildEscapeShapes(layout, db, testContactVC)

	require.NoError(t, err)
	assert.Equal(t, 1, countLayer(shapes, common.M1))
	assert.Equal(t, 0, countLayer(shapes, common.Contact))
}

// TestBuildEscapeShapes_PinNotInDB_Skipped: escape cell but pin absent from cells.toml
// → pin silently skipped, no shapes.
func TestBuildEscapeShapes_PinNotInDB_Skipped(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{},
		escape: map[string]bool{"mylib/mycell": true},
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "mylib", Cell: "mycell", XY: [2]float64{0, 0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"G": {Layer: "PC", Bbox: [2][2]float64{{0, 0}, {0.2, 0.2}}}}},
	})

	shapes, err := BuildEscapeShapes(layout, db, testContactVC)

	require.NoError(t, err)
	assert.Empty(t, shapes)
}

// TestBuildEscapeShapes_BadTerminalLayer_ReturnsError: unrecognised terminal layer → error.
func TestBuildEscapeShapes_BadTerminalLayer_ReturnsError(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{"mylib/mycell/G": {0, 200, 0, 200, common.M1}},
		escape: map[string]bool{"mylib/mycell": true},
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "mylib", Cell: "mycell", XY: [2]float64{0, 0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"G": {Layer: "BADLAYER", Bbox: [2][2]float64{{0, 0}, {0.2, 0.2}}}}},
	})

	_, err := BuildEscapeShapes(layout, db, testContactVC)

	assert.Error(t, err)
}

// TestBuildEscapeShapes_ZeroContactVC_NoContacts: contactVC with zero dimensions → no cuts.
func TestBuildEscapeShapes_ZeroContactVC_NoContacts(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{"mylib/mycell/G": {0, 200, 0, 200, common.M1}},
		escape: map[string]bool{"mylib/mycell": true},
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "mylib", Cell: "mycell", XY: [2]float64{0, 0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"G": {Layer: "PC", Bbox: [2][2]float64{{0, 0}, {0.2, 0.2}}}}},
	})

	shapes, err := BuildEscapeShapes(layout, db, common.ViaConfig{})

	require.NoError(t, err)
	assert.Equal(t, 1, countLayer(shapes, common.M1))
	assert.Equal(t, 0, countLayer(shapes, common.Contact))
}

// TestBuildEscapeShapes_MultipleInstances_OnlyEscapeProducesShapes: non-escape instances
// are skipped even when they have terminals.
func TestBuildEscapeShapes_MultipleInstances_OnlyEscapeProducesShapes(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{"esclib/esccell/G": {0, 200, 0, 200, common.M1}},
		escape: map[string]bool{"esclib/esccell": true},
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "esclib", Cell: "esccell", XY: [2]float64{0, 0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"G": {Layer: "PC", Bbox: [2][2]float64{{0, 0}, {0.2, 0.2}}}}},
		{Name: "I1", Lib: "otherlib", Cell: "othercell", XY: [2]float64{1, 0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"A": {Layer: "M1", Bbox: [2][2]float64{{0, 0}, {0.2, 0.2}}}}},
	})

	shapes, err := BuildEscapeShapes(layout, db, common.ViaConfig{})

	require.NoError(t, err)
	assert.Equal(t, 1, countLayer(shapes, common.M1)) // only I0 contributes
}

// ── escapePathShapes unit tests ───────────────────────────────────────────────

// TestEscapePathShapes_Containment: PC completely contains pin → 1 PC + 1 M1 + contacts.
func TestEscapePathShapes_Containment(t *testing.T) {
	pc := common.Shape{
		Layer:      common.PC,
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 400, Y: 400},
	}
	pin := common.Shape{
		Layer:      common.M1,
		LowerLeft:  common.Point{X: 100, Y: 100},
		UpperRight: common.Point{X: 300, Y: 300},
	}

	shapes := escapePathShapes(pc, pin, testContactVC)

	assert.Equal(t, 1, countLayer(shapes, common.PC))
	assert.Equal(t, 1, countLayer(shapes, common.M1))
	assert.Greater(t, countLayer(shapes, common.Contact), 0)
	// The single M1 shape must be exactly the pin bbox.
	var m1 common.Shape
	for _, s := range shapes {
		if s.Layer == common.M1 {
			m1 = s
		}
	}
	assert.Equal(t, pin.LowerLeft, m1.LowerLeft)
	assert.Equal(t, pin.UpperRight, m1.UpperRight)
}

// TestEscapePathShapes_PCBelowPin_LShape: PC is below pin (Y gap) and to the left in X.
// Expects vertSeg (pcX width, spanning pcY0..pinY1) + horizSeg (pinY height, pcX0..pinX1).
func TestEscapePathShapes_PCBelowPin_LShape(t *testing.T) {
	// PC: X=100..200, Y=0..100  (below and to the left)
	// pin: X=400..600, Y=300..500
	pc := common.Shape{
		Layer:      common.PC,
		LowerLeft:  common.Point{X: 100, Y: 0},
		UpperRight: common.Point{X: 200, Y: 100},
	}
	pin := common.Shape{
		Layer:      common.M1,
		LowerLeft:  common.Point{X: 400, Y: 300},
		UpperRight: common.Point{X: 600, Y: 500},
	}

	shapes := escapePathShapes(pc, pin, testContactVC)

	assert.Equal(t, 1, countLayer(shapes, common.PC))
	assert.Equal(t, 2, countLayer(shapes, common.M1)) // vertSeg + horizSeg
	assert.Greater(t, countLayer(shapes, common.Contact), 0)

	var vert, horiz common.Shape
	for _, s := range shapes {
		if s.Layer != common.M1 {
			continue
		}
		if s.LowerLeft.X == 100 && s.UpperRight.X == 200 {
			vert = s
		} else {
			horiz = s
		}
	}
	// vertSeg: PC's X width, from pcY0=0 up to pinY1=500
	assert.Equal(t, common.Point{X: 100, Y: 0}, vert.LowerLeft)
	assert.Equal(t, common.Point{X: 200, Y: 500}, vert.UpperRight)
	// horizSeg: pin's Y height, from pcX0=100 across to pinX1=600
	assert.Equal(t, common.Point{X: 100, Y: 300}, horiz.LowerLeft)
	assert.Equal(t, common.Point{X: 600, Y: 500}, horiz.UpperRight)
}

// TestEscapePathShapes_PCAbovePin_LShape: PC is above pin (Y gap).
func TestEscapePathShapes_PCAbovePin_LShape(t *testing.T) {
	// PC: X=100..200, Y=600..700  (above and to the right)
	// pin: X=0..150, Y=0..200
	pc := common.Shape{
		Layer:      common.PC,
		LowerLeft:  common.Point{X: 100, Y: 600},
		UpperRight: common.Point{X: 200, Y: 700},
	}
	pin := common.Shape{
		Layer:      common.M1,
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: 150, Y: 200},
	}

	shapes := escapePathShapes(pc, pin, testContactVC)

	assert.Equal(t, 1, countLayer(shapes, common.PC))
	assert.Equal(t, 2, countLayer(shapes, common.M1))
	assert.Greater(t, countLayer(shapes, common.Contact), 0)

	var vert, horiz common.Shape
	for _, s := range shapes {
		if s.Layer != common.M1 {
			continue
		}
		if s.LowerLeft.X == 100 && s.UpperRight.X == 200 {
			vert = s
		} else {
			horiz = s
		}
	}
	// vertSeg: PC's X width, from pinY0=0 down to pcY1=700
	assert.Equal(t, common.Point{X: 100, Y: 0}, vert.LowerLeft)
	assert.Equal(t, common.Point{X: 200, Y: 700}, vert.UpperRight)
	// horizSeg: pin's Y height, from pinX0=0 to pcX1=200
	assert.Equal(t, common.Point{X: 0, Y: 0}, horiz.LowerLeft)
	assert.Equal(t, common.Point{X: 200, Y: 200}, horiz.UpperRight)
}

// TestEscapePathShapes_YOverlap_XGap: PC and pin overlap in Y but not in X.
// No Y-gap vertical travel needed; still emits vertSeg (PC column) + horizSeg.
func TestEscapePathShapes_YOverlap_XGap(t *testing.T) {
	// PC: X=0..100, Y=200..400
	// pin: X=300..500, Y=250..350  (Y overlaps, X gap)
	pc := common.Shape{
		Layer:      common.PC,
		LowerLeft:  common.Point{X: 0, Y: 200},
		UpperRight: common.Point{X: 100, Y: 400},
	}
	pin := common.Shape{
		Layer:      common.M1,
		LowerLeft:  common.Point{X: 300, Y: 250},
		UpperRight: common.Point{X: 500, Y: 350},
	}

	shapes := escapePathShapes(pc, pin, testContactVC)

	assert.Equal(t, 1, countLayer(shapes, common.PC))
	assert.Equal(t, 2, countLayer(shapes, common.M1))
	assert.Greater(t, countLayer(shapes, common.Contact), 0)

	var horiz common.Shape
	for _, s := range shapes {
		if s.Layer == common.M1 && s.LowerLeft.Y == 250 {
			horiz = s
		}
	}
	// horizSeg must span from pcX0=0 to pinX1=500 at pin's Y height
	assert.Equal(t, common.Point{X: 0, Y: 250}, horiz.LowerLeft)
	assert.Equal(t, common.Point{X: 500, Y: 350}, horiz.UpperRight)
}

// TestBuildEscapeShapes_LShape_TransformAndOffset: instance XY offset and orient
// are applied to the cells.toml pin before building the L-path.
func TestBuildEscapeShapes_LShape_TransformAndOffset(t *testing.T) {
	// pin (from cells.toml): X=300..500, Y=600..800 (nm), offset by instXY=(1,2)µm
	// after offset: X=1300..1500, Y=2600..2800
	// PC terminal (absolute): X=0.1..0.2 µm → 100..200 nm, Y=0.0..0.1 µm → 0..100 nm
	// PC is far below pin → L-shape expected
	db := &stubDB{
		pins:   map[string]stubPinData{"mylib/mycell/G": {300, 500, 600, 800, common.M1}},
		escape: map[string]bool{"mylib/mycell": true},
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "mylib", Cell: "mycell", XY: [2]float64{1.0, 2.0}, Orient: "R0",
			Terminals: map[string]TerminalInfo{"G": {Layer: "PC", Bbox: [2][2]float64{{0.1, 0.0}, {0.2, 0.1}}}}},
	})

	shapes, err := BuildEscapeShapes(layout, db, common.ViaConfig{})

	require.NoError(t, err)
	assert.Equal(t, 2, countLayer(shapes, common.M1)) // vertSeg + horizSeg

	// horizSeg must reach pinX1=1500 and cover pinY0..pinY1=2600..2800
	var horiz common.Shape
	for _, s := range shapes {
		if s.Layer == common.M1 && s.LowerLeft.Y == 2600 {
			horiz = s
		}
	}
	require.NotZero(t, horiz.UpperRight.X, "horizSeg not found")
	assert.Equal(t, 1500, horiz.UpperRight.X)
	assert.Equal(t, 2800, horiz.UpperRight.Y)
}
