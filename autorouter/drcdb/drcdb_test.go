package drcdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"autorouter/common"
	"autorouter/drcdb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// segWithArea returns a Segment whose area equals w*h.
func segWithArea(area int) common.Segment {
	return common.Segment{
		LowerLeft:  common.Point{X: 0, Y: 0},
		UpperRight: common.Point{X: area, Y: 1},
	}
}

const testTOML = `
[tsmc18.M2]
min_area = 100000

[tsmc18.M3]
min_area = 200000

[other.M2]
min_area = 50000
`

func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "drc.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoad_ValidFile(t *testing.T) {
	db, err := drcdb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := drcdb.Load("/nonexistent/path/drc.toml")
	assert.Error(t, err)
}

func TestLoad_InvalidTOML(t *testing.T) {
	_, err := drcdb.Load(writeTempTOML(t, "this is not valid toml ]["))
	assert.Error(t, err)
}

func TestQuery_ReturnsCorrectMinArea(t *testing.T) {
	db, err := drcdb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	spec, err := db.Query("tsmc18", "M2")
	require.NoError(t, err)
	assert.True(t, spec.SatisfiesMinArea(segWithArea(100000)), "area==minArea should satisfy")
	assert.False(t, spec.SatisfiesMinArea(segWithArea(99999)), "area<minArea should not satisfy")
}

func TestQuery_DifferentLibAndLayer(t *testing.T) {
	db, err := drcdb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	spec, err := db.Query("other", "M2")
	require.NoError(t, err)
	assert.True(t, spec.SatisfiesMinArea(segWithArea(50000)), "area==minArea should satisfy")
	assert.False(t, spec.SatisfiesMinArea(segWithArea(49999)), "area<minArea should not satisfy")
}

func TestQuery_LibNotFound(t *testing.T) {
	db, err := drcdb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, err = db.Query("unknown", "M2")
	assert.ErrorIs(t, err, drcdb.ErrLibNotFound)
}

func TestQuery_LayerNotFound(t *testing.T) {
	db, err := drcdb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	_, err = db.Query("tsmc18", "M1")
	assert.ErrorIs(t, err, drcdb.ErrLayerNotFound)
}

func TestQuery_ReturnsViaEnclosure(t *testing.T) {
	toml := `
[lib.M2]
min_area = 100000
end_extension = 60
via_enclosure = 30
`
	db, err := drcdb.Load(writeTempTOML(t, toml))
	require.NoError(t, err)

	spec, err := db.Query("lib", "M2")
	require.NoError(t, err)
	lo, hi := spec.ApplyEndExtension(100, 200)
	assert.Equal(t, 40, lo)  // 100 - 60
	assert.Equal(t, 260, hi) // 200 + 60
	assert.Equal(t, 30, spec.ViaEnclosure())
}

func TestQuery_ViaEnclosureDefaultsToZero(t *testing.T) {
	db, err := drcdb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	spec, err := db.Query("tsmc18", "M2")
	require.NoError(t, err)
	assert.Equal(t, 0, spec.ViaEnclosure())
}

func TestQuery_ReturnsViaTrackSpacing(t *testing.T) {
	toml := `
[lib.M2]
via_track_spacing = 3
`
	db, err := drcdb.Load(writeTempTOML(t, toml))
	require.NoError(t, err)

	spec, err := db.Query("lib", "M2")
	require.NoError(t, err)
	assert.Equal(t, 3, spec.ViaTrackSpacing())
}

func TestQuery_ViaTrackSpacingDefaultsToOne(t *testing.T) {
	db, err := drcdb.Load(writeTempTOML(t, testTOML))
	require.NoError(t, err)

	spec, err := db.Query("tsmc18", "M2")
	require.NoError(t, err)
	assert.Equal(t, 1, spec.ViaTrackSpacing())
}
