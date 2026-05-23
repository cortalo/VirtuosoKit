package celldb_test

import (
	"os"
	"path/filepath"
	"testing"

	"autorouter/celldb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTOML = `
[tsmc18.nmos2v]
pins = [
  { name = "D", ll = [10, 20], ur = [30, 50] },
  { name = "G", ll = [30, 40], ur = [50, 70] },
  { name = "S", ll = [50, 60], ur = [70, 90] },
]

[tsmc18.pmos2v]
pins = [
  { name = "D", ll = [5, 15], ur = [25, 45] },
]

[other.cell1]
pins = [
  { name = "A", ll = [1, 2], ur = [3, 4] },
]

[other.cell2]
pins = [
  { name = "A", ll = [1, 2], ur = [3, 4] },
]
metals = [
  { layer = "M2", ll = [10, 0], ur = [20, 100] },
  { layer = "M3", ll = [0, 50], ur = [200, 60] },
]

[other.cell3]
pins = [
  { name = "A", ll = [1, 2], ur = [3, 4] },
]
metals = []
`

func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cells.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoad_ValidFile(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := celldb.Load("/nonexistent/path/cells.toml")
	assert.Error(t, err)
}

func TestLoad_InvalidTOML(t *testing.T) {
	_, err := celldb.Load(writeTempTOML(t, "this is not valid toml ]["))
	assert.Error(t, err)
}

func TestQuery_ReturnsCorrectCoordinates(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	xLow, xHigh, yLow, yHigh, err := db.Query("tsmc18", "nmos2v", "D")
	require.NoError(t, err)
	assert.Equal(t, 10, xLow)
	assert.Equal(t, 30, xHigh)
	assert.Equal(t, 20, yLow)
	assert.Equal(t, 50, yHigh)
}

func TestQuery_DifferentLibAndCell(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	xLow, xHigh, yLow, yHigh, err := db.Query("other", "cell1", "A")
	require.NoError(t, err)
	assert.Equal(t, 1, xLow)
	assert.Equal(t, 3, xHigh)
	assert.Equal(t, 2, yLow)
	assert.Equal(t, 4, yHigh)
}

func TestQuery_LibNotFound(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, _, _, err = db.Query("unknown", "nmos2v", "D")
	assert.ErrorIs(t, err, celldb.ErrLibNotFound)
}

func TestQuery_CellNotFound(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, _, _, err = db.Query("tsmc18", "unknown", "D")
	assert.ErrorIs(t, err, celldb.ErrCellNotFound)
}

func TestQuery_PinNotFound(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, _, _, err = db.Query("tsmc18", "nmos2v", "Z")
	assert.ErrorIs(t, err, celldb.ErrPinNotFound)
}

// --- QueryMetals ---

func TestQueryMetals_ReturnsMetals(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	metals, err := db.QueryMetals("other", "cell2")
	require.NoError(t, err)
	require.Len(t, metals, 2)
	assert.Equal(t, "M2", metals[0].Layer)
	assert.Equal(t, [2]int{10, 0}, metals[0].LL)
	assert.Equal(t, [2]int{20, 100}, metals[0].UR)
	assert.Equal(t, "M3", metals[1].Layer)
}

func TestQueryMetals_NoMetalsField_ReturnsEmpty(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	metals, err := db.QueryMetals("tsmc18", "nmos2v")
	require.NoError(t, err)
	assert.Empty(t, metals)
}

func TestQueryMetals_EmptyMetalsField_ReturnsEmpty(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	metals, err := db.QueryMetals("other", "cell3")
	require.NoError(t, err)
	assert.Empty(t, metals)
}

func TestQueryMetals_LibNotFound(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, err = db.QueryMetals("unknown", "cell2")
	assert.ErrorIs(t, err, celldb.ErrLibNotFound)
}

func TestQueryMetals_CellNotFound(t *testing.T) {
	db, err := celldb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, err = db.QueryMetals("other", "unknown")
	assert.ErrorIs(t, err, celldb.ErrCellNotFound)
}
