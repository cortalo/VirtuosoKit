package integration_test

import (
	"placer/celldb"
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

	placed, err := place.Place(grouped, db, 2000)
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
