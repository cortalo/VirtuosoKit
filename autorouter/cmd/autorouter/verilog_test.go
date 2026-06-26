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
			name:       "single net one sink",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					{Name: "net1", Pins: []common.RoutingPin{
						{Name: "I0.ZN"},
						{Name: "I1.I"},
					}},
				},
			},
			want: `module route (
    inout I0_ZN,
    inout I1_I
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
					{Name: "net1", Pins: []common.RoutingPin{
						{Name: "I0.ZN"},
						{Name: "I1.I"},
						{Name: "I2.I"},
					}},
				},
			},
			want: `module route (
    inout I0_ZN,
    inout I1_I,
    inout I2_I
);

    assign I1_I = I0_ZN;
    assign I2_I = I0_ZN;

endmodule
`,
		},
		{
			// MID is an internal net (2 instance pins) that is also a top-level port.
			// Port name is appended after instance pins; first pin drives all others.
			name:       "internal net also exposed as top-level port",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					{Name: "MID", Pins: []common.RoutingPin{
						{Name: "I1.ZN"},
						{Name: "I0.I"},
					}},
				},
				Pins: []*common.RoutingPin{
					{Name: "MID"},
				},
			},
			want: `module route (
    inout I0_I,
    inout I1_ZN,
    inout MID
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
					{Name: "net1", Pins: []common.RoutingPin{
						{Name: "I0.ZN"},
						{Name: "I1.I"},
					}},
					{Name: "net2", Pins: []common.RoutingPin{
						{Name: "I1.ZN"},
						{Name: "I2.I"},
					}},
				},
			},
			want: `module route (
    inout I0_ZN,
    inout I1_I,
    inout I1_ZN,
    inout I2_I
);

    assign I1_I = I0_ZN;
    assign I2_I = I1_ZN;

endmodule
`,
		},
		{
			// Bus indices <N> must be rewritten to _N for valid Verilog identifiers.
			name:       "bus indices rewritten to underscores",
			moduleName: "route",
			nl: &common.Netlist{
				Nets: []*common.Net{
					{Name: "net1", Pins: []common.RoutingPin{
						{Name: "I0.s<0>"},
						{Name: "I1.a<0>"},
					}},
					{Name: "WIDTH<0>", Pins: []common.RoutingPin{
						{Name: "I2.width<0>"},
					}},
				},
				Pins: []*common.RoutingPin{
					{Name: "WIDTH<0>"},
				},
			},
			want: `module route (
    inout I0_s_0,
    inout I1_a_0,
    inout I2_width_0,
    inout WIDTH_0
);

    assign I1_a_0 = I0_s_0;
    assign WIDTH_0 = I2_width_0;

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

// TestWriteVerilog_InvChain is an end-to-end test using real layout+schematic.
// All ports are inout; first pin in each net drives the rest.
//
// net1:  I1.VOUT (first) → I0.VIN      →  assign I0_VIN = I1_VOUT
// VIN:   I1.VIN  (first) → VIN (port)  →  assign VIN    = I1_VIN
// VOUT:  I0.VOUT (first) → VOUT (port) →  assign VOUT   = I0_VOUT
func TestWriteVerilog_InvChain(t *testing.T) {
	layout, schem, err := netlist.LoadFiles(
		"../../netlist/testdata/inv2_terminal_layout.json",
		"../../netlist/testdata/inv2_terminal_schematic.json",
	)
	if err != nil {
		t.Fatalf("load files: %v", err)
	}

	nl, err := netlist.BuildNetsFromData(layout, schem, &emptyDB{}, nil, []string{"VDD", "VSS"}, nil, nil, nil, true)
	if err != nil {
		t.Fatalf("build nets: %v", err)
	}

	got := writeVerilog("inv_chain", nl)
	want := `module inv_chain (
    inout I0_VIN,
    inout I0_VOUT,
    inout I1_VIN,
    inout I1_VOUT,
    inout VIN,
    inout VOUT
);

    assign I0_VIN = I1_VOUT;
    assign VIN = I1_VIN;
    assign VOUT = I0_VOUT;

endmodule
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// emptyDB satisfies netlist.PinDB with no entries, forcing terminal fallback.
type emptyDB struct{}

func (e *emptyDB) Query(lib, cell, pin string) (common.Nm, common.Nm, common.Nm, common.Nm, common.Layer, error) {
	return 0, 0, 0, 0, 0, common.ErrPinNotFound
}
