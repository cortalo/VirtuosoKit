package pindb

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
	X    int    `toml:"x"`
	Y    int    `toml:"y"`
}

type cell struct {
	Pins []Pin `toml:"pins"`
}

type DB struct {
	libs map[string]map[string]cell
}

func Load(path string) (*DB, error) {
	var raw map[string]map[string]cell
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("pindb: %w", err)
	}
	return &DB{libs: raw}, nil
}

func (db *DB) Query(lib, cellName, pinName string) (x, y int, err error) {
	cells, ok := db.libs[lib]
	if !ok {
		return 0, 0, fmt.Errorf("%w: %s", ErrLibNotFound, lib)
	}
	c, ok := cells[cellName]
	if !ok {
		return 0, 0, fmt.Errorf("%w: %s", ErrCellNotFound, cellName)
	}
	for _, p := range c.Pins {
		if p.Name == pinName {
			return p.X, p.Y, nil
		}
	}
	return 0, 0, fmt.Errorf("%w: %s", ErrPinNotFound, pinName)
}
