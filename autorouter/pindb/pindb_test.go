package pindb_test

import (
	"os"
	"path/filepath"
	"testing"

	"autorouter/pindb"

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
`

func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pins.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoad_ValidFile(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := pindb.Load("/nonexistent/path/pins.toml")
	assert.Error(t, err)
}

func TestLoad_InvalidTOML(t *testing.T) {
	_, err := pindb.Load(writeTempTOML(t, "this is not valid toml ]["))
	assert.Error(t, err)
}

func TestQuery_ReturnsCorrectCoordinates(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	xLow, yLow, yHigh, err := db.Query("tsmc18", "nmos2v", "D")
	require.NoError(t, err)
	assert.Equal(t, 10, xLow)
	assert.Equal(t, 20, yLow)
	assert.Equal(t, 50, yHigh)
}

func TestQuery_DifferentLibAndCell(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	xLow, yLow, yHigh, err := db.Query("other", "cell1", "A")
	require.NoError(t, err)
	assert.Equal(t, 1, xLow)
	assert.Equal(t, 2, yLow)
	assert.Equal(t, 4, yHigh)
}

func TestQuery_LibNotFound(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, _, err = db.Query("unknown", "nmos2v", "D")
	assert.ErrorIs(t, err, pindb.ErrLibNotFound)
}

func TestQuery_CellNotFound(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, _, err = db.Query("tsmc18", "unknown", "D")
	assert.ErrorIs(t, err, pindb.ErrCellNotFound)
}

func TestQuery_PinNotFound(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, _, err = db.Query("tsmc18", "nmos2v", "Z")
	assert.ErrorIs(t, err, pindb.ErrPinNotFound)
}
