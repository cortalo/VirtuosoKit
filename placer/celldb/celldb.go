package celldb

import (
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
)

var (
	ErrLibNotFound  = errors.New("lib not found")
	ErrCellNotFound = errors.New("cell not found")
)

type cell struct {
	Width int `toml:"width"`
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
