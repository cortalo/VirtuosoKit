package main

import (
	"autorouter/common"
	"autorouter/netlist"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWritePinsJSON_InvChain loads the inv2_terminal test data (ignoring VDD/VSS),
// builds the netlist with includePortNets=true, and verifies all pin JSON entries.
//
// Expected pins (4 total):
//   - net1:  I1.VOUT [[4.0,6.22],[4.23,8.14]] M1   I0.VIN [[8.09,4.365],[8.32,4.595]] M1
//   - VIN:   I1.VIN  [[2.09,4.365],[2.32,4.595]] M1
//   - VOUT:  I0.VOUT [[10.0,6.22],[10.23,8.14]] M1
func TestWritePinsJSON_InvChain(t *testing.T) {
	layout, schem, err := netlist.LoadFiles(
		"../../netlist/testdata/inv2_terminal_layout.json",
		"../../netlist/testdata/inv2_terminal_schematic.json",
	)
	require.NoError(t, err)

	nl, err := netlist.BuildNetsFromData(layout, schem, &emptyDB{}, []string{"VDD", "VSS"}, nil, nil, nil, true)
	require.NoError(t, err)

	data, err := writePinsJSON(nl)
	require.NoError(t, err)

	var pins []pinEntry
	require.NoError(t, json.Unmarshal(data, &pins))
	require.Len(t, pins, 4)

	byName := make(map[string]pinEntry, len(pins))
	for _, p := range pins {
		byName[p.Name] = p
	}

	cases := []struct {
		name string
		bbox [2][2]float64
	}{
		{"I1.VOUT", [2][2]float64{{4.0, 6.22}, {4.23, 8.14}}},
		{"I0.VIN", [2][2]float64{{8.09, 4.365}, {8.32, 4.595}}},
		{"I1.VIN", [2][2]float64{{2.09, 4.365}, {2.32, 4.595}}},
		{"I0.VOUT", [2][2]float64{{10.0, 6.22}, {10.23, 8.14}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, ok := byName[c.name]
			require.True(t, ok, "%s missing from output", c.name)
			assert.Equal(t, common.M1, p.Layer)
			assert.Equal(t, c.bbox, p.BBox)
		})
	}
}
