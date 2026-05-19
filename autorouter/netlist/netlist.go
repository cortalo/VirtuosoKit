package netlist

import (
	"autorouter/common"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type PinDB interface {
	Query(lib, cell, pin string) (xLow, yLow, yHigh int, err error)
}

// Layout mirrors the layout JSON structure.
type Layout struct {
	Shapes    []LayoutShape    `json:"shapes"`
	Instances []LayoutInstance `json:"instances"`
}

type LayoutShape struct {
	Layer string        `json:"layer"`
	BBox  [2][2]float64 `json:"bbox"`
}

type LayoutInstance struct {
	Name string     `json:"name"`
	Lib  string     `json:"lib"`
	Cell string     `json:"cell"`
	XY   [2]float64 `json:"xy"`
}

// Schematic mirrors the schematic JSON structure.
type Schematic struct {
	Instances []SchematicInstance `json:"instances"`
	Nets      map[string][]string `json:"nets"` // net name → ["inst.pin", ...]
}

type SchematicInstance struct {
	Name string `json:"name"`
	Lib  string `json:"lib"`
}

var ErrNoPRBoundary = errors.New("netlist: prBoundary shape not found in layout")

func prBoundary(shapes []LayoutShape) (lowerLeft, upperRight common.Point, err error) {
	for _, s := range shapes {
		if s.Layer == "prBoundary" {
			return common.Point{
					X: int(math.Round(s.BBox[0][0] * 1000)),
					Y: int(math.Round(s.BBox[0][1] * 1000)),
				}, common.Point{
					X: int(math.Round(s.BBox[1][0] * 1000)),
					Y: int(math.Round(s.BBox[1][1] * 1000)),
				}, nil
		}
	}
	return common.Point{}, common.Point{}, ErrNoPRBoundary
}

// BuildNetsFromData builds nets from already-parsed layout and schematic data.
// It also returns the lower-left and upper-right corners of the prBoundary shape,
// converted to nm. For multi-pin nets, consecutive pin pairs are chained and share
// the same net ID. Nets in ignoreNets and pins whose instance belongs to a lib in
// ignoreLibs are skipped; a net with fewer than 2 remaining pins is dropped.
func BuildNetsFromData(layout Layout, schematic Schematic, db PinDB, ignoreNets, ignoreLibs []string) (lowerLeft, upperRight common.Point, nets []*common.Net, err error) {
	lowerLeft, upperRight, err = prBoundary(layout.Shapes)
	if err != nil {
		return
	}

	ignoredNets := make(map[string]struct{}, len(ignoreNets))
	for _, n := range ignoreNets {
		ignoredNets[n] = struct{}{}
	}
	ignoredLibs := make(map[string]struct{}, len(ignoreLibs))
	for _, l := range ignoreLibs {
		ignoredLibs[l] = struct{}{}
	}

	schemLib := make(map[string]string, len(schematic.Instances))
	for _, inst := range schematic.Instances {
		schemLib[inst.Name] = inst.Lib
	}

	instByName := make(map[string]LayoutInstance, len(layout.Instances))
	for _, inst := range layout.Instances {
		instByName[inst.Name] = inst
	}

	netNames := make([]string, 0, len(schematic.Nets))
	for name := range schematic.Nets {
		netNames = append(netNames, name)
	}
	sort.Strings(netNames)

	for netID, name := range netNames {
		if _, skip := ignoredNets[name]; skip {
			continue
		}
		instPins := schematic.Nets[name]

		var pins []common.RoutingPin
		for _, instPin := range instPins {
			parts := strings.SplitN(instPin, ".", 2)
			if len(parts) != 2 {
				err = fmt.Errorf("netlist: invalid inst.pin %q in net %q", instPin, name)
				return
			}
			if lib, found := schemLib[parts[0]]; found {
				if _, skip := ignoredLibs[lib]; skip {
					continue
				}
			}
			inst, ok := instByName[parts[0]]
			if !ok {
				err = fmt.Errorf("netlist: instance %q not found in layout", parts[0])
				return
			}
			if _, skip := ignoredLibs[inst.Lib]; skip {
				continue
			}
			xLow, yLow, yHigh, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
			if qerr != nil {
				err = fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, qerr)
				return
			}
			instX := int(math.Round(inst.XY[0] * 1000))
			instY := int(math.Round(inst.XY[1] * 1000))
			pins = append(pins, common.RoutingPin{
				XLow:  instX + xLow,
				YLow:  instY + yLow,
				YHigh: instY + yHigh,
			})
		}
		if len(pins) < 2 {
			continue
		}

		for i := 0; i < len(pins)-1; i++ {
			nets = append(nets, &common.Net{
				ID:   netID + 1,
				From: pins[i],
				To:   pins[i+1],
			})
		}
	}

	return
}

// BuildNets loads layout and schematic from JSON files and calls BuildNetsFromData.
func BuildNets(layoutPath, schematicPath string, db PinDB, ignoreNets, ignoreLibs []string) (lowerLeft, upperRight common.Point, nets []*common.Net, err error) {
	var layout Layout
	if err = parseJSON(layoutPath, &layout); err != nil {
		return
	}
	var schematic Schematic
	if err = parseJSON(schematicPath, &schematic); err != nil {
		return
	}
	return BuildNetsFromData(layout, schematic, db, ignoreNets, ignoreLibs)
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
