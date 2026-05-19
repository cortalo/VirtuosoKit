package rows

import (
	"placer/common"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func inst(name string, x, y float64) common.SchematicInstance {
	return common.SchematicInstance{Name: name, X: x, Y: y}
}

func names(row []common.SchematicInstance) []string {
	out := make([]string, len(row))
	for i, r := range row {
		out[i] = r.Name
	}
	return out
}

func TestGroup_Empty(t *testing.T) {
	assert.Nil(t, Group(nil, 1))
}

func TestGroup_SingleInstance(t *testing.T) {
	result := Group([]common.SchematicInstance{inst("A", 0, 0)}, 1)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"A"}, names(result[0]))
}

func TestGroup_AllSameY_OneRow(t *testing.T) {
	instances := []common.SchematicInstance{
		inst("C", 30, 0),
		inst("A", 10, 0),
		inst("B", 20, 0),
	}
	result := Group(instances, 1)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"A", "B", "C"}, names(result[0]))
}

func TestGroup_TwoRows_SortedBottomToTop(t *testing.T) {
	instances := []common.SchematicInstance{
		inst("B", 20, 100),
		inst("A", 10, 100),
		inst("D", 20, 0),
		inst("C", 10, 0),
	}
	result := Group(instances, 1)
	require.Len(t, result, 2)
	assert.Equal(t, []string{"C", "D"}, names(result[0]), "bottom row")
	assert.Equal(t, []string{"A", "B"}, names(result[1]), "top row")
}

func TestGroup_FiveRows_VaryingSpacing(t *testing.T) {
	instances := []common.SchematicInstance{
		inst("A1", 0, 0), inst("A2", 10, 0),
		inst("B1", 0, 20), inst("B2", 10, 20),
		inst("C1", 0, 60), inst("C2", 10, 60),
		inst("D1", 0, 80), inst("D2", 10, 80),
		inst("E1", 0, 180), inst("E2", 10, 180),
	}
	result := Group(instances, 1)
	require.Len(t, result, 5)
	assert.Equal(t, []string{"A1", "A2"}, names(result[0]))
	assert.Equal(t, []string{"B1", "B2"}, names(result[1]))
	assert.Equal(t, []string{"C1", "C2"}, names(result[2]))
	assert.Equal(t, []string{"D1", "D2"}, names(result[3]))
	assert.Equal(t, []string{"E1", "E2"}, names(result[4]))
}

func TestGroup_SloppySchematic_WithinRowOffsets(t *testing.T) {
	// Within-row Y offsets up to 0.5, row spacing is 50. threshold=1.
	instances := []common.SchematicInstance{
		inst("A1", 0, 0), inst("A2", 10, 0.5), inst("A3", 20, 0.3),
		inst("B1", 0, 50), inst("B2", 10, 50.5), inst("B3", 20, 50.2),
	}
	result := Group(instances, 1)
	require.Len(t, result, 2)
	assert.Len(t, result[0], 3)
	assert.Len(t, result[1], 3)
}
