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
  { name = "D", x = 10, y = 20 },
  { name = "G", x = 30, y = 40 },
  { name = "S", x = 50, y = 60 },
]

[tsmc18.pmos2v]
pins = [
  { name = "D", x = 5, y = 15 },
]

[other.cell1]
pins = [
  { name = "A", x = 1, y = 2 },
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

	x, y, err := db.Query("tsmc18", "nmos2v", "D")
	require.NoError(t, err)
	assert.Equal(t, 10, x)
	assert.Equal(t, 20, y)
}

func TestQuery_DifferentLibAndCell(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	x, y, err := db.Query("other", "cell1", "A")
	require.NoError(t, err)
	assert.Equal(t, 1, x)
	assert.Equal(t, 2, y)
}

func TestQuery_LibNotFound(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, err = db.Query("unknown", "nmos2v", "D")
	assert.ErrorIs(t, err, pindb.ErrLibNotFound)
}

func TestQuery_CellNotFound(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, err = db.Query("tsmc18", "unknown", "D")
	assert.ErrorIs(t, err, pindb.ErrCellNotFound)
}

func TestQuery_PinNotFound(t *testing.T) {
	db, err := pindb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, _, err = db.Query("tsmc18", "nmos2v", "Z")
	assert.ErrorIs(t, err, pindb.ErrPinNotFound)
}
