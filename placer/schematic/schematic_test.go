package schematic

import (
	"placer/common"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_NoIgnore_ReturnsAll(t *testing.T) {
	instances, err := Load("testdata/test.json", nil)
	require.NoError(t, err)
	assert.Len(t, instances, 11)
}

func TestLoad_IgnoreBasic_FiltersNoConn(t *testing.T) {
	instances, err := Load("testdata/test.json", []string{"basic"})
	require.NoError(t, err)
	assert.Len(t, instances, 9)
	for _, inst := range instances {
		assert.NotEqual(t, "basic", inst.Lib)
	}
}

func TestLoad_IgnoreMultipleLibs(t *testing.T) {
	instances, err := Load("testdata/test.json", []string{"basic", "tcb018gbwp7t"})
	require.NoError(t, err)
	assert.Empty(t, instances)
}

func TestLoad_XYParsed(t *testing.T) {
	instances, err := Load("testdata/test.json", []string{"basic"})
	require.NoError(t, err)

	// I11: xy=[-0.59375, 0.125]
	var i11 *struct{ X, Y float64 }
	for _, inst := range instances {
		if inst.Name == "I11" {
			i11 = &struct{ X, Y float64 }{inst.X, inst.Y}
			break
		}
	}
	require.NotNil(t, i11)
	assert.InDelta(t, -0.59375, i11.X, 1e-9)
	assert.InDelta(t, 0.125, i11.Y, 1e-9)
}

func TestLoad_BusInstances_Expanded(t *testing.T) {
	instances, err := Load("testdata/instance_bus.json", nil)
	require.NoError(t, err)
	// I0<14:0>=15, I1<3:0>=4, I5=1, I6=1
	require.Len(t, instances, 21)

	byName := make(map[string]common.SchematicInstance, len(instances))
	for _, inst := range instances {
		byName[inst.Name] = inst
	}

	inst0 := byName["I0<14>"]
	assert.Equal(t, "tcb018gbwp7t", inst0.Lib)
	assert.Equal(t, "AN2D1BWP7T", inst0.Cell)
	assert.InDelta(t, -0.5625, inst0.X, 1e-9)

	_, ok0 := byName["I0<0>"]
	assert.True(t, ok0, "I0<0> should exist")
	_, ok14 := byName["I0<14>"]
	assert.True(t, ok14, "I0<14> should exist")
	_, okBus := byName["I0<14:0>"]
	assert.False(t, okBus, "unexpanded bus name should not exist")

	inst1 := byName["I1<3>"]
	assert.Equal(t, "OR4D1BWP7T", inst1.Cell)
}

func TestLoad_InvalidPath_ReturnsError(t *testing.T) {
	_, err := Load("nonexistent.json", nil)
	assert.Error(t, err)
}

func TestParse_OrientParsed(t *testing.T) {
	const input = `{"instances":[
		{"name":"I0","lib":"lib","cell":"A","xy":[0,0],"orient":"MY"},
		{"name":"I1","lib":"lib","cell":"A","xy":[0,0],"orient":"\"R180\""},
		{"name":"I2","lib":"lib","cell":"A","xy":[0,0]}
	]}`
	insts, err := Parse(strings.NewReader(input), nil)
	require.NoError(t, err)
	require.Len(t, insts, 3)
	assert.Equal(t, common.MY, insts[0].Orient)
	assert.Equal(t, common.R180, insts[1].Orient) // extra-quoted Virtuoso format
	assert.Equal(t, common.R0, insts[2].Orient)   // missing → defaults to R0
}
