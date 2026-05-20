package schematic

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"placer/common"
	"strings"
)

type raw struct {
	Instances []rawInstance `json:"instances"`
}

type rawInstance struct {
	Name   string     `json:"name"`
	Lib    string     `json:"lib"`
	Cell   string     `json:"cell"`
	XY     [2]float64 `json:"xy"`
	Orient string     `json:"orient"`
}

// Parse reads schematic JSON from r and returns the instances, excluding any
// whose lib appears in ignoreLibs.
func Parse(r io.Reader, ignoreLibs []string) ([]common.SchematicInstance, error) {
	var s raw
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, fmt.Errorf("schematic: %w", err)
	}
	return filter(s.Instances, ignoreLibs), nil
}

// Load reads a schematic JSON file and returns the instances, excluding any
// whose lib appears in ignoreLibs.
func Load(path string, ignoreLibs []string) ([]common.SchematicInstance, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("schematic: %w", err)
	}
	defer f.Close()
	return Parse(f, ignoreLibs)
}

func filter(insts []rawInstance, ignoreLibs []string) []common.SchematicInstance {
	ignored := make(map[string]struct{}, len(ignoreLibs))
	for _, lib := range ignoreLibs {
		ignored[lib] = struct{}{}
	}
	var result []common.SchematicInstance
	for _, inst := range insts {
		if _, skip := ignored[inst.Lib]; skip {
			continue
		}
		result = append(result, common.SchematicInstance{
			Name:   inst.Name,
			Lib:    inst.Lib,
			Cell:   inst.Cell,
			X:      inst.XY[0],
			Y:      inst.XY[1],
			Orient: parseOrient(inst.Orient),
		})
	}
	return result
}

// parseOrient converts a Virtuoso orient string (optionally wrapped in extra
// quotes) to an Orient value, defaulting to R0 on empty or unknown input.
func parseOrient(s string) common.Orient {
	s = strings.Trim(s, "\"")
	var o common.Orient
	_ = o.UnmarshalText([]byte(s))
	return o
}
