package main

import (
	"autorouter/canvas"
	"autorouter/netlist"
	"autorouter/pindb"
	"autorouter/router"
	"autorouter/session"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- request ---

type request struct {
	Layout    netlist.Layout    `json:"layout"`
	Schematic netlist.Schematic `json:"schematic"`
}

// --- response ---

type response struct {
	Routes []session.NetResult `json:"routes"`
}

// pinsPath returns the path to pins.toml, expected one directory above the binary
// (i.e. the module root when the binary lives in bin/).
func pinsPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "..", "pins.toml"), nil
}

// ignoreNetFlag accumulates repeated -ignore-net flags into a slice.
type ignoreNetFlag []string

func (f *ignoreNetFlag) String() string { return strings.Join(*f, ",") }
func (f *ignoreNetFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func main() {
	m3TrackWidth := flag.Int("m3-track-width", 100, "M3 track width in nm")
	m2Width := flag.Int("m2-width", 100, "M2 via width in nm")
	var ignoreNets ignoreNetFlag
	flag.Var(&ignoreNets, "ignore-net", "net name to skip routing (repeatable, e.g. -ignore-net VDD -ignore-net VSS)")
	var ignoreLibs ignoreNetFlag
	flag.Var(&ignoreLibs, "ignore-lib", "lib name whose instances are excluded from routing (repeatable, e.g. -ignore-lib analogLib)")
	flag.Parse()

	pinPath, err := pinsPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	db, err := pindb.Load(pinPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load pins.toml: %v\n", err)
		os.Exit(1)
	}

	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fmt.Fprintf(os.Stderr, "error: decode request: %v\n", err)
		os.Exit(1)
	}

	ll, ur, nets, err := netlist.BuildNetsFromData(req.Layout, req.Schematic, db, ignoreNets, ignoreLibs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: build nets: %v\n", err)
		os.Exit(1)
	}

	trackCount := (ur.Y - ll.Y) / *m3TrackWidth
	c := &canvas.Canvas{
		LowerLeft:  ll,
		UpperRight: ur,
		M2Storage:  canvas.NewSegmentStore(ll, ur),
		M3Storage:  canvas.NewTrackSegmentStorage(trackCount, *m3TrackWidth),
	}
	s := session.NewSession(c, router.NewTwoLayerRouter(c, *m2Width), nets)
	resp := response{Routes: s.Route()}

	if err := json.NewEncoder(os.Stdout).Encode(resp); err != nil {
		fmt.Fprintf(os.Stderr, "error: encode response: %v\n", err)
		os.Exit(1)
	}
}
