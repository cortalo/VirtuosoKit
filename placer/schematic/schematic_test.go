package schematic

import (
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

func TestLoad_InvalidPath_ReturnsError(t *testing.T) {
	_, err := Load("nonexistent.json", nil)
	assert.Error(t, err)
}
