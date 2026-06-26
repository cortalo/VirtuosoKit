package netlist

import (
	"encoding/json"
	"fmt"
	"regexp"
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

type PinDir int

const (
	PinDirInput PinDir = iota
	PinDirOutput
	PinDirInputOutput
)

var pinDirName = map[PinDir]string{
	PinDirInput:       "input",
	PinDirOutput:      "output",
	PinDirInputOutput: "inputOutput",
}

var pinDirValue = map[string]PinDir{
	"input":       PinDirInput,
	"output":      PinDirOutput,
	"inputOutput": PinDirInputOutput,
}

func (d PinDir) MarshalJSON() ([]byte, error) {
	s, ok := pinDirName[d]
	if !ok {
		return nil, fmt.Errorf("unknown PinDir %d", d)
	}
	return json.Marshal(s)
}

func (d *PinDir) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, ok := pinDirValue[s]
	if !ok {
		return fmt.Errorf("unknown PinDir %q", s)
	}
	*d = v
	return nil
}

type SchematicInstance struct {
	Name string            `json:"name"`
	Lib  string            `json:"lib"`
	Pins map[string]PinDir `json:"pins,omitempty"`
}

// Schematic is the expanded form of RawSchematic: bus notation resolved,
// instances indexed by name.
type Schematic struct {
	Instances map[string]SchematicInstance
	Nets      map[string][]string
	Pins      map[string]PinDir // top-level port name → direction
}

// Expand resolves bus notation and returns a Schematic with instances indexed by name.
func (r RawSchematic) Expand() (Schematic, error) {
	nets, err := expandNets(r.Nets)
	if err != nil {
		return Schematic{}, err
	}
	pins := make(map[string]PinDir, len(r.Pins))
	for rawName, raw := range r.Pins {
		var entry struct {
			Direction PinDir `json:"direction"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return Schematic{}, fmt.Errorf("netlist: pin %q direction: %w", rawName, err)
		}
		for _, name := range expandNetKey(rawName) {
			pins[name] = entry.Direction
		}
	}
	return Schematic{
		Instances: expandSchematicInstances(r.Instances),
		Nets:      nets,
		Pins:      pins,
	}, nil
}

// Filter returns a new Schematic with excluded nets and inst.pins removed.
// When includeNets is non-empty only those nets are kept and ignoreNets is
// ignored entirely. When includeNets is empty, nets listed in ignoreNets are
// dropped. Individual inst.pins whose instance lib is in ignoreLibs, or whose
// (lib, net) pair is in ignoreLibNets, are removed from their net's connection
// list. Nets in s.Pins (top-level ports) are kept with >= 1 connection; all
// other nets require >= 2 connections to be kept.
func (s Schematic) Filter(includeNets, ignoreNets, ignoreLibs, ignoreLibNets []string) Schematic {
	includeNetsSet := toSet(includeNets)
	ignoreNetsSet := toSet(ignoreNets)
	ignoreLibsSet := toSet(ignoreLibs)
	libNetsMap := parseLibNets(ignoreLibNets)
	nets := make(map[string][]string, len(s.Nets))
	for netName, instPins := range s.Nets {
		if len(includeNetsSet) > 0 {
			if _, keep := includeNetsSet[netName]; !keep {
				continue
			}
		} else if _, skip := ignoreNetsSet[netName]; skip {
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
	return Schematic{Instances: s.Instances, Nets: nets, Pins: s.Pins}
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
	pins := expandBusName(instPin[dot+1:])
	switch {
	case len(insts) == 1 && len(pins) == 1:
		return []string{insts[0] + "." + pins[0]}
	case len(insts) == 1:
		result := make([]string, len(pins))
		for i, p := range pins {
			result[i] = insts[0] + "." + p
		}
		return result
	case len(pins) == 1:
		result := make([]string, len(insts))
		for i, inst := range insts {
			result[i] = inst + "." + pins[0]
		}
		return result
	default:
		// Both expanded: lengths must match (checked later in expandNets).
		result := make([]string, len(insts))
		for i := range insts {
			if i < len(pins) {
				result[i] = insts[i] + "." + pins[i]
			}
		}
		return result
	}
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
			result[name] = SchematicInstance{Name: name, Lib: inst.Lib, Pins: inst.Pins}
		}
	}
	return result
}
