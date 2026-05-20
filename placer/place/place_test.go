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

func siO(name, lib, cell string, orient common.Orient) common.SchematicInstance {
	return common.SchematicInstance{Name: name, Lib: lib, Cell: cell, Orient: orient}
}

var db = mockDB{
	"lib/A": 100,
	"lib/B": 200,
	"lib/C": 150,
	"lib/T": 50,
}

func TestPlace_SingleRow_R0_XAbutted(t *testing.T) {
	rows := [][]common.SchematicInstance{
		{si("i0", "lib", "A"), si("i1", "lib", "B"), si("i2", "lib", "C")},
	}
	got, err := Place(rows, db, 400, nil)
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
	got, err := Place(rows, db, 400, nil)
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
	got, err := Place(rows, db, 400, nil)
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
	_, err := Place(rows, db, 400, nil)
	assert.Error(t, err)
}

// Covers all four (rowFlipped × hFlip) combinations.
// lib/A width = 100.
//
// Row 0 (even, R0 base):
//
//	i0 R0  → R0,  origin at left  edge: X=0,   occupies [0,100]
//	i1 MY  → MY,  origin at right edge: X=200,  occupies [100,200]
//
// Row 1 (odd, MX base):
//
//	i2 MX  → MX,  origin at left  edge: X=0,   occupies [0,100]
//	i3 R180→ R180, origin at right edge: X=200, occupies [100,200]
func TestPlace_Orient_HorizontalFlipPreserved(t *testing.T) {
	rows := [][]common.SchematicInstance{
		{siO("i0", "lib", "A", common.R0), siO("i1", "lib", "A", common.MY)},
		{siO("i2", "lib", "A", common.MX), siO("i3", "lib", "A", common.R180)},
	}
	got, err := Place(rows, db, 400, nil)
	require.NoError(t, err)
	require.Len(t, got, 4)

	assert.Equal(t, common.R0, got[0].Orient)
	assert.Equal(t, 0, got[0].X)

	assert.Equal(t, common.MY, got[1].Orient)
	assert.Equal(t, 200, got[1].X) // origin at right edge: x(100) + width(100)

	assert.Equal(t, common.MX, got[2].Orient)
	assert.Equal(t, 0, got[2].X)

	assert.Equal(t, common.R180, got[3].Orient)
	assert.Equal(t, 200, got[3].X) // origin at right edge: x(100) + width(100)
}

// tieWidth=50, MaxSpacing=500: only start and end ties, no mid-row insertion.
// Layout: [T:0] [A:50] [T:150]
func TestPlace_Tapcell_StartAndEnd(t *testing.T) {
	tc := &common.TapcellConfig{Lib: "lib", Cell: "T", MaxSpacing: 500}
	rows := [][]common.SchematicInstance{
		{si("i0", "lib", "A")},
	}
	got, err := Place(rows, db, 400, tc)
	require.NoError(t, err)
	require.Len(t, got, 3) // start TIE, A, end TIE

	assert.Equal(t, "lib", got[0].Lib)
	assert.Equal(t, "T", got[0].Cell)
	assert.Equal(t, 0, got[0].X)

	assert.Equal(t, "i0", got[1].Name)
	assert.Equal(t, 50, got[1].X)

	assert.Equal(t, "T", got[2].Cell)
	assert.Equal(t, 150, got[2].X)
}

// tieWidth=50, MaxSpacing=80: mid-row tie inserted after each A(100) cell.
// Layout: [T:0] [A:50] [T:150] [B:200] [T:400] [T:450]
// After T(0→50): lastTieEnd=50
// After A(50→150): 150-50=100 > 80 → T(150→200), lastTieEnd=200
// After B(200→400): 400-200=200 > 80 → T(400→450), lastTieEnd=450
// End: T(450→500)
func TestPlace_Tapcell_MidRowInsertion(t *testing.T) {
	tc := &common.TapcellConfig{Lib: "lib", Cell: "T", MaxSpacing: 80}
	rows := [][]common.SchematicInstance{
		{si("i0", "lib", "A"), si("i1", "lib", "B")},
	}
	got, err := Place(rows, db, 400, tc)
	require.NoError(t, err)
	require.Len(t, got, 6) // start T, A, mid T, B, mid T, end T

	assert.Equal(t, 0, got[0].X)   // start TIE
	assert.Equal(t, 50, got[1].X)  // A
	assert.Equal(t, 150, got[2].X) // mid TIE (150-50=100 > 80)
	assert.Equal(t, 200, got[3].X) // B
	assert.Equal(t, 400, got[4].X) // mid TIE (400-200=200 > 80)
	assert.Equal(t, 450, got[5].X) // end TIE
}
