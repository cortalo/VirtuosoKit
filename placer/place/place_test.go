package place

import (
	"placer/common"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDB map[string]int // "lib/cell" -> width

func (m mockDB) Query(lib, cell string) (int, error) {
	if w, ok := m[lib+"/"+cell]; ok {
		return w, nil
	}
	return 0, assert.AnError
}

func si(name, lib, cell string) common.SchematicInstance {
	return common.SchematicInstance{Name: name, Lib: lib, Cell: cell}
}

var db = mockDB{
	"lib/A": 100,
	"lib/B": 200,
	"lib/C": 150,
}

func TestPlace_SingleRow_R0_XAbutted(t *testing.T) {
	rows := [][]common.SchematicInstance{
		{si("i0", "lib", "A"), si("i1", "lib", "B"), si("i2", "lib", "C")},
	}
	got, err := Place(rows, db, 400)
	require.NoError(t, err)
	require.Len(t, got, 3)

	assert.Equal(t, common.R0, got[0].Orient)
	assert.Equal(t, 0, got[0].X)
	assert.Equal(t, 0, got[0].Y)

	assert.Equal(t, 100, got[1].X) // after A (width 100)
	assert.Equal(t, 300, got[2].X) // after A+B (100+200)
}

func TestPlace_TwoRows_OrientAlternates(t *testing.T) {
	rows := [][]common.SchematicInstance{
		{si("i0", "lib", "A")},
		{si("i1", "lib", "A")},
	}
	got, err := Place(rows, db, 400)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Row 0: R0 at y=0
	assert.Equal(t, common.R0, got[0].Orient)
	assert.Equal(t, 0, got[0].Y)

	// Row 1: MX at y=(1+1)*400=800 (cell extends downward to y=400)
	assert.Equal(t, common.MX, got[1].Orient)
	assert.Equal(t, 800, got[1].Y)
}

func TestPlace_FourRows_RailsSare(t *testing.T) {
	// Verify Y positions for 4 rows with rowHeight=400.
	// Row 0 R0  at y=0    spans [0,   400]
	// Row 1 MX  at y=800  spans [400, 800]
	// Row 2 R0  at y=800  spans [800, 1200]
	// Row 3 MX  at y=1600 spans [1200,1600]
	rows := [][]common.SchematicInstance{
		{si("a", "lib", "A")},
		{si("b", "lib", "A")},
		{si("c", "lib", "A")},
		{si("d", "lib", "A")},
	}
	got, err := Place(rows, db, 400)
	require.NoError(t, err)

	assert.Equal(t, 0, got[0].Y)
	assert.Equal(t, 800, got[1].Y)
	assert.Equal(t, 800, got[2].Y)
	assert.Equal(t, 1600, got[3].Y)
}

func TestPlace_UnknownCell_ReturnsError(t *testing.T) {
	rows := [][]common.SchematicInstance{
		{si("i0", "lib", "X")},
	}
	_, err := Place(rows, db, 400)
	assert.Error(t, err)
}
