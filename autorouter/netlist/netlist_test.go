package netlist

import (
	"errors"
	"testing"

	"autorouter/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDB implements PinDB for testing.
type stubDB struct {
	pins      map[string]stubPinData // key: "lib/cell/pin"
	escape    map[string]bool        // key: "lib/cell"
	escapeErr error                  // returned by IsEscapeCell when non-nil
}

type stubPinData struct {
	xLow, xHigh, yLow, yHigh int
	layer                    common.Layer
}

func (db *stubDB) Query(lib, cell, pin string) (int, int, int, int, common.Layer, error) {
	if p, ok := db.pins[lib+"/"+cell+"/"+pin]; ok {
		return p.xLow, p.xHigh, p.yLow, p.yHigh, p.layer, nil
	}
	return 0, 0, 0, 0, 0, errors.New("stub: pin not found")
}

func (db *stubDB) IsEscapeCell(lib, cell string) (bool, error) {
	if db.escapeErr != nil {
		return false, db.escapeErr
	}
	return db.escape[lib+"/"+cell], nil
}

func TestExpandBusName_Scalar(t *testing.T) {
	assert.Equal(t, []string{"VSS"}, expandBusName("VSS"))
}

func TestExpandBusName_Range(t *testing.T) {
	assert.Equal(t, []string{"PH<2>", "PH<1>", "PH<0>"}, expandBusName("PH<2:0>"))
}

func TestExpandBusName_SingleBit(t *testing.T) {
	assert.Equal(t, []string{"SEL<3>"}, expandBusName("SEL<3:3>"))
}

func TestExpandNetKey_Scalar(t *testing.T) {
	assert.Equal(t, []string{"VDD"}, expandNetKey("VDD"))
}

func TestExpandNetKey_BusRange(t *testing.T) {
	assert.Equal(t, []string{"A<2>", "A<1>", "A<0>"}, expandNetKey("A<2:0>"))
}

func TestExpandNetKey_Repeat(t *testing.T) {
	assert.Equal(t, []string{"VSS", "VSS", "VSS"}, expandNetKey("<*3>VSS"))
}

func TestExpandNetKey_CommaList(t *testing.T) {
	assert.Equal(t, []string{"A<2>", "A<1>", "A<0>", "VSS"}, expandNetKey("A<2:0>,VSS"))
}

func TestExpandInstPin_Scalar(t *testing.T) {
	assert.Equal(t, []string{"I5.VDD"}, expandInstPin("I5.VDD"))
}

func TestExpandInstPin_BusInst(t *testing.T) {
	assert.Equal(t, []string{"I0<2>.A1", "I0<1>.A1", "I0<0>.A1"}, expandInstPin("I0<2:0>.A1"))
}

func TestExpandNets_Simple(t *testing.T) {
	raw := map[string][]string{
		"VSS":     {"I5.VSS", "I6.VSS"}, // scalar → both pins on same net
		"A<1:0>":  {"I0<1:0>.Z"},        // bus 1:1 pairing
		"<*2>VDD": {"I0<1:0>.VDD"},      // repeat → scalar, both pins on VDD
	}
	got, err := expandNets(raw)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"I5.VSS", "I6.VSS"}, got["VSS"])
	assert.Equal(t, []string{"I0<1>.Z"}, got["A<1>"])
	assert.Equal(t, []string{"I0<0>.Z"}, got["A<0>"])
	assert.ElementsMatch(t, []string{"I0<1>.VDD", "I0<0>.VDD"}, got["VDD"])
}

func TestExpandNets_MismatchError(t *testing.T) {
	raw := map[string][]string{
		"A<2:0>": {"I0<1:0>.Z"}, // 3 net names vs 2 inst.pins
	}
	_, err := expandNets(raw)
	assert.Error(t, err)
}

