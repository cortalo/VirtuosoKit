package netlist

import (
	"errors"
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

// TestBuildEscapeShapes_IsEscapeCellError_ReturnsError: IsEscapeCell failure → error propagated.
func TestBuildEscapeShapes_IsEscapeCellError_ReturnsError(t *testing.T) {
	db := &stubDB{
		escapeErr: errors.New("db unavailable"),
	}
	layout := escapeLayout([]LayoutInstance{
		{Name: "I0", Lib: "mylib", Cell: "mycell", XY: [2]float64{0, 0}, Orient: "R0"},
	})

	_, err := BuildEscapeShapes(layout, db, testContactVC)

	assert.Error(t, err)
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

// TestEscapePathShapes_Containment: PC X and Y already contain pin.
// vertPC covers original PC range; horizPC sits at pin Y spanning PC X.
// M1 is exactly pin.
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

	// 2 PC (vertPC + horizPC) + 1 M1 + contacts.
	assert.Equal(t, 2, countLayer(shapes, common.PC))
	assert.Equal(t, 1, countLayer(shapes, common.M1))
	assert.Greater(t, countLayer(shapes, common.Contact), 0)

	// M1 must be exactly the pin bbox.
	var m1 common.Shape
	for _, s := range shapes {
		if s.Layer == common.M1 {
			m1 = s
		}
	}
	assert.Equal(t, pin.LowerLeft, m1.LowerLeft)
	assert.Equal(t, pin.UpperRight, m1.UpperRight)
}

// TestEscapePathShapes_PCBelowPin: PC is below and to the left of pin.
// vertPC extends up to pin Y; horizPC covers pin X at pin Y. M1 is exactly m1Pin.
func TestEscapePathShapes_PCBelowPin(t *testing.T) {
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

	assert.Equal(t, 2, countLayer(shapes, common.PC)) // vertPC + horizPC
	assert.Equal(t, 1, countLayer(shapes, common.M1))
	assert.Greater(t, countLayer(shapes, common.Contact), 0)

	// vertPC: original PC X width, extended up to pin Y.
	// horizPC: at pin Y height, spanning from pcX0 to pinX1.
	var vertPC, horizPC common.Shape
	for _, s := range shapes {
		if s.Layer != common.PC {
			continue
		}
		if s.LowerLeft.X == 100 && s.UpperRight.X == 200 {
			vertPC = s
		} else {
			horizPC = s
		}
	}
	assert.Equal(t, common.Point{X: 100, Y: 0}, vertPC.LowerLeft)
	assert.Equal(t, common.Point{X: 200, Y: 500}, vertPC.UpperRight) // extended up to pinY1

	assert.Equal(t, common.Point{X: 100, Y: 300}, horizPC.LowerLeft) // pcX0..pinX1
	assert.Equal(t, common.Point{X: 600, Y: 500}, horizPC.UpperRight)

	// M1 is exactly the pin from cells.toml.
	var m1Shape common.Shape
	for _, s := range shapes {
		if s.Layer == common.M1 {
			m1Shape = s
		}
	}
	assert.Equal(t, pin.LowerLeft, m1Shape.LowerLeft)
	assert.Equal(t, pin.UpperRight, m1Shape.UpperRight)
}

// TestEscapePathShapes_PCAbovePin: PC is above and to the right of pin.
// vertPC extends down to pin Y; horizPC covers pin X at pin Y. M1 is exactly m1Pin.
func TestEscapePathShapes_PCAbovePin(t *testing.T) {
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

	assert.Equal(t, 2, countLayer(shapes, common.PC)) // vertPC + horizPC
	assert.Equal(t, 1, countLayer(shapes, common.M1))
	assert.Greater(t, countLayer(shapes, common.Contact), 0)

	// vertPC: original PC X width, extended down to pin Y.
	// horizPC: at pin Y height, spanning from pinX0 to pcX1.
	var vertPC, horizPC common.Shape
	for _, s := range shapes {
		if s.Layer != common.PC {
			continue
		}
		if s.LowerLeft.X == 100 && s.UpperRight.X == 200 {
			vertPC = s
		} else {
			horizPC = s
		}
	}
	assert.Equal(t, common.Point{X: 100, Y: 0}, vertPC.LowerLeft)    // extended down to pinY0
	assert.Equal(t, common.Point{X: 200, Y: 700}, vertPC.UpperRight)  // original pcY1 preserved

	assert.Equal(t, common.Point{X: 0, Y: 0}, horizPC.LowerLeft)     // pinX0..pcX1
	assert.Equal(t, common.Point{X: 200, Y: 200}, horizPC.UpperRight)

	// M1 is exactly the pin from cells.toml.
	var m1Shape common.Shape
	for _, s := range shapes {
		if s.Layer == common.M1 {
			m1Shape = s
		}
	}
	assert.Equal(t, pin.LowerLeft, m1Shape.LowerLeft)
	assert.Equal(t, pin.UpperRight, m1Shape.UpperRight)
}

// TestEscapePathShapes_YOverlap_XGap: PC and pin overlap in Y but not in X.
// vertPC is the original PC (Y already covers pin Y); horizPC covers pin X at pin Y.
// M1 is exactly m1Pin.
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

	assert.Equal(t, 2, countLayer(shapes, common.PC)) // vertPC + horizPC
	assert.Equal(t, 1, countLayer(shapes, common.M1))
	assert.Greater(t, countLayer(shapes, common.Contact), 0)

	var vertPC, horizPC common.Shape
	for _, s := range shapes {
		if s.Layer != common.PC {
			continue
		}
		if s.LowerLeft.X == 0 && s.UpperRight.X == 100 {
			vertPC = s
		} else {
			horizPC = s
		}
	}
	// vertPC: unchanged (Y already covered)
	assert.Equal(t, common.Point{X: 0, Y: 200}, vertPC.LowerLeft)
	assert.Equal(t, common.Point{X: 100, Y: 400}, vertPC.UpperRight)
	// horizPC: at pin Y, pcX0..pinX1
	assert.Equal(t, common.Point{X: 0, Y: 250}, horizPC.LowerLeft)
	assert.Equal(t, common.Point{X: 500, Y: 350}, horizPC.UpperRight)

	// M1 is exactly the pin from cells.toml.
	var m1Shape common.Shape
	for _, s := range shapes {
		if s.Layer == common.M1 {
			m1Shape = s
		}
	}
	assert.Equal(t, pin.LowerLeft, m1Shape.LowerLeft)
	assert.Equal(t, pin.UpperRight, m1Shape.UpperRight)
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
	assert.Equal(t, 1, countLayer(shapes, common.M1)) // horizSeg only — no M1 through PC

	// horizSeg must cover pin X range and sit at pin Y level.
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
