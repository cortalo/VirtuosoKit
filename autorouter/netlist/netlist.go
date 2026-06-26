package netlist

import (
	"autorouter/common"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// StringSet is a set of strings.
type StringSet = map[string]struct{}

type PinDB interface {
	Query(lib, cell, pin string) (xLow, xHigh, yLow, yHigh common.Nm, layer common.Layer, err error)
}

func toSet(ss []string) StringSet {
	s := make(StringSet, len(ss))
	for _, v := range ss {
		s[v] = struct{}{}
	}
	return s
}

func buildNets(layout Layout, schematic Schematic, db PinDB, minOverlapLibs []string, includePortNets bool) ([]*common.Net, error) {
	minOverlapSet := toSet(minOverlapLibs)
	netNames := make([]string, 0, len(schematic.Nets))
	for name := range schematic.Nets {
		netNames = append(netNames, name)
	}
	sort.Strings(netNames)

	var nets []*common.Net
	for netID, name := range netNames {
		instPins := schematic.Nets[name]
		var pins []common.RoutingPin
		for _, instPin := range instPins {
			parts := strings.SplitN(instPin, ".", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("netlist: invalid inst.pin %q in net %q", instPin, name)
			}
			inst, ok := layout.Instances[parts[0]]
			if !ok {
				return nil, fmt.Errorf("netlist: instance %q not found in layout", parts[0])
			}
			_, isMinOverlap := minOverlapSet[inst.Lib]
			xLow, xHigh, yLow, yHigh, pinLayer, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
			if qerr != nil {
				if !errors.Is(qerr, common.ErrPinNotFound) {
					return nil, fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, qerr)
				}
				term, hasTerm := inst.Terminals[parts[1]]
				if !hasTerm {
					return nil, fmt.Errorf("netlist: net %q pin %q: %w", name, instPin, qerr)
				}
				termLayer, lerr := common.ParseLayer(term.Layer)
				if lerr != nil {
					return nil, fmt.Errorf("netlist: net %q pin %q: terminal layer %q: %w", name, instPin, term.Layer, lerr)
				}
				pins = append(pins, common.RoutingPin{
					Name:       instPin,
					Layer:      termLayer,
					XLow:       common.Micron(term.Bbox[0][0]).ToNm(),
					YLow:       common.Micron(term.Bbox[0][1]).ToNm(),
					XHigh:      common.Micron(term.Bbox[1][0]).ToNm(),
					YHigh:      common.Micron(term.Bbox[1][1]).ToNm(),
					MinOverlap: isMinOverlap,
				})
				continue
			}
			instX := common.Micron(inst.XY[0]).ToNm()
			instY := common.Micron(inst.XY[1]).ToNm()
			txLow, txHigh, tyLow, tyHigh := transformPin(xLow, xHigh, yLow, yHigh, parseOrient(inst.Orient))
			pins = append(pins, common.RoutingPin{
				Name:       instPin,
				Layer:      pinLayer,
				XLow:       instX + txLow,
				XHigh:      instX + txHigh,
				YLow:       instY + tyLow,
				YHigh:      instY + tyHigh,
				MinOverlap: isMinOverlap,
			})
		}
		_, isPort := schematic.Pins[name]
		if len(pins) == 0 || (len(pins) < 2 && !(includePortNets && isPort)) {
			continue
		}
		nets = append(nets, &common.Net{
			ID:   netID + 1,
			Name: name,
			Pins: pins,
		})
	}
	return nets, nil
}

func buildPins(layout Layout, schematic Schematic, db PinDB, ignoreNets []string) ([]*common.RoutingPin, error) {
	ignoreNetsSet := toSet(ignoreNets)
	portNames := make([]string, 0, len(schematic.Pins))
	for name := range schematic.Pins {
		portNames = append(portNames, name)
	}
	sort.Strings(portNames)
	var layoutPins []*common.RoutingPin
	for _, name := range portNames {
		if _, skip := ignoreNetsSet[name]; skip {
			continue
		}
		instPinList, ok := schematic.Nets[name]
		if !ok || len(instPinList) == 0 {
			continue
		}
		parts := strings.SplitN(instPinList[0], ".", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("netlist: invalid inst.pin %q in pin %q", instPinList[0], name)
		}
		inst, ok := layout.Instances[parts[0]]
		if !ok {
			return nil, fmt.Errorf("netlist: instance %q not found in layout for pin %q", parts[0], name)
		}
		xLow, xHigh, yLow, yHigh, pinLayer, qerr := db.Query(inst.Lib, inst.Cell, parts[1])
		if qerr != nil {
			if !errors.Is(qerr, common.ErrPinNotFound) {
				return nil, fmt.Errorf("netlist: pin %q: %w", name, qerr)
			}
			term, hasTerm := inst.Terminals[parts[1]]
			if !hasTerm {
				return nil, fmt.Errorf("netlist: pin %q: %w", name, qerr)
			}
			termLayer, lerr := common.ParseLayer(term.Layer)
			if lerr != nil {
				return nil, fmt.Errorf("netlist: pin %q: terminal layer %q: %w", name, term.Layer, lerr)
			}
			layoutPins = append(layoutPins, &common.RoutingPin{
				Name:  name,
				Layer: termLayer,
				XLow:  common.Micron(term.Bbox[0][0]).ToNm(),
				YLow:  common.Micron(term.Bbox[0][1]).ToNm(),
				XHigh: common.Micron(term.Bbox[1][0]).ToNm(),
				YHigh: common.Micron(term.Bbox[1][1]).ToNm(),
			})
			continue
		}
		instX := common.Micron(inst.XY[0]).ToNm()
		instY := common.Micron(inst.XY[1]).ToNm()
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
	return layoutPins, nil
}

// BuildNetsFromData builds a Netlist from already-parsed layout and schematic data.
// When includeNets is non-empty only those nets are routed and ignoreNets is ignored.
// When includeNets is empty, nets in ignoreNets are skipped.
// Pins whose instance belongs to a lib in ignoreLibs are skipped; a net with
// fewer than 2 remaining pins is dropped.
// Pins whose instance lib is in minOverlapLibs have RoutingPin.MinOverlap set to true.
// ignoreLibNets is a list of "lib:net" pairs: pins of those nets are skipped only
// when the pin's instance lib matches.
func BuildNetsFromData(rawLayout RawLayout, rawSchematic RawSchematic, db PinDB, includeNets, ignoreNets, ignoreLibs, minOverlapLibs, ignoreLibNets []string, includePortNets bool) (nl *common.Netlist, err error) {
	schematic, err := rawSchematic.Expand()
	if err != nil {
		return
	}
	schematic, err = schematic.Filter(includeNets, ignoreNets, ignoreLibs, ignoreLibNets)
	if err != nil {
		return
	}
	layout := rawLayout.Index()

	nets, err := buildNets(layout, schematic, db, minOverlapLibs, includePortNets)
	if err != nil {
		return
	}

	layoutPins, err := buildPins(layout, schematic, db, ignoreNets)
	if err != nil {
		return
	}

	nl = &common.Netlist{Nets: nets, Pins: layoutPins}
	return
}

// LoadFiles parses layout and schematic from JSON files.
func LoadFiles(layoutPath, schematicPath string) (layout RawLayout, schematic RawSchematic, err error) {
	if err = parseJSON(layoutPath, &layout); err != nil {
		return
	}
	err = parseJSON(schematicPath, &schematic)
	return
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
