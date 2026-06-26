package netlist

import (
	"fmt"
	"testing"

	"autorouter/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDB implements PinDB for testing.
type stubDB struct {
	pins map[string]stubPinData // key: "lib/cell/pin"
}

type stubPinData struct {
	xLow, xHigh, yLow, yHigh int
	layer                    common.Layer
}

func (db *stubDB) Query(lib, cell, pin string) (int, int, int, int, common.Layer, error) {
	if p, ok := db.pins[lib+"/"+cell+"/"+pin]; ok {
		return p.xLow, p.xHigh, p.yLow, p.yHigh, p.layer, nil
	}
	return 0, 0, 0, 0, 0, fmt.Errorf("stub: %w: %s/%s/%s", common.ErrPinNotFound, lib, cell, pin)
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

func TestExpandInstPin_BusPin(t *testing.T) {
	// Pin part carries the bus notation: single inst, bus pin → expand pin.
	assert.Equal(t,
		[]string{"Ipulse_filter.width<3>", "Ipulse_filter.width<2>", "Ipulse_filter.width<1>", "Ipulse_filter.width<0>"},
		expandInstPin("Ipulse_filter.width<3:0>"),
	)
}

func TestExpandNets_BusNetBusPin(t *testing.T) {
	// "FILTER_WIDTH<3:0>": ["Ipulse_filter.width<3:0>"] should expand 1:1 into 4 pairs.
	raw := map[string][]string{
		"FILTER_WIDTH<3:0>": {"Ipulse_filter.width<3:0>"},
	}
	got, err := expandNets(raw)
	require.NoError(t, err)
	assert.Equal(t, []string{"Ipulse_filter.width<3>"}, got["FILTER_WIDTH<3>"])
	assert.Equal(t, []string{"Ipulse_filter.width<2>"}, got["FILTER_WIDTH<2>"])
	assert.Equal(t, []string{"Ipulse_filter.width<1>"}, got["FILTER_WIDTH<1>"])
	assert.Equal(t, []string{"Ipulse_filter.width<0>"}, got["FILTER_WIDTH<0>"])
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
	for _, name := range []string{"I0<2>", "I0<1>", "I0<0>", "I5"} {
		inst, ok := got[name]
		require.True(t, ok, "missing instance %q", name)
		assert.Equal(t, name, inst.Name)
		assert.Equal(t, "mylib", inst.Lib)
	}
}

func TestParseOrient(t *testing.T) {
	assert.Equal(t, "R0", parseOrient(`"R0"`))
	assert.Equal(t, "MX", parseOrient(`"MX"`))
	assert.Equal(t, "MY", parseOrient(`"MY"`))
	assert.Equal(t, "R180", parseOrient(`"R180"`))
	assert.Equal(t, "R0", parseOrient("R0"))
}

// twoInstLayout builds a minimal RawLayout with a prBoundary and two instances of the same cell.
func twoInstLayout(lib, cell string, i0xy, i1xy [2]float64, i0terms, i1terms map[string]TerminalInfo) RawLayout {
	return RawLayout{
		Shapes: []LayoutShape{{Layer: "prBoundary", BBox: [2][2]float64{{0, 0}, {10, 10}}}},
		Instances: []LayoutInstance{
			{Name: "I0", Lib: lib, Cell: cell, XY: i0xy, Orient: "R0", Terminals: i0terms},
			{Name: "I1", Lib: lib, Cell: cell, XY: i1xy, Orient: "R0", Terminals: i1terms},
		},
	}
}

func twoInstSchem(lib string, pinName string) RawSchematic {
	return RawSchematic{
		Instances: []SchematicInstance{{Name: "I0", Lib: lib}, {Name: "I1", Lib: lib}},
		Nets:      map[string][]string{"net1": {"I0." + pinName, "I1." + pinName}},
	}
}

// TestBuildNets_NormalPin_TransformAndOffsetApplied: pin found in db;
// instance XY offset is added to the transformed cell-relative bbox.
func TestBuildNets_NormalPin_TransformAndOffsetApplied(t *testing.T) {
	db := &stubDB{
		pins: map[string]stubPinData{"mylib/mycell/A": {10, 20, 30, 40, common.M1}},
	}
	layout := twoInstLayout("mylib", "mycell", [2]float64{1.0, 2.0}, [2]float64{3.0, 4.0}, nil, nil)
	schem := twoInstSchem("mylib", "A")

	nl, err := BuildNetsFromData(layout, schem, db, nil, nil, nil, nil, nil, false)

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

func TestSchematicFilter_IncludeNets_OnlyKeepsListedNets(t *testing.T) {
	s := Schematic{
		Nets: map[string][]string{
			"net1": {"I0.A", "I1.B"},
			"net2": {"I1.C", "I2.D"},
			"VDD":  {"I0.VDD", "I1.VDD"},
		},
	}
	got := s.Filter([]string{"net1"}, nil, nil, nil)
	assert.Len(t, got.Nets, 1)
	assert.Contains(t, got.Nets, "net1")
}

func TestSchematicFilter_IncludeNets_IgnoreNetsHasNoEffect(t *testing.T) {
	// When includeNets is set, ignoreNets is completely bypassed —
	// even if a net appears in both lists, it is kept.
	s := Schematic{
		Nets: map[string][]string{
			"net1": {"I0.A", "I1.B"},
			"net2": {"I1.C", "I2.D"},
		},
	}
	got := s.Filter([]string{"net1"}, []string{"net1"}, nil, nil)
	assert.Len(t, got.Nets, 1)
	assert.Contains(t, got.Nets, "net1")
}

// TestBuildNets_PinNotFound_ReturnsError: pin absent from db → error.
func TestBuildNets_PinNotFound_ReturnsError(t *testing.T) {
	db := &stubDB{pins: map[string]stubPinData{}}
	layout := twoInstLayout("mylib", "mycell", [2]float64{0, 0}, [2]float64{1, 0}, nil, nil)
	schem := twoInstSchem("mylib", "A")

	_, err := BuildNetsFromData(layout, schem, db, nil, nil, nil, nil, nil, false)

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

// TestBuildNetsFromData_TerminalFallback exercises the terminal-fallback path
// (empty celldb → all pin positions come from inst.Terminals).
//
// Layout has two inv instances; schematic has (after ignoring VDD/VSS):
//   - net1: I1.VOUT, I0.VIN  (2-pin internal net)
//   - VIN:  I1.VIN            (1-pin port net)
//   - VOUT: I0.VOUT           (1-pin port net)
func TestBuildNetsFromData_TerminalFallback(t *testing.T) {
	layout, schem, err := LoadFiles(
		"testdata/inv2_terminal_layout.json",
		"testdata/inv2_terminal_schematic.json",
	)
	require.NoError(t, err)

	db := &stubDB{pins: map[string]stubPinData{}} // empty → terminal fallback for all

	nl, err := BuildNetsFromData(layout, schem, db, nil, []string{"VDD", "VSS"}, nil, nil, nil, true)
	require.NoError(t, err)

	// ── layout pins (top-level ports) ────────────────────────────────────────
	wantPorts := map[string]bool{"VIN": false, "VOUT": false}
	assert.Len(t, nl.Pins, len(wantPorts), "unexpected number of layout pins")
	for _, p := range nl.Pins {
		assert.NotEmpty(t, p.Name, "layout pin missing Name")
		assert.True(t, p.XLow < p.XHigh, "layout pin %q: XLow >= XHigh", p.Name)
		assert.True(t, p.YLow < p.YHigh, "layout pin %q: YLow >= YHigh", p.Name)
		assert.NotZero(t, p.Layer, "layout pin %q: zero Layer", p.Name)
		wantPorts[p.Name] = true
	}
	for name, seen := range wantPorts {
		assert.True(t, seen, "expected layout pin %q not found", name)
	}

	// ── routing nets ─────────────────────────────────────────────────────────
	type wantPinData struct {
		name                     string
		layer                    common.Layer
		xLow, xHigh, yLow, yHigh int
	}
	type wantNetData struct {
		pins []wantPinData
	}
	// Coordinates in nm (µm × 1000), from testdata terminal bboxes.
	wantNets := map[string]wantNetData{
		"net1": {
			pins: []wantPinData{
				{"I1.VOUT", common.M1, 4000, 4230, 6220, 8140},
				{"I0.VIN", common.M1, 8090, 8320, 4365, 4595},
			},
		},
		"VIN": {
			pins: []wantPinData{
				{"I1.VIN", common.M1, 2090, 2320, 4365, 4595},
			},
		},
		"VOUT": {
			pins: []wantPinData{
				{"I0.VOUT", common.M1, 10000, 10230, 6220, 8140},
			},
		},
	}
	assert.Len(t, nl.Nets, len(wantNets), "unexpected number of nets")

	byNetName := make(map[string]*common.Net, len(nl.Nets))
	for _, net := range nl.Nets {
		byNetName[net.Name] = net
	}

	for netName, want := range wantNets {
		t.Run(netName, func(t *testing.T) {
			net, ok := byNetName[netName]
			require.Truef(t, ok, "net %q missing", netName)

			require.Len(t, net.Pins, len(want.pins))
			byPin := make(map[string]common.RoutingPin, len(net.Pins))
			for _, p := range net.Pins {
				byPin[p.Name] = p
			}
			for _, wp := range want.pins {
				p, ok := byPin[wp.name]
				require.Truef(t, ok, "pin %q missing", wp.name)
				assert.Equal(t, wp.layer, p.Layer, "layer")
				assert.Equal(t, wp.xLow, p.XLow, "XLow")
				assert.Equal(t, wp.xHigh, p.XHigh, "XHigh")
				assert.Equal(t, wp.yLow, p.YLow, "YLow")
				assert.Equal(t, wp.yHigh, p.YHigh, "YHigh")
			}
		})
	}
}
