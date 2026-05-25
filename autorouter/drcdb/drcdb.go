package drcdb

import (
	"autorouter/common"
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
)

var (
	ErrLibNotFound   = errors.New("lib not found")
	ErrLayerNotFound = errors.New("layer not found")
)

// rawEntry holds all possible fields for both metal layer and via entries.
// TOML leaves unset fields at their zero values.
type rawEntry struct {
	// metal layer fields
	MinArea         int `toml:"min_area"`
	EndExtension    int `toml:"end_extension"`
	ViaEnclosure    int `toml:"via_enclosure"`
	ViaTrackSpacing int `toml:"via_track_spacing"`
	MinSpace        int `toml:"min_space"`
	MinPinOverlap   int `toml:"min_pin_overlap"`
	// via fields
	ViaDef string `toml:"via_def"`
	CutW   int    `toml:"cut_w"`
	CutH   int    `toml:"cut_h"`
	SpaceX int    `toml:"space_x"`
	SpaceY int    `toml:"space_y"`
}

// DRCSpec holds the manufacturing rules for a single metal layer.
type DRCSpec struct {
	minArea         int
	endExtension    int
	viaEnclosure    int
	viaTrackSpacing int
	minSpace        int
	minPinOverlap   int
}

func (s DRCSpec) SatisfiesMinArea(seg common.Segment) bool { return seg.GetArea() >= s.minArea }
func (s DRCSpec) ApplyEndExtension(lo, hi int) (int, int) {
	return lo - s.endExtension, hi + s.endExtension
}
func (s DRCSpec) ViaEnclosure() int { return s.viaEnclosure }
func (s DRCSpec) ViaTrackSpacing() int {
	if s.viaTrackSpacing == 0 {
		return 1
	}
	return s.viaTrackSpacing
}
func (s DRCSpec) ApplyMinSpaceExtension(lo, hi int) (int, int) {
	return lo - s.minSpace, hi + s.minSpace
}
func (s DRCSpec) MinPinOverlap() int { return s.minPinOverlap }

type DB struct {
	libs map[string]map[string]rawEntry
}

func Load(path string) (*DB, error) {
	var raw map[string]map[string]rawEntry
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
	e, ok := layers[layer]
	if !ok {
		return DRCSpec{}, fmt.Errorf("%w: %s", ErrLayerNotFound, layer)
	}
	return DRCSpec{minArea: e.MinArea, endExtension: e.EndExtension, viaEnclosure: e.ViaEnclosure, viaTrackSpacing: e.ViaTrackSpacing, minSpace: e.MinSpace, minPinOverlap: e.MinPinOverlap}, nil
}

func (db *DB) QueryVia(lib, viaName string) (common.ViaConfig, error) {
	layers, ok := db.libs[lib]
	if !ok {
		return common.ViaConfig{}, fmt.Errorf("%w: %s", ErrLibNotFound, lib)
	}
	e, ok := layers[viaName]
	if !ok {
		return common.ViaConfig{}, fmt.Errorf("%w: %s", ErrLayerNotFound, viaName)
	}
	return common.ViaConfig{
		ViaDef: e.ViaDef,
		CutW:   e.CutW,
		CutH:   e.CutH,
		SpaceX: e.SpaceX,
		SpaceY: e.SpaceY,
	}, nil
}
