package netlist

import (
	"autorouter/common"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var busRangeRE = regexp.MustCompile(`^(.*)<(\d+):(\d+)>$`)
var repeatRE = regexp.MustCompile(`^<\*(\d+)>(.+)$`)

// expandBusName expands "NAME<high:low>" into individual bit names.
// Returns a single-element slice if no bus syntax is present.
func expandBusName(name string) []string {
	m := busRangeRE.FindStringSubmatch(name)
	if m == nil {
		return []string{name}
	}
	base := m[1]
	high, _ := strconv.Atoi(m[2])
	low, _ := strconv.Atoi(m[3])
	var result []string
	if high >= low {
		for i := high; i >= low; i-- {
			result = append(result, fmt.Sprintf("%s<%d>", base, i))
		}
	} else {
		for i := high; i <= low; i++ {
			result = append(result, fmt.Sprintf("%s<%d>", base, i))
		}
	}
	return result
}

// expandNetKey expands a net key into individual net names, handling:
//   - "<*N>NAME"    → NAME repeated N times
//   - "A<h:l>,B"   → per-bit names for A, then B (comma-separated, each may be a bus)
func expandNetKey(key string) []string {
	if m := repeatRE.FindStringSubmatch(key); m != nil {
		n, _ := strconv.Atoi(m[1])
		name := m[2]
		result := make([]string, n)
		for i := range result {
			result[i] = name
		}
		return result
	}
	parts := strings.Split(key, ",")
	var result []string
	for _, p := range parts {
		result = append(result, expandBusName(strings.TrimSpace(p))...)
	}
	return result
}

// expandInstPin expands "INST<h:l>.PIN" into individual inst.pin strings.
func expandInstPin(instPin string) []string {
	dot := strings.LastIndex(instPin, ".")
	if dot < 0 {
		return []string{instPin}
	}
	insts := expandBusName(instPin[:dot])
	pin := instPin[dot:]
	result := make([]string, len(insts))
	for i, inst := range insts {
		result[i] = inst + pin
	}
	return result
}

// expandNets flattens bus-notation net entries into a plain net→instPins map.
// A scalar net key (1 name) may connect to multiple inst.pins (all appended to
// that one net). A bus key (N>1 names) must match 1:1 with the expanded inst.pins.
func expandNets(raw map[string][]string) (map[string][]string, error) {
	result := make(map[string][]string)
	for key, instPins := range raw {
		netNames := expandNetKey(key)
		var expanded []string
		for _, ip := range instPins {
			expanded = append(expanded, expandInstPin(ip)...)
		}
		if len(netNames) == 1 {
			result[netNames[0]] = append(result[netNames[0]], expanded...)
		} else if len(netNames) == len(expanded) {
			for i, name := range netNames {
				result[name] = append(result[name], expanded[i])
			}
		} else {
			return nil, fmt.Errorf("netlist: net %q: %d net names vs %d inst.pins", key, len(netNames), len(expanded))
		}
	}
	return result, nil
}

// expandSchematicInstances flattens bus instance names into individual instances.
func expandSchematicInstances(instances []SchematicInstance) []SchematicInstance {
	var result []SchematicInstance
	for _, inst := range instances {
		for _, name := range expandBusName(inst.Name) {
			result = append(result, SchematicInstance{Name: name, Lib: inst.Lib})
		}
	}
	return result
}

type PinDB interface {
	Query(lib, cell, pin string) (xLow, xHigh, yLow, yHigh int, layer common.Layer, err error)
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

	expandedInsts := expandSchematicInstances(schematic.Instances)
	expandedNetsMap, expErr := expandNets(schematic.Nets)
	if expErr != nil {
		err = expErr
		return
	}

	schemLib := make(map[string]string, len(expandedInsts))
	for _, inst := range expandedInsts {
		schemLib[inst.Name] = inst.Lib
	}

	instByName := make(map[string]LayoutInstance, len(layout.Instances))
	for _, inst := range layout.Instances {
		instByName[inst.Name] = inst
	}

	netNames := make([]string, 0, len(expandedNetsMap))
	for name := range expandedNetsMap {
		netNames = append(netNames, name)
	}
	sort.Strings(netNames)

	var nets []*common.Net
	for netID, name := range netNames {
		if _, skip := ignoredNets[name]; skip {
			continue
		}
		instPins := expandedNetsMap[name]

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
			xLow, xHigh, yLow, yHigh, pinLayer, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
			if qerr != nil {
				err = fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, qerr)
				return
			}
			instX := int(math.Round(inst.XY[0] * 1000))
			instY := int(math.Round(inst.XY[1] * 1000))
			txLow, txHigh, tyLow, tyHigh := transformPin(xLow, xHigh, yLow, yHigh, parseOrient(inst.Orient))
			pins = append(pins, common.RoutingPin{
				Layer: pinLayer,
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
			Name: name,
			Pins: pins,
		})
	}

	rawPinNames := make([]string, 0, len(schematic.Pins))
	for name := range schematic.Pins {
		rawPinNames = append(rawPinNames, name)
	}
	sort.Strings(rawPinNames)

	var layoutPins []*common.RoutingPin
	for _, rawName := range rawPinNames {
		for _, name := range expandNetKey(rawName) {
			if _, skip := ignoredNets[name]; skip {
				continue
			}
			instPinList, ok := expandedNetsMap[name]
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
			xLow, xHigh, yLow, yHigh, pinLayer, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
			if qerr != nil {
				err = fmt.Errorf("netlist: pin %q: %w", name, qerr)
				return
			}
			instX := int(math.Round(inst.XY[0] * 1000))
			instY := int(math.Round(inst.XY[1] * 1000))
			txLow, txHigh, tyLow, tyHigh := transformPin(xLow, xHigh, yLow, yHigh, parseOrient(inst.Orient))
			layoutPins = append(layoutPins, &common.RoutingPin{
				Name:  name,
				Layer: pinLayer,
				XLow:  instX + txLow,
				XHigh: instX + txHigh,
				YLow:  instY + tyLow,
				YHigh: instY + tyHigh,
			})
		}
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
