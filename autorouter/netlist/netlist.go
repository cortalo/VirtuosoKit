package netlist

import (
	"autorouter/common"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type layoutInstance struct {
	Name string     `json:"name"`
	Lib  string     `json:"lib"`
	Cell string     `json:"cell"`
	XY   [2]float64 `json:"xy"`
}

type layoutFile struct {
	Instances []layoutInstance `json:"instances"`
}

type schematicFile struct {
	Nets map[string][]string `json:"nets"` // net name → ["inst.pin", ...]
}

func parseJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("netlist: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("netlist: %w", err)
	}
	return nil
}

// BuildNets combines a layout JSON, schematic JSON, and pin database into a
// slice of Nets ready to feed to session.Session.
// For multi-pin nets, consecutive pin pairs are chained and share the same net ID.
func BuildNets(layoutPath, schematicPath string, db common.PinDB) ([]*common.Net, error) {
	var layout layoutFile
	if err := parseJSON(layoutPath, &layout); err != nil {
		return nil, err
	}

	var schematic schematicFile
	if err := parseJSON(schematicPath, &schematic); err != nil {
		return nil, err
	}

	instByName := make(map[string]layoutInstance, len(layout.Instances))
	for _, inst := range layout.Instances {
		instByName[inst.Name] = inst
	}

	// Sort net names for deterministic net ID assignment.
	netNames := make([]string, 0, len(schematic.Nets))
	for name := range schematic.Nets {
		netNames = append(netNames, name)
	}
	sort.Strings(netNames)

	var nets []*common.Net
	for netID, name := range netNames {
		instPins := schematic.Nets[name]
		if len(instPins) < 2 {
			continue
		}

		points := make([]common.Point, len(instPins))
		for i, instPin := range instPins {
			parts := strings.SplitN(instPin, ".", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("netlist: invalid inst.pin %q in net %q", instPin, name)
			}
			instName, pinName := parts[0], parts[1]

			inst, ok := instByName[instName]
			if !ok {
				return nil, fmt.Errorf("netlist: instance %q not found in layout", instName)
			}

			px, py, err := db.Query(inst.Lib, inst.Cell, pinName)
			if err != nil {
				return nil, fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, err)
			}

			points[i] = common.Point{
				X: int(math.Round(inst.XY[0]*1000)) + px,
				Y: int(math.Round(inst.XY[1]*1000)) + py,
			}
		}

		for i := 0; i < len(points)-1; i++ {
			nets = append(nets, &common.Net{
				ID:   netID + 1,
				From: points[i],
				To:   points[i+1],
			})
		}
	}

	return nets, nil
}
