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
	nl, err := netlist.BuildNetsFromData(layout, schem, db, []string{"VDD", "VSS"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("build nets: %v", err)
	}

	got := writeVerilog("inv_chain", nl)
	want := `module inv_chain (
    input  I1_VOUT,
    output I0_VIN
);

    assign I0_VIN = I1_VOUT;

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
