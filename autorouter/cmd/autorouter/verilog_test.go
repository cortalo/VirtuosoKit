package main

import (
	"autorouter/common"
	"autorouter/netlist"
	"testing"
)

func TestWriteVerilog(t *testing.T) {
	tests := []struct {
		name       string
		moduleName string
		nl         *common.Netlist
		want       string
	}{
		{
			name:       "empty netlist",
			moduleName: "route",
			nl:         &common.Netlist{},
			want: `module route ();
endmodule
`,
		},
		{
			name:       "net with no driver is skipped",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					{Name: "net1", Driver: "", Pins: []common.RoutingPin{
						{Name: "I0.ZN"},
						{Name: "I1.I"},
					}},
				},
			},
			want: `module route ();
endmodule
`,
		},
		{
			// Bus indices <N> in driver, sink, and port-net names must be rewritten
			// to _N so the output is legal Verilog (angle brackets are not valid identifiers).
			name:       "bus indices rewritten to underscores",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					// driver and sink both carry bus notation in pin part
					{Name: "net1", Driver: "I0.s<0>", Pins: []common.RoutingPin{
						{Name: "I0.s<0>"},
						{Name: "I1.a<0>"},
					}},
					// net name itself is a bus bit and is a top-level port
					{Name: "WIDTH<0>", Driver: "I2.width<0>", Pins: []common.RoutingPin{
						{Name: "I2.width<0>"},
					}},
				},
				Pins: []*common.RoutingPin{
					{Name: "WIDTH<0>"},
				},
			},
			want: `module route (
    input  I0_s_0,
    input  I2_width_0,
    output I1_a_0,
    output WIDTH_0
);

    assign I1_a_0 = I0_s_0;
    assign WIDTH_0 = I2_width_0;

endmodule
`,
		},
		{
			name:       "single net one sink",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					{Name: "net1", Driver: "I0.ZN", Pins: []common.RoutingPin{
						{Name: "I0.ZN"},
						{Name: "I1.I"},
					}},
				},
			},
			want: `module route (
    input  I0_ZN,
    output I1_I
);

    assign I1_I = I0_ZN;

endmodule
`,
		},
		{
			name:       "single net multiple sinks",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					{Name: "net1", Driver: "I0.ZN", Pins: []common.RoutingPin{
						{Name: "I0.ZN"},
						{Name: "I1.I"},
						{Name: "I2.I"},
					}},
				},
			},
			want: `module route (
    input  I0_ZN,
    output I1_I,
    output I2_I
);

    assign I1_I = I0_ZN;
    assign I2_I = I0_ZN;

endmodule
`,
		},
		{
			// MID is an internal net (2 instance pins) that is also a top-level port.
			// Both I0_I (instance sink) and MID (port) must appear as outputs,
			// each with their own assign statement.
			name:       "internal net also exposed as top-level port",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					{Name: "MID", Driver: "I1.ZN", Pins: []common.RoutingPin{
						{Name: "I1.ZN"},
						{Name: "I0.I"},
					}},
				},
				Pins: []*common.RoutingPin{
					{Name: "MID"},
				},
			},
			want: `module route (
    input  I1_ZN,
    output I0_I,
    output MID
);

    assign I0_I = I1_ZN;
    assign MID = I1_ZN;

endmodule
`,
		},
		{
			name:       "multiple nets",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					{Name: "net1", Driver: "I0.ZN", Pins: []common.RoutingPin{
						{Name: "I0.ZN"},
						{Name: "I1.I"},
					}},
					{Name: "net2", Driver: "I1.ZN", Pins: []common.RoutingPin{
						{Name: "I1.ZN"},
						{Name: "I2.I"},
					}},
				},
			},
			want: `module route (
    input  I0_ZN,
    input  I1_ZN,
    output I1_I,
    output I2_I
);

    assign I1_I = I0_ZN;
    assign I2_I = I1_ZN;

endmodule
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writeVerilog(tt.moduleName, tt.nl)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestWriteVerilog_InvChain is an end-to-end test: load real layout+schematic,
// build a Netlist, then verify the Verilog output.
//
// Topology: I1.VOUT (output) drives I0.VIN (input) via net1.
// VDD/VSS are ignored (no output driver on those nets yet).
//
// Expected module:
//
//	module inv_chain (
//	    input  I1_VOUT,
//	    output I0_VIN
//	);
//
//	    assign I0_VIN = I1_VOUT;
//
//	endmodule
func TestWriteVerilog_InvChain(t *testing.T) {
	layout, schem, err := netlist.LoadFiles(
		"../../netlist/testdata/inv2_terminal_layout.json",
		"../../netlist/testdata/inv2_terminal_schematic.json",
	)
	if err != nil {
		t.Fatalf("load files: %v", err)
	}

	db := &emptyDB{}
	nl, err := netlist.BuildNetsFromData(layout, schem, db, []string{"VDD", "VSS"}, nil, nil, nil, true)
	if err != nil {
		t.Fatalf("build nets: %v", err)
	}

	got := writeVerilog("inv_chain", nl)
	// With includePortNets=true:
	//   net1:  I1.VOUT (driver) → I0.VIN        →  assign I0_VIN = I1_VOUT
	//   VIN:   "VIN" (top-level input port)  → I1.VIN  →  assign I1_VIN = VIN
	//   VOUT:  I0.VOUT (driver, only inst pin) → VOUT (port) →  assign VOUT = I0_VOUT
	want := `module inv_chain (
    input  I0_VOUT,
    input  I1_VOUT,
    input  VIN,
    output I0_VIN,
    output I1_VIN,
    output VOUT
);

    assign I0_VIN = I1_VOUT;
    assign I1_VIN = VIN;
    assign VOUT = I0_VOUT;

endmodule
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// emptyDB satisfies netlist.PinDB with no entries, forcing terminal fallback.
type emptyDB struct{}

func (e *emptyDB) Query(lib, cell, pin string) (int, int, int, int, common.Layer, error) {
	return 0, 0, 0, 0, 0, common.ErrPinNotFound
}
