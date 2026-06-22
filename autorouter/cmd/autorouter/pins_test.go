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
// builds the netlist, and verifies the pin JSON output.
//
// With ignoreNets=[VDD, VSS], only net1 survives:
//   - I1.VOUT: terminal bbox [[4.0, 6.22], [4.23, 8.14]] in µm, layer M1
//   - I0.VIN:  terminal bbox [[8.09, 4.365], [8.32, 4.595]] in µm, layer M1
func TestWritePinsJSON_InvChain(t *testing.T) {
	layout, schem, err := netlist.LoadFiles(
		"../../netlist/testdata/inv2_terminal_layout.json",
		"../../netlist/testdata/inv2_terminal_schematic.json",
	)
	require.NoError(t, err)

	nl, err := netlist.BuildNetsFromData(layout, schem, &emptyDB{}, []string{"VDD", "VSS"}, nil, nil, nil)
	require.NoError(t, err)

	data, err := writePinsJSON(nl)
	require.NoError(t, err)

	var pins []pinEntry
	require.NoError(t, json.Unmarshal(data, &pins))

	// net1 has exactly 2 pins
	require.Len(t, pins, 2)

	byName := make(map[string]pinEntry, len(pins))
	for _, p := range pins {
		byName[p.Name] = p
	}

	t.Run("I1.VOUT", func(t *testing.T) {
		p, ok := byName["I1.VOUT"]
		require.True(t, ok, "I1.VOUT missing from output")
		assert.Equal(t, common.M1, p.Layer)
		assert.Equal(t, [2][2]float64{{4.0, 6.22}, {4.23, 8.14}}, p.BBox)
	})

	t.Run("I0.VIN", func(t *testing.T) {
		p, ok := byName["I0.VIN"]
		require.True(t, ok, "I0.VIN missing from output")
		assert.Equal(t, common.M1, p.Layer)
		assert.Equal(t, [2][2]float64{{8.09, 4.365}, {8.32, 4.595}}, p.BBox)
	})
}
