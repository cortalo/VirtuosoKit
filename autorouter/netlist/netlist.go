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
	Query(lib, cell, pin string) (xLow, xHigh, yLow, yHigh int, err error)
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
	Name   string     `json:"name"`
	Lib    string     `json:"lib"`
	Cell   string     `json:"cell"`
	XY     [2]float64 `json:"xy"`
	Orient string     `json:"orient"`
}

// Schematic mirrors the schematic JSON structure.
type Schematic struct {
	Instances []SchematicInstance        `json:"instances"`
	Nets      map[string][]string        `json:"nets"` // net name → ["inst.pin", ...]
	Pins      map[string]json.RawMessage `json:"pins"` // port names (values ignored)
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

// parseOrient strips surrounding double-quotes that Virtuoso adds to orient strings
// (e.g. `"\"MX\""` → `"MX"`) and returns the canonical orientation token.
func parseOrient(s string) string {
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimSuffix(s, "\"")
	return s
}

// transformPin applies an orientation transform to a pin bbox relative to the cell origin.
// Covers all eight Cadence orientations.
func transformPin(xLow, xHigh, yLow, yHigh int, orient string) (int, int, int, int) {
	switch orient {
	case "R90": // 90° CCW: (x,y) → (-y, x)
		return -yHigh, -yLow, xLow, xHigh
	case "R180": // 180°: (x,y) → (-x,-y)
		return -xHigh, -xLow, -yHigh, -yLow
	case "R270": // 270° CCW: (x,y) → (y,-x)
		return yLow, yHigh, -xHigh, -xLow
	case "MX": // mirror X axis: (x,y) → (x,-y)
		return xLow, xHigh, -yHigh, -yLow
	case "MY": // mirror Y axis: (x,y) → (-x,y)
		return -xHigh, -xLow, yLow, yHigh
	case "MXR90": // MX then R90: (x,y) → (y,x)
		return yLow, yHigh, xLow, xHigh
	case "MYR90": // MY then R90: (x,y) → (-y,-x)
		return -yHigh, -yLow, -xHigh, -xLow
	default: // R0 and anything unrecognised
		return xLow, xHigh, yLow, yHigh
	}
}

// BuildNetsFromData builds a Netlist from already-parsed layout and schematic data.
// It also returns the lower-left and upper-right corners of the prBoundary shape,
// converted to nm. For multi-pin nets, consecutive pin pairs are chained and share
// the same net ID. Nets in ignoreNets and pins whose instance belongs to a lib in
// ignoreLibs are skipped; a net with fewer than 2 remaining pins is dropped.
func BuildNetsFromData(layout Layout, schematic Schematic, db PinDB, ignoreNets, ignoreLibs []string) (lowerLeft, upperRight common.Point, nl *common.Netlist, err error) {
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

	var nets []*common.Net
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
			xLow, xHigh, yLow, yHigh, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
			if qerr != nil {
				err = fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, qerr)
				return
			}
			instX := int(math.Round(inst.XY[0] * 1000))
			instY := int(math.Round(inst.XY[1] * 1000))
			txLow, txHigh, tyLow, tyHigh := transformPin(xLow, xHigh, yLow, yHigh, parseOrient(inst.Orient))
			pins = append(pins, common.RoutingPin{
				XLow:  instX + txLow,
				XHigh: instX + txHigh,
				YLow:  instY + tyLow,
				YHigh: instY + tyHigh,
			})
		}
		if len(pins) < 2 {
			continue
		}
		nets = append(nets, &common.Net{
			ID:   netID + 1,
			Pins: pins,
		})
	}

	pinNames := make([]string, 0, len(schematic.Pins))
	for name := range schematic.Pins {
		pinNames = append(pinNames, name)
	}
	sort.Strings(pinNames)

	var layoutPins []*common.RoutingPin
	for _, name := range pinNames {
		if _, skip := ignoredNets[name]; skip {
			continue
		}
		instPinList, ok := schematic.Nets[name]
		if !ok || len(instPinList) == 0 {
			continue
		}
		parts := strings.SplitN(instPinList[0], ".", 2)
		if len(parts) != 2 {
			err = fmt.Errorf("netlist: invalid inst.pin %q in pin %q", instPinList[0], name)
			return
		}
		inst, ok := instByName[parts[0]]
		if !ok {
			err = fmt.Errorf("netlist: instance %q not found in layout for pin %q", parts[0], name)
			return
		}
		xLow, xHigh, yLow, yHigh, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
		if qerr != nil {
			err = fmt.Errorf("netlist: pin %q: %w", name, qerr)
			return
		}
		instX := int(math.Round(inst.XY[0] * 1000))
		instY := int(math.Round(inst.XY[1] * 1000))
		txLow, txHigh, tyLow, tyHigh := transformPin(xLow, xHigh, yLow, yHigh, parseOrient(inst.Orient))
		layoutPins = append(layoutPins, &common.RoutingPin{
			Name:  name,
			XLow:  instX + txLow,
			XHigh: instX + txHigh,
			YLow:  instY + tyLow,
			YHigh: instY + tyHigh,
		})
	}

	nl = &common.Netlist{Nets: nets, Pins: layoutPins}
	return
}

// BuildNets loads layout and schematic from JSON files and calls BuildNetsFromData.
func BuildNets(layoutPath, schematicPath string, db PinDB, ignoreNets, ignoreLibs []string) (lowerLeft, upperRight common.Point, nl *common.Netlist, err error) {
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