func TestExpandNets_BusKeyNoInstPins(t *testing.T) {
	raw := map[string][]string{
		"s<1:0>": {}, // bus net with no connected instance pins
	}
	got, err := expandNets(raw)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestExpandSchematicInstances(t *testing.T) {
	insts := []SchematicInstance{
		{Name: "I0<2:0>", Lib: "mylib"},
		{Name: "I5", Lib: "mylib"},
	}
	got := expandSchematicInstances(insts)
	require.Len(t, got, 4)
	assert.Equal(t, "I0<2>", got[0].Name)
	assert.Equal(t, "I0<1>", got[1].Name)
	assert.Equal(t, "I0<0>", got[2].Name)
	assert.Equal(t, "I5", got[3].Name)
	for _, g := range got {
		assert.Equal(t, "mylib", g.Lib)
	}
}

func TestParseOrient(t *testing.T) {
	assert.Equal(t, "R0", parseOrient(`"R0"`))
	assert.Equal(t, "MX", parseOrient(`"MX"`))
	assert.Equal(t, "MY", parseOrient(`"MY"`))
	assert.Equal(t, "R180", parseOrient(`"R180"`))
	assert.Equal(t, "R0", parseOrient("R0"))
}

// twoInstLayout builds a minimal Layout with a prBoundary and two instances of the same cell.
func twoInstLayout(lib, cell string, i0xy, i1xy [2]float64, i0terms, i1terms map[string]TerminalInfo) Layout {
	return Layout{
		Shapes: []LayoutShape{{Layer: "prBoundary", BBox: [2][2]float64{{0, 0}, {10, 10}}}},
		Instances: []LayoutInstance{
			{Name: "I0", Lib: lib, Cell: cell, XY: i0xy, Orient: "R0", Terminals: i0terms},
			{Name: "I1", Lib: lib, Cell: cell, XY: i1xy, Orient: "R0", Terminals: i1terms},
		},
	}
}

func twoInstSchem(lib string, pinName string) Schematic {
	return Schematic{
		Instances: []SchematicInstance{{Name: "I0", Lib: lib}, {Name: "I1", Lib: lib}},
		Nets:      map[string][]string{"net1": {"I0." + pinName, "I1." + pinName}},
	}
}

// TestBuildNets_NormalPin_TransformAndOffsetApplied: non-escape cell, pin found in db;
// instance XY offset is added to the transformed cell-relative bbox.
func TestBuildNets_NormalPin_TransformAndOffsetApplied(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{"mylib/mycell/A": {10, 20, 30, 40, common.M1}},
		escape: map[string]bool{},
	}
	layout := twoInstLayout("mylib", "mycell", [2]float64{1.0, 2.0}, [2]float64{3.0, 4.0}, nil, nil)
	schem := twoInstSchem("mylib", "A")

	_, _, nl, err := BuildNetsFromData(layout, schem, db, nil, nil, nil, nil)

	require.NoError(t, err)
	require.Len(t, nl.Nets, 1)
	pins := nl.Nets[0].Pins
	require.Len(t, pins, 2)
	// I0 at (1000, 2000) nm + pin (10,20,30,40) R0 → no transform → (1010, 1020, 2030, 2040)
	assert.Equal(t, 1010, pins[0].XLow)
	assert.Equal(t, 1020, pins[0].XHigh)
	assert.Equal(t, 2030, pins[0].YLow)
	assert.Equal(t, 2040, pins[0].YHigh)
	assert.Equal(t, common.M1, pins[0].Layer)
	// I1 at (3000, 4000) nm
	assert.Equal(t, 3010, pins[1].XLow)
	assert.Equal(t, 4030, pins[1].YLow)
}

// TestBuildNets_EscapeCell_PinInDB_UsesTransformAndOffset: escape=true but pin IS in db;
// should still use transform+offset, not the terminal coords.
func TestBuildNets_EscapeCell_PinInDB_UsesTransformAndOffset(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{"mylib/mycell/A": {10, 20, 30, 40, common.M1}},
		escape: map[string]bool{"mylib/mycell": true},
	}
	layout := twoInstLayout("mylib", "mycell", [2]float64{1.0, 0.0}, [2]float64{2.0, 0.0}, nil, nil)
	schem := twoInstSchem("mylib", "A")

	_, _, nl, err := BuildNetsFromData(layout, schem, db, nil, nil, nil, nil)

	require.NoError(t, err)
	require.Len(t, nl.Nets, 1)
	pins := nl.Nets[0].Pins
	require.Len(t, pins, 2)
	assert.Equal(t, 1010, pins[0].XLow) // 1000+10, not terminal
	assert.Equal(t, 2010, pins[1].XLow) // 2000+10
}

