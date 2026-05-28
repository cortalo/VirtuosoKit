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
		if len(expanded) == 0 {
			continue
		} else if len(netNames) == 1 {
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
	IsEscapeCell(lib, cell string) (bool, error)
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

type TerminalInfo struct {
	Layer string        `json:"layer"`
	Bbox  [2][2]float64 `json:"bbox"` // absolute coordinates [[xLow,yLow],[xHigh,yHigh]] in µm
}

type LayoutInstance struct {
	Name      string                  `json:"name"`
	Lib       string                  `json:"lib"`
	Cell      string                  `json:"cell"`
	XY        [2]float64              `json:"xy"`
	Orient    string                  `json:"orient"`
	Terminals map[string]TerminalInfo `json:"terminals,omitempty"`
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
// Pins whose instance lib is in minOverlapLibs have RoutingPin.MinOverlap set to true.
// ignoreLibNets entries have the form "lib:net": pins of that net are skipped only
// when the pin's instance lib matches; the net itself is kept if other libs still
// contribute enough pins.
func BuildNetsFromData(layout Layout, schematic Schematic, db PinDB, ignoreNets, ignoreLibs, minOverlapLibs, ignoreLibNets []string) (lowerLeft, upperRight common.Point, nl *common.Netlist, err error) {
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
	minOverlapLibSet := make(map[string]struct{}, len(minOverlapLibs))
	for _, l := range minOverlapLibs {
		minOverlapLibSet[l] = struct{}{}
	}
	// ignoredLibNetSet: lib → set of net names whose pins from that lib are dropped.
	ignoredLibNetSet := make(map[string]map[string]struct{}, len(ignoreLibNets))
	for _, entry := range ignoreLibNets {
		idx := strings.IndexByte(entry, ':')
		if idx < 0 {
			err = fmt.Errorf("netlist: --ignore-lib-net %q: expected format lib:net", entry)
			return
		}
		lib, net := entry[:idx], entry[idx+1:]
		if ignoredLibNetSet[lib] == nil {
			ignoredLibNetSet[lib] = make(map[string]struct{})
		}
		ignoredLibNetSet[lib][net] = struct{}{}
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

	// filteredNetsMap removes instance pins that are excluded by ignoredLibs or
	// ignoredLibNetSet, so both the net routing loop and the layout pins loop see
	// only connections that are actually routable.
	filteredNetsMap := make(map[string][]string, len(expandedNetsMap))
	for netName, instPins := range expandedNetsMap {
		for _, ip := range instPins {
			parts := strings.SplitN(ip, ".", 2)
			if len(parts) != 2 {
				continue
			}
			lib := schemLib[parts[0]]
			if _, skip := ignoredLibs[lib]; skip {
				continue
			}
			if libNets, ok := ignoredLibNetSet[lib]; ok {
				if _, skip := libNets[netName]; skip {
					continue
				}
			}
			filteredNetsMap[netName] = append(filteredNetsMap[netName], ip)
		}
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
				if nets, ok := ignoredLibNetSet[lib]; ok {
					if _, skip := nets[name]; skip {
						continue
					}
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
			if nets, ok := ignoredLibNetSet[inst.Lib]; ok {
				if _, skip := nets[name]; skip {
					continue
				}
			}
			isEscape, ierr := db.IsEscapeCell(inst.Lib, inst.Cell)
			if ierr != nil {
				err = fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, ierr)
				return
			}
			xLow, xHigh, yLow, yHigh, pinLayer, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
			if qerr != nil {
				if !isEscape {
					err = fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, qerr)
					return
				}
				term, hasTerm := inst.Terminals[parts[1]]
				if !hasTerm {
					err = fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, qerr)
					return
				}
				termLayer, lerr := common.ParseLayer(term.Layer)
				if lerr != nil {
					err = fmt.Errorf("netlist: net %q pin %q: terminal layer %q: %w", name, instPin, term.Layer, lerr)
					return
				}
				_, isMinOverlap := minOverlapLibSet[inst.Lib]
				pins = append(pins, common.RoutingPin{
					Layer:      termLayer,
					XLow:       int(math.Round(term.Bbox[0][0] * 1000)),
					YLow:       int(math.Round(term.Bbox[0][1] * 1000)),
					XHigh:      int(math.Round(term.Bbox[1][0] * 1000)),
					YHigh:      int(math.Round(term.Bbox[1][1] * 1000)),
					MinOverlap: isMinOverlap,
				})
				continue
			}
			instX := int(math.Round(inst.XY[0] * 1000))
			instY := int(math.Round(inst.XY[1] * 1000))
			txLow, txHigh, tyLow, tyHigh := transformPin(xLow, xHigh, yLow, yHigh, parseOrient(inst.Orient))
			_, isMinOverlap := minOverlapLibSet[inst.Lib]
			pins = append(pins, common.RoutingPin{
				Layer:      pinLayer,
				XLow:       instX + txLow,
				XHigh:      instX + txHigh,
				YLow:       instY + tyLow,
				YHigh:      instY + tyHigh,
				MinOverlap: isMinOverlap,
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
			instPinList, ok := filteredNetsMap[name]
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
			isEscape, ierr := db.IsEscapeCell(inst.Lib, inst.Cell)
			if ierr != nil {
				err = fmt.Errorf("netlist: pin %q: %w", name, ierr)
				return
			}
			xLow, xHigh, yLow, yHigh, pinLayer, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
			if qerr != nil {
				if !isEscape {
					err = fmt.Errorf("netlist: pin %q: %w", name, qerr)
					return
				}
				term, hasTerm := inst.Terminals[parts[1]]
				if !hasTerm {
					err = fmt.Errorf("netlist: pin %q: %w", name, qerr)
					return
				}
				termLayer, lerr := common.ParseLayer(term.Layer)
				if lerr != nil {
					err = fmt.Errorf("netlist: pin %q: terminal layer %q: %w", name, term.Layer, lerr)
					return
				}
				layoutPins = append(layoutPins, &common.RoutingPin{
					Name:  name,
					Layer: termLayer,
					XLow:  int(math.Round(term.Bbox[0][0] * 1000)),
					YLow:  int(math.Round(term.Bbox[0][1] * 1000)),
					XHigh: int(math.Round(term.Bbox[1][0] * 1000)),
					YHigh: int(math.Round(term.Bbox[1][1] * 1000)),
				})
				continue
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
func BuildNets(layoutPath, schematicPath string, db PinDB, ignoreNets, ignoreLibs, minOverlapLibs, ignoreLibNets []string) (lowerLeft, upperRight common.Point, nl *common.Netlist, err error) {
	var layout Layout
	if err = parseJSON(layoutPath, &layout); err != nil {
		return
	}
	var schematic Schematic
	if err = parseJSON(schematicPath, &schematic); err != nil {
		return
	}
	return BuildNetsFromData(layout, schematic, db, ignoreNets, ignoreLibs, minOverlapLibs, ignoreLibNets)
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
