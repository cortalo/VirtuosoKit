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
	Width int `toml:"width"`
}

type rawTapcell struct {
	Lib        string `toml:"lib"`
	Cell       string `toml:"cell"`
	MaxSpacing int    `toml:"max_spacing"`
}

type DB struct {
	libs    map[string]map[string]cell
	tapcell *common.TapcellConfig
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
