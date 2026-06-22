package netlist

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var busRangeRE = regexp.MustCompile(`^(.*)<(\d+):(\d+)>$`)
var repeatRE = regexp.MustCompile(`^<\*(\d+)>(.+)$`)

// RawSchematic mirrors the schematic JSON structure.
type RawSchematic struct {
	Instances []SchematicInstance        `json:"instances"`
	Nets      map[string][]string        `json:"nets"` // net name → ["inst.pin", ...]
	Pins      map[string]json.RawMessage `json:"pins"` // port names (values ignored)
}

type SchematicInstance struct {
	Name     string `json:"name"`
	CellName string `json:"cell"`
	Lib      string `json:"lib"`
}

// Schematic is the expanded form of RawSchematic: bus notation resolved,
// instances indexed by name.
type Schematic struct {
	Instances map[string]SchematicInstance
	Nets      map[string][]string
	Pins      map[string]json.RawMessage
	PinNames  []string // sorted raw pin keys from RawSchematic.Pins
}

// Expand resolves bus notation and returns a Schematic with instances indexed by name.
func (r RawSchematic) Expand() (Schematic, error) {
	nets, err := expandNets(r.Nets)
	if err != nil {
		return Schematic{}, err
	}
	pinNames := make([]string, 0, len(r.Pins))
	for name := range r.Pins {
		pinNames = append(pinNames, name)
	}
	sort.Strings(pinNames)
	return Schematic{
		Instances: expandSchematicInstances(r.Instances),
		Nets:      nets,
		Pins:      r.Pins,
		PinNames:  pinNames,
	}, nil
}

// Filter returns a new Schematic with excluded nets and inst.pins removed.
// Nets listed in ignoreNets are dropped entirely. Individual inst.pins whose
// instance lib is in ignoreLibs, or whose (lib, net) pair is in ignoreLibNets,
// are removed from their net's connection list.
// Nets in s.Pins (top-level ports) are kept with >= 1 connection; all other
// nets require >= 2 connections to be kept.
func (s Schematic) Filter(ignoreNets, ignoreLibs, ignoreLibNets []string) Schematic {
	ignoreNetsSet := toSet(ignoreNets)
	ignoreLibsSet := toSet(ignoreLibs)
	libNetsMap := parseLibNets(ignoreLibNets)
	nets := make(map[string][]string, len(s.Nets))
	for netName, instPins := range s.Nets {
		if _, skip := ignoreNetsSet[netName]; skip {
			continue
		}
		var filtered []string
		for _, ip := range instPins {
			parts := strings.SplitN(ip, ".", 2)
			if len(parts) != 2 {
				continue
			}
			lib := s.Instances[parts[0]].Lib
			if _, skip := ignoreLibsSet[lib]; skip {
				continue
			}
			if libNets, ok := libNetsMap[lib]; ok {
				if _, skip := libNets[netName]; skip {
					continue
				}
			}
			filtered = append(filtered, ip)
		}
		_, isPin := s.Pins[netName]
		if len(filtered) >= 2 || (isPin && len(filtered) > 0) {
			nets[netName] = filtered
		}
	}
	return Schematic{Instances: s.Instances, Nets: nets, Pins: s.Pins, PinNames: s.PinNames}
}

// parseLibNets converts "lib:net" strings into a map[lib]StringSet.
func parseLibNets(entries []string) map[string]StringSet {
	m := make(map[string]StringSet, len(entries))
	for _, entry := range entries {
		idx := strings.IndexByte(entry, ':')
		if idx < 0 {
			continue
		}
		lib, net := entry[:idx], entry[idx+1:]
		if m[lib] == nil {
			m[lib] = make(StringSet)
		}
		m[lib][net] = struct{}{}
	}
	return m
}

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

func expandSchematicInstances(instances []SchematicInstance) map[string]SchematicInstance {
	result := make(map[string]SchematicInstance)
	for _, inst := range instances {
		for _, name := range expandBusName(inst.Name) {
			result[name] = SchematicInstance{Name: name, CellName: inst.CellName, Lib: inst.Lib}
		}
	}
	return result
}
