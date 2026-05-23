package celldb

import (
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
)

var (
	ErrLibNotFound  = errors.New("lib not found")
	ErrCellNotFound = errors.New("cell not found")
	ErrPinNotFound  = errors.New("pin not found")
)

type Pin struct {
	Name string `toml:"name"`
	LL   [2]int `toml:"ll"`
	UR   [2]int `toml:"ur"`
}

type Metal struct {
	Layer string `toml:"layer"` // "M1", "M2", "M3"
	LL    [2]int `toml:"ll"`
	UR    [2]int `toml:"ur"`
}

type cell struct {
	Pins   []Pin   `toml:"pins"`
	Metals []Metal `toml:"metals"`
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

func (db *DB) Query(lib, cellName, pinName string) (xLow, xHigh, yLow, yHigh int, err error) {
	cells, ok := db.libs[lib]
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("%w: %s", ErrLibNotFound, lib)
	}
	c, ok := cells[cellName]
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("%w: %s", ErrCellNotFound, cellName)
	}
	for _, p := range c.Pins {
		if p.Name == pinName {
			return p.LL[0], p.UR[0], p.LL[1], p.UR[1], nil
		}
	}
	return 0, 0, 0, 0, fmt.Errorf("%w: %s", ErrPinNotFound, pinName)
}
