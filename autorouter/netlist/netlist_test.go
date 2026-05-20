package netlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