// TestBuildNets_EscapeCell_PinFromTerminal: escape=true, pin absent from db;
// absolute bbox from terminal used directly (no XY offset, no transform).
func TestBuildNets_EscapeCell_PinFromTerminal(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{},
		escape: map[string]bool{"mylib/mycell": true},
	}
	i0terms := map[string]TerminalInfo{"G": {Layer: "M1", Bbox: [2][2]float64{{0.1, 0.2}, {0.3, 0.4}}}}
	i1terms := map[string]TerminalInfo{"G": {Layer: "M1", Bbox: [2][2]float64{{0.5, 0.6}, {0.7, 0.8}}}}
	layout := twoInstLayout("mylib", "mycell", [2]float64{99.0, 99.0}, [2]float64{99.0, 99.0}, i0terms, i1terms)
	schem := twoInstSchem("mylib", "G")

	_, _, nl, err := BuildNetsFromData(layout, schem, db, nil, nil, nil, nil)

	require.NoError(t, err)
	require.Len(t, nl.Nets, 1)
	pins := nl.Nets[0].Pins
	require.Len(t, pins, 2)
	// Absolute coords from terminal — XY offset (99000, 99000) must NOT be added.
	assert.Equal(t, 100, pins[0].XLow)
	assert.Equal(t, 300, pins[0].XHigh)
	assert.Equal(t, 200, pins[0].YLow)
	assert.Equal(t, 400, pins[0].YHigh)
	assert.Equal(t, common.M1, pins[0].Layer)
	assert.Equal(t, 500, pins[1].XLow)
	assert.Equal(t, 600, pins[1].YLow)
}

// TestBuildNets_EscapeCell_MissingTerminal_ReturnsError: escape=true, pin absent from both
// db and terminals map → error.
func TestBuildNets_EscapeCell_MissingTerminal_ReturnsError(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{},
		escape: map[string]bool{"mylib/mycell": true},
	}
	layout := twoInstLayout("mylib", "mycell", [2]float64{0, 0}, [2]float64{1, 0}, nil, nil)
	schem := twoInstSchem("mylib", "G")

	_, _, _, err := BuildNetsFromData(layout, schem, db, nil, nil, nil, nil)

	assert.Error(t, err)
}

// TestBuildNets_NonEscapeCell_PinNotFound_ReturnsError: non-escape cell, pin absent → error.
func TestBuildNets_NonEscapeCell_PinNotFound_ReturnsError(t *testing.T) {
	db := &stubDB{
		pins:   map[string]stubPinData{},
		escape: map[string]bool{},
	}
	layout := twoInstLayout("mylib", "mycell", [2]float64{0, 0}, [2]float64{1, 0}, nil, nil)
	schem := twoInstSchem("mylib", "A")

	_, _, _, err := BuildNetsFromData(layout, schem, db, nil, nil, nil, nil)

	assert.Error(t, err)
}

func TestTransformPin(t *testing.T) {
	xL, xH, yL, yH := 100, 200, 300, 400

	t.Run("R0", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "R0")
		assert.Equal(t, 100, xl)
		assert.Equal(t, 200, xh)
		assert.Equal(t, 300, yl)
		assert.Equal(t, 400, yh)
	})
	t.Run("MX", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "MX")
		assert.Equal(t, 100, xl)
		assert.Equal(t, 200, xh)
		assert.Equal(t, -400, yl)
		assert.Equal(t, -300, yh)
	})
	t.Run("MY", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "MY")
		assert.Equal(t, -200, xl)
		assert.Equal(t, -100, xh)
		assert.Equal(t, 300, yl)
		assert.Equal(t, 400, yh)
	})
	t.Run("R180", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "R180")
		assert.Equal(t, -200, xl)
		assert.Equal(t, -100, xh)
		assert.Equal(t, -400, yl)
		assert.Equal(t, -300, yh)
	})
	t.Run("unknown acts as R0", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "XYZ")
		assert.Equal(t, xL, xl)
		assert.Equal(t, xH, xh)
		assert.Equal(t, yL, yl)
		assert.Equal(t, yH, yh)
	})
}
