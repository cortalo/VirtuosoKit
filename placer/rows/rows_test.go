package rows

import (
	"placer/common"
	"sort"
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

func instW(name string, width int) common.SchematicInstance {
	return common.SchematicInstance{Name: name, Width: width}
}

func groupOf(insts ...common.SchematicInstance) []common.SchematicInstance {
	return insts
}

func TestSplitByWidth_Empty(t *testing.T) {
	assert.Nil(t, SplitByWidth(nil, 1000))
}

func TestSplitByWidth_AllFitInOne(t *testing.T) {
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 300), instW("B", 300), instW("C", 300)),
	}
	result := SplitByWidth(groups, 1000)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"A", "B", "C"}, names(result[0]))
}

func TestSplitByWidth_SplitsWhenFull(t *testing.T) {
	// 400+400=800 ≤ 1000, adding 400 → 1200 > 1000 → split
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 400), instW("B", 400), instW("C", 400)),
	}
	result := SplitByWidth(groups, 1000)
	require.Len(t, result, 2)
	assert.Equal(t, []string{"A", "B"}, names(result[0]))
	assert.Equal(t, []string{"C"}, names(result[1]))
}

func TestSplitByWidth_ExactFit(t *testing.T) {
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 500), instW("B", 500)),
	}
	result := SplitByWidth(groups, 1000)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"A", "B"}, names(result[0]))
}

func TestSplitByWidth_OversizedSingleCell_PlacedAlone(t *testing.T) {
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 300), instW("B", 1500), instW("C", 300)),
	}
	result := SplitByWidth(groups, 1000)
	require.Len(t, result, 3)
	assert.Equal(t, []string{"A"}, names(result[0]))
	assert.Equal(t, []string{"B"}, names(result[1]))
	assert.Equal(t, []string{"C"}, names(result[2]))
}

func TestSplitByWidth_MultipleInputGroups(t *testing.T) {
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 600), instW("B", 600)),
		groupOf(instW("C", 400), instW("D", 400), instW("E", 400)),
	}
	result := SplitByWidth(groups, 1000)
	// group1: A(600)+B(600)=1200>1000 → [A],[B]
	// group2: C+D=800≤1000, +E=1200>1000 → [C,D],[E]
	require.Len(t, result, 4)
	assert.Equal(t, []string{"A"}, names(result[0]))
	assert.Equal(t, []string{"B"}, names(result[1]))
	assert.Equal(t, []string{"C", "D"}, names(result[2]))
	assert.Equal(t, []string{"E"}, names(result[3]))
}

func TestSplitByWidth_ZeroWidthCells_NeverSplit(t *testing.T) {
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 0), instW("B", 0), instW("C", 0)),
	}
	result := SplitByWidth(groups, 1000)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"A", "B", "C"}, names(result[0]))
}

func TestRepackByWidth_Empty(t *testing.T) {
	assert.Nil(t, RepackByWidth(nil, 1000))
}

func TestRepackByWidth_AllFitInOne(t *testing.T) {
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 300)),
		groupOf(instW("B", 300)),
		groupOf(instW("C", 300)),
	}
	result := RepackByWidth(groups, 1000)
	require.Len(t, result, 1)
	assert.Len(t, result[0], 3)
}

func TestRepackByWidth_FFD_PacksOptimally(t *testing.T) {
	// widths: 600, 400, 400, 400 → FFD sorts: 600,400,400,400
	// bin0: 600 → +400=1000 ✓; bin1: 400 → +400=800 ✓
	// result: 2 bins of width 1000 and 800
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 400)),
		groupOf(instW("B", 400)),
		groupOf(instW("C", 600)),
		groupOf(instW("D", 400)),
	}
	result := RepackByWidth(groups, 1000)
	require.Len(t, result, 2)
	widths := []int{totalWidth(result[0]), totalWidth(result[1])}
	sort.Ints(widths)
	assert.Equal(t, []int{800, 1000}, widths)
}

func TestRepackByWidth_OversizedGroup_PlacedAlone(t *testing.T) {
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 1500)),
		groupOf(instW("B", 300)),
	}
	result := RepackByWidth(groups, 1000)
	// A is oversized → its own bin; B fits in a new bin
	require.Len(t, result, 2)
}

func TestAddFiller_Empty(t *testing.T) {
	assert.Nil(t, AddFiller(nil, func(lib, cell string) bool { return true }, "lib", "FILL", 280))
}

func TestAddFiller_NoCompatibleCells_NoInsert(t *testing.T) {
	groups := [][]common.SchematicInstance{
		groupOf(instW("A", 300), instW("B", 300)),
	}
	result := AddFiller(groups, func(lib, cell string) bool { return false }, "lib", "FILL", 280)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"A", "B"}, names(result[0]))
}

func TestAddFiller_BothCompatible_InsertsCell(t *testing.T) {
	groups := [][]common.SchematicInstance{
		{{Name: "A", Lib: "mylib", Cell: "CellA", Width: 300}, {Name: "B", Lib: "mylib", Cell: "CellA", Width: 300}},
	}
	compat := func(lib, cell string) bool { return lib == "mylib" }
	result := AddFiller(groups, compat, "mylib", "FILL", 280)
	require.Len(t, result, 1)
	require.Len(t, result[0], 3)
	assert.Equal(t, []string{"A", "FILLER_0", "B"}, names(result[0]))
	assert.Equal(t, 280, result[0][1].Width)
}

func TestAddFiller_OnlyOneCompatible_NoInsert(t *testing.T) {
	groups := [][]common.SchematicInstance{
		{{Name: "A", Lib: "mylib", Cell: "CellA"}, {Name: "B", Lib: "other", Cell: "CellB"}},
	}
	compat := func(lib, cell string) bool { return lib == "mylib" }
	result := AddFiller(groups, compat, "mylib", "FILL", 280)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"A", "B"}, names(result[0]))
}

func TestAddFiller_MultipleRows_UniqueNames(t *testing.T) {
	mk := func(name, lib string) common.SchematicInstance { return common.SchematicInstance{Name: name, Lib: lib} }
	groups := [][]common.SchematicInstance{
		{mk("A", "mylib"), mk("B", "mylib")},
		{mk("C", "mylib"), mk("D", "mylib")},
	}
	compat := func(lib, cell string) bool { return lib == "mylib" }
	result := AddFiller(groups, compat, "mylib", "FILL", 280)
	require.Len(t, result, 2)
	assert.Equal(t, []string{"A", "FILLER_0", "B"}, names(result[0]))
	assert.Equal(t, []string{"C", "FILLER_1", "D"}, names(result[1]))
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
