package drcdb

import (
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
)

var (
	ErrLibNotFound   = errors.New("lib not found")
	ErrLayerNotFound = errors.New("layer not found")
)

type layerRules struct {
	MinArea int `toml:"min_area"`
}

// DRCSpec holds the manufacturing rules for a single metal layer.
type DRCSpec struct {
	minArea int
}

func (s DRCSpec) MinArea() int { return s.minArea }

type DB struct {
	libs map[string]map[string]layerRules
}

func Load(path string) (*DB, error) {
	var raw map[string]map[string]layerRules
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("drcdb: %w", err)
	}
	return &DB{libs: raw}, nil
}

func (db *DB) Query(lib, layer string) (DRCSpec, error) {
	layers, ok := db.libs[lib]
	if !ok {
		return DRCSpec{}, fmt.Errorf("%w: %s", ErrLibNotFound, lib)
	}
	rules, ok := layers[layer]
	if !ok {
		return DRCSpec{}, fmt.Errorf("%w: %s", ErrLayerNotFound, layer)
	}
	return DRCSpec{minArea: rules.MinArea}, nil
}
