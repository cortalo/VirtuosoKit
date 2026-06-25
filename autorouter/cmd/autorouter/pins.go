package main

import (
	"autorouter/common"
	"encoding/json"
)

type pinEntry struct {
	Name  string        `json:"name"`
	Layer common.Layer  `json:"layer"`
	BBox  [2][2]float64 `json:"bbox"` // [[xLow, yLow], [xHigh, yHigh]] in µm
}

func collectPins(nl *common.Netlist) []pinEntry {
	var entries []pinEntry
	for _, net := range nl.Nets {
		for _, p := range net.Pins {
			entries = append(entries, pinEntry{
				Name:  p.Name,
				Layer: p.Layer,
				BBox: [2][2]float64{
					{float64(p.XLow) / 1000, float64(p.YLow) / 1000},
					{float64(p.XHigh) / 1000, float64(p.YHigh) / 1000},
				},
			})
		}
	}
	return entries
}

func writePinsJSON(nl *common.Netlist) ([]byte, error) {
	return json.MarshalIndent(collectPins(nl), "", "  ")
}

// writeNetConnectionsJSON returns a JSON object mapping each net name to the
// list of instance terminal names (inst.pin format) that belong to that net.
// This lets the Python caller resolve which terminal to use when auto-placing
// a top-level port pin, using Go's fully-resolved netlist rather than the
// primitive Python schematic reader.
func writeNetConnectionsJSON(nl *common.Netlist) ([]byte, error) {
	connections := make(map[string][]string, len(nl.Nets))
	for _, net := range nl.Nets {
		terminals := make([]string, 0, len(net.Pins))
		for _, p := range net.Pins {
			terminals = append(terminals, p.Name)
		}
		if len(terminals) > 0 {
			connections[net.Name] = terminals
		}
	}
	return json.MarshalIndent(connections, "", "  ")
}
