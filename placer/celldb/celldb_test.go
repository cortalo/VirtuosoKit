package celldb_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"placer/celldb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeToml(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "cells*.toml")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return filepath.Clean(f.Name())
}

func TestLoad_ValidFile_ReturnsDB(t *testing.T) {
	path := writeToml(t, `
[mylib.CellA]
width = 1000

[mylib.CellB]
width = 2000
`)
	db, err := celldb.Load(path)
	require.NoError(t, err)
	require.NotNil(t, db)
}

func TestQuery_ExistingCell_ReturnsWidth(t *testing.T) {
	path := writeToml(t, `
[mylib.CellA]
width = 1000
`)
	db, err := celldb.Load(path)
	require.NoError(t, err)

	w, err := db.Query("mylib", "CellA")
	require.NoError(t, err)
	assert.Equal(t, 1000, w)
}

func TestQuery_UnknownLib_ReturnsError(t *testing.T) {
	path := writeToml(t, `
[mylib.CellA]
width = 1000
`)
	db, _ := celldb.Load(path)

	_, err := db.Query("otherlib", "CellA")
	assert.ErrorIs(t, err, celldb.ErrLibNotFound)
}

func TestQuery_UnknownCell_ReturnsError(t *testing.T) {
	path := writeToml(t, `
[mylib.CellA]
width = 1000
`)
	db, _ := celldb.Load(path)

	_, err := db.Query("mylib", "CellB")
	assert.ErrorIs(t, err, celldb.ErrCellNotFound)
}

func TestLoad_InvalidFile_ReturnsError(t *testing.T) {
	_, err := celldb.Load("nonexistent.toml")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errors.Unwrap(err))
}

func TestLoad_WithTapcell_ReturnsTapcellConfig(t *testing.T) {
	path := writeToml(t, `
[tapcell]
lib = "mylib"
cell = "TIE"
max_spacing = 50000

[mylib.CellA]
width = 1000
`)
	db, err := celldb.Load(path)
	require.NoError(t, err)

	tc, ok := db.Tapcell()
	require.True(t, ok)
	assert.Equal(t, "mylib", tc.Lib)
	assert.Equal(t, "TIE", tc.Cell)
	assert.Equal(t, 50000, tc.MaxSpacing)

	w, err := db.Query("mylib", "CellA")
	require.NoError(t, err)
	assert.Equal(t, 1000, w)
}

func TestLoad_WithoutTapcell_ReturnsFalse(t *testing.T) {
	path := writeToml(t, `
[mylib.CellA]
width = 1000
`)
	db, err := celldb.Load(path)
	require.NoError(t, err)

	_, ok := db.Tapcell()
	assert.False(t, ok)
}
