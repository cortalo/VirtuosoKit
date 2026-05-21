package celldb

import (
	"errors"
	"fmt"
	"placer/common"

	"github.com/BurntSushi/toml"
)

var (
	ErrLibNotFound  = errors.New("lib not found")
	ErrCellNotFound = errors.New("cell not found")
)

type cell struct {
	Width  int `toml:"width"`
	Filler int `toml:"filler"`
}

type rawTapcell struct {
	Lib        string `toml:"lib"`
	Cell       string `toml:"cell"`
	MaxSpacing int    `toml:"max_spacing"`
}

type rawFillercell struct {
	Lib  string `toml:"lib"`
	Cell string `toml:"cell"`
}

type DB struct {
	libs       map[string]map[string]cell
	tapcell    *common.TapcellConfig
	fillercell *common.FillerCellConfig
}

func Load(path string) (*DB, error) {
	var raw map[string]toml.Primitive
	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return nil, fmt.Errorf("celldb: %w", err)
	}

	db := &DB{libs: make(map[string]map[string]cell)}

	for key, prim := range raw {
		if key == "tapcell" {
			var tc rawTapcell
			if err := meta.PrimitiveDecode(prim, &tc); err != nil {
				return nil, fmt.Errorf("celldb: tapcell: %w", err)
			}
			db.tapcell = &common.TapcellConfig{
				Lib:        tc.Lib,
				Cell:       tc.Cell,
				MaxSpacing: tc.MaxSpacing,
			}
		} else if key == "fillercell" {
			var fc rawFillercell
			if err := meta.PrimitiveDecode(prim, &fc); err != nil {
				return nil, fmt.Errorf("celldb: fillercell: %w", err)
			}
			db.fillercell = &common.FillerCellConfig{Lib: fc.Lib, Cell: fc.Cell}
		} else {
			var cells map[string]cell
			if err := meta.PrimitiveDecode(prim, &cells); err != nil {
				return nil, fmt.Errorf("celldb: lib %q: %w", key, err)
			}
			db.libs[key] = cells
		}
	}

	return db, nil
}

func (db *DB) Query(lib, cellName string) (width int, err error) {
	cells, ok := db.libs[lib]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrLibNotFound, lib)
	}
	c, ok := cells[cellName]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrCellNotFound, cellName)
	}
	return c.Width, nil
}

func (db *DB) Tapcell() (common.TapcellConfig, bool) {
	if db.tapcell == nil {
		return common.TapcellConfig{}, false
	}
	return *db.tapcell, true
}

func (db *DB) FillerCell() (common.FillerCellConfig, bool) {
	if db.fillercell == nil {
		return common.FillerCellConfig{}, false
	}
	return *db.fillercell, true
}

func (db *DB) IsFillerCompatible(lib, cellName string) bool {
	cells, ok := db.libs[lib]
	if !ok {
		return false
	}
	c, ok := cells[cellName]
	if !ok {
		return false
	}
	return c.Filler == 1
}
