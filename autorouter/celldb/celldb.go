package celldb

import (
	"autorouter/common"
	"fmt"

	"github.com/BurntSushi/toml"
)

var (
	ErrLibNotFound  = fmt.Errorf("lib not found: %w", common.ErrPinNotFound)
	ErrCellNotFound = fmt.Errorf("cell not found: %w", common.ErrPinNotFound)
	ErrPinNotFound  = fmt.Errorf("pin not found: %w", common.ErrPinNotFound)
)

type Pin struct {
	Name  string `toml:"name"`
	Layer string `toml:"layer"` // "M1", "M2", etc.; absent means M1
	LL    [2]int `toml:"ll"`
	UR    [2]int `toml:"ur"`
}

type Metal struct {
	Layer string `toml:"layer"` // "M1", "M2", "M3"
	LL    [2]int `toml:"ll"`
	UR    [2]int `toml:"ur"`
}

type cell struct {
	Pins   []Pin   `toml:"pins"`
	Metals []Metal `toml:"metals"`
	Escape bool    `toml:"escape"` // if true, pin ll/ur values are escape targets; actual geometry comes from layout JSON
}

type DB struct {
	libs map[string]map[string]cell
}

func Load(path string) (*DB, error) {
	var raw map[string]map[string]cell
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("celldb: %w", err)
	}
	return &DB{libs: raw}, nil
}

func (db *DB) IsEscapeCell(lib, cellName string) (bool, error) {
	cells, ok := db.libs[lib]
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrLibNotFound, lib)
	}
	c, ok := cells[cellName]
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrCellNotFound, cellName)
	}
	return c.Escape, nil
}

func (db *DB) QueryMetals(lib, cellName string) ([]Metal, error) {
	cells, ok := db.libs[lib]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrLibNotFound, lib)
	}
	c, ok := cells[cellName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCellNotFound, cellName)
	}
	return c.Metals, nil
}

func (db *DB) Query(lib, cellName, pinName string) (xLow, xHigh, yLow, yHigh common.Nm, layer common.Layer, err error) {
	cells, ok := db.libs[lib]
	if !ok {
		err = fmt.Errorf("%w: %s", ErrLibNotFound, lib)
		return
	}
	c, ok := cells[cellName]
	if !ok {
		err = fmt.Errorf("%w: %s", ErrCellNotFound, cellName)
		return
	}
	for _, p := range c.Pins {
		if p.Name == pinName {
			xLow, xHigh, yLow, yHigh = common.Nm(p.LL[0]), common.Nm(p.UR[0]), common.Nm(p.LL[1]), common.Nm(p.UR[1])
			if p.Layer == "" {
				layer = common.M1
			} else {
				layer, err = common.ParseLayer(p.Layer)
			}
			return
		}
	}
	err = fmt.Errorf("%w: %s", ErrPinNotFound, pinName)
	return
}
