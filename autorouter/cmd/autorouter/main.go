package main

import (
	"autorouter/canvas"
	"autorouter/common"
	"autorouter/drcdb"
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

// drcsPath returns the path to drcs.toml, next to pins.toml.
func drcsPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "..", "drcs.toml"), nil
}

// loadDRCSpec queries a single layer's DRC rules, falling back to NoDRC on any error.
func loadDRCSpec(db *drcdb.DB, lib, layer string) common.DRCSpec {
	if db == nil || lib == "" {
		return common.NoDRC{}
	}
	spec, err := db.Query(lib, layer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: drc rule not found for %s.%s: %v, using no constraint\n", lib, layer, err)
		return common.NoDRC{}
	}
	return spec
}

// loadViaConfig queries a via config from the DB, returning a zero ViaConfig on any error.
func loadViaConfig(db *drcdb.DB, lib, viaName string) common.ViaConfig {
	if db == nil || lib == "" {
		return common.ViaConfig{}
	}
	vc, err := db.QueryVia(lib, viaName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: via config not found for %s.%s: %v, via placement disabled\n", lib, viaName, err)
		return common.ViaConfig{}
	}
	return vc
}

// ignoreNetFlag accumulates repeated -ignore-net flags into a slice.
type ignoreNetFlag []string

func (f *ignoreNetFlag) String() string { return strings.Join(*f, ",") }
func (f *ignoreNetFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func main() {
	verbose := flag.Bool("verbose", false, "print routing progress to stderr")
	m3TrackWidth := flag.Int("m3-track-width", 100, "M3 track width in nm")
	m2Width := flag.Int("m2-width", 100, "M2 via width in nm")
	var ignoreNets ignoreNetFlag
	flag.Var(&ignoreNets, "ignore-net", "net name to skip routing (repeatable, e.g. -ignore-net VDD -ignore-net VSS)")
	var ignoreLibs ignoreNetFlag
	flag.Var(&ignoreLibs, "ignore-lib", "lib name whose instances are excluded from routing (repeatable, e.g. -ignore-lib analogLib)")
	processLib := flag.String("process-lib", "", "process library name for DRC rules lookup (e.g. tsmc18)")
	flag.Parse()

	drcsP, err := drcsPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	var drcDB *drcdb.DB
	if db, loadErr := drcdb.Load(drcsP); loadErr == nil {
		drcDB = db
	} else {
		fmt.Fprintf(os.Stderr, "warning: could not load drcs.toml: %v, DRC rules disabled\n", loadErr)
	}

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

	ll, ur, nl, err := netlist.BuildNetsFromData(req.Layout, req.Schematic, db, ignoreNets, ignoreLibs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: build nets: %v\n", err)
		os.Exit(1)
	}

	trackCount := (ur.Y - ll.Y) / *m3TrackWidth
	c := &canvas.TwoLayerCanvas{
		LowerLeft:  ll,
		UpperRight: ur,
		M2Storage:  canvas.NewSegmentStore(ll, ur),
		M3Storage:  canvas.NewTrackSegmentStorage(trackCount, *m3TrackWidth),
	}
	m2DRC := loadDRCSpec(drcDB, *processLib, "M2")
	m3DRC := loadDRCSpec(drcDB, *processLib, "M3")
	via12 := loadViaConfig(drcDB, *processLib, "Via12")
	via23 := loadViaConfig(drcDB, *processLib, "Via23")
	if *verbose {
		fmt.Fprintf(os.Stderr, "canvas: ll=%v ur=%v tracks=%d\n", ll, ur, trackCount)
		fmt.Fprintf(os.Stderr, "nets: %d to route\n", len(nl.Nets))
		fmt.Fprintf(os.Stderr, "via12: %+v\n", via12)
		fmt.Fprintf(os.Stderr, "via23: %+v\n", via23)
	}

	s := session.NewSession(c, router.NewTwoLayerRouter(c, *m2Width, m2DRC, m3DRC), nl, via12, via23, m2DRC, m3DRC)
	routes := s.Route()

	if *verbose {
		ok := 0
		for _, r := range routes {
			if r.Err != nil {
				fmt.Fprintf(os.Stderr, "  net %d FAILED: %v\n", r.NetID, r.Err)
				continue
			}
			ok++
			fmt.Fprintf(os.Stderr, "  net %d:\n", r.NetID)
			for _, seg := range r.Shapes {
				fmt.Fprintf(os.Stderr, "    seg  layer=%v ll=%v ur=%v\n", seg.Layer, seg.LowerLeft, seg.UpperRight)
			}
		}
		fmt.Fprintf(os.Stderr, "routed %d/%d nets\n", ok, len(routes))
	}

	resp := response{Routes: routes}

	if err := json.NewEncoder(os.Stdout).Encode(resp); err != nil {
		fmt.Fprintf(os.Stderr, "error: encode response: %v\n", err)
		os.Exit(1)
	}
}
