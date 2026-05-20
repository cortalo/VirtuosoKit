package integration_test

import (
	"placer/celldb"
	"placer/common"
	"placer/place"
	"placer/rows"
	"placer/schematic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two INVD1BWP7T at the same Y=0 → one row, sorted by schematic X (I0 left of I1).
// Row 0 is R0. Cells abut: I0 at X=0, I1 at X=500 (INVD1BWP7T width).
func TestIntegration_TwoInverters_OneRow(t *testing.T) {
	instances, err := schematic.Load("testdata/schematic.json", nil)
	require.NoError(t, err)
	require.Len(t, instances, 2)

	grouped := rows.Group(instances, 1)
	require.Len(t, grouped, 1, "both instances share Y=0, expect one row")
	require.Len(t, grouped[0], 2)

	db, err := celldb.Load("testdata/cells.toml")
	require.NoError(t, err)

	placed, err := place.Place(grouped, db, 2000, nil)
	require.NoError(t, err)
	require.Len(t, placed, 2)

	// Sorted by schematic X: I0 (-0.96875) before I1 (-0.53125)
	assert.Equal(t, "I0", placed[0].Name)
	assert.Equal(t, "I1", placed[1].Name)

	// Row 0: R0, Y=0, cells abutted
	assert.Equal(t, 0, placed[0].X)
	assert.Equal(t, 0, placed[0].Y)
	assert.Equal(t, 1680, placed[1].X) // I0 width = 500
	assert.Equal(t, 0, placed[1].Y)
}

// schematic_three_row.json has 11 instances across three Y-clusters.
// basic lib instances (I47, I48) are filtered out, leaving 9.
// After grouping (threshold=1) and placing (rowHeight=2000):
//
//	row 0 (bottom, R0,  y=0):    I39, I27         — lowest Y ≈ -1.5
//	row 1 (middle, MX,  y=4000): I11, I38, I41, I42, I43
//	row 2 (top,    R0,  y=4000): I26, I28         — highest Y ≈ 1.7–2.0
func TestIntegration_ThreeRows(t *testing.T) {
	instances, err := schematic.Load("testdata/schematic_three_row.json", []string{"basic"})
	require.NoError(t, err)
	require.Len(t, instances, 9)

	grouped := rows.Group(instances, 1)
	require.Len(t, grouped, 3, "expect three distinct Y-clusters")

	namesOf := func(row []common.SchematicInstance) []string {
		names := make([]string, len(row))
		for i, inst := range row {
			names[i] = inst.Name
		}
		return names
	}

	// Rows are ordered bottom to top; within each row sorted by schematic X.
	assert.Equal(t, []string{"I39", "I27"}, namesOf(grouped[0]))
	assert.Equal(t, []string{"I11", "I38", "I41", "I42", "I43"}, namesOf(grouped[1]))
	assert.Equal(t, []string{"I26", "I28"}, namesOf(grouped[2]))

	db, err := celldb.Load("testdata/cells.toml")
	require.NoError(t, err)

	placed, err := place.Place(grouped, db, 2000, nil)
	require.NoError(t, err)
	require.Len(t, placed, 9)

	// Row 0: i=0, R0, y=0
	assert.Equal(t, "I39", placed[0].Name)
	assert.Equal(t, 0, placed[0].X)
	assert.Equal(t, 0, placed[0].Y)
	assert.Equal(t, common.R0, placed[0].Orient)

	assert.Equal(t, "I27", placed[1].Name)
	assert.Equal(t, 1680, placed[1].X)
	assert.Equal(t, 0, placed[1].Y)
	assert.Equal(t, common.R0, placed[1].Orient)

	// Row 1: i=1, MX, y=(1+1)*2000=4000
	assert.Equal(t, "I11", placed[2].Name)
	assert.Equal(t, 0, placed[2].X)
	assert.Equal(t, 4000, placed[2].Y)
	assert.Equal(t, common.MX, placed[2].Orient)

	assert.Equal(t, "I38", placed[3].Name)
	assert.Equal(t, 1680, placed[3].X)
	assert.Equal(t, 4000, placed[3].Y)

	assert.Equal(t, "I41", placed[4].Name)
	assert.Equal(t, 3360, placed[4].X)
	assert.Equal(t, 4000, placed[4].Y)

	assert.Equal(t, "I42", placed[5].Name)
	assert.Equal(t, 5040, placed[5].X)
	assert.Equal(t, 4000, placed[5].Y)

	assert.Equal(t, "I43", placed[6].Name)
	assert.Equal(t, 6720, placed[6].X)
	assert.Equal(t, 4000, placed[6].Y)

	// Row 2: i=2, R0, y=2*2000=4000
	assert.Equal(t, "I26", placed[7].Name)
	assert.Equal(t, 0, placed[7].X)
	assert.Equal(t, 4000, placed[7].Y)
	assert.Equal(t, common.R0, placed[7].Orient)

	assert.Equal(t, "I28", placed[8].Name)
	assert.Equal(t, 1680, placed[8].X)
	assert.Equal(t, 4000, placed[8].Y)
	assert.Equal(t, common.R0, placed[8].Orient)
}
