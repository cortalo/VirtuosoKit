package main

import (
	"autorouter/canvas"
	"autorouter/celldb"
	"autorouter/common"
	"autorouter/drcdb"
	"autorouter/netlist"
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

// cellsPath returns the path to cells.toml, expected one directory above the binary
// (i.e. the module root when the binary lives in bin/).
func cellsPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "..", "cells.toml"), nil
}

// drcsPath returns the path to drcs.toml, next to cells.toml.
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

func parseDir(s string) common.Direction {
	switch s {
	case "vertical":
		return common.Vertical
	case "horizontal":
		return common.Horizontal
	default:
		fmt.Fprintf(os.Stderr, "error: unknown --m2-dir %q (use vertical or horizontal)\n", s)
		os.Exit(1)
		return 0
	}
}

func main() {
	verbose := flag.Bool("verbose", false, "print routing progress to stderr")
	mode := flag.String("mode", "classic", "routing mode: classic or full-track")
	m3TrackWidth := flag.Int("m3-track-width", 100, "M3 track width in nm")
	m2Width := flag.Int("m2-width", 100, "M2 wire width in nm (classic mode)")
	m2TrackWidth := flag.Int("m2-track-width", 100, "M2 track width in nm (full-track mode)")
	m2Dir := flag.String("m2-dir", "vertical", "M2 routing direction: vertical or horizontal (full-track mode)")
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

	cellsP, err := cellsPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	db, err := celldb.Load(cellsP)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load cells.toml: %v\n", err)
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

	m2DRC := loadDRCSpec(drcDB, *processLib, "M2")
	m3DRC := loadDRCSpec(drcDB, *processLib, "M3")
	via12 := loadViaConfig(drcDB, *processLib, "Via12")
	via23 := loadViaConfig(drcDB, *processLib, "Via23")

	var c session.Canvas
	var r session.Router
	switch *mode {
	case "classic":
		m3TrackCount := (ur.Y - ll.Y) / *m3TrackWidth
		tlc := &canvas.TwoLayerCanvas{
			LowerLeft:  ll,
			UpperRight: ur,
			M2Storage:  canvas.NewSegmentStore(ll, ur),
			M3Storage:  canvas.NewTrackSegmentStorage(m3TrackCount, *m3TrackWidth),
		}
		c, r = tlc, router.NewTwoLayerRouter(tlc, *m2Width, m2DRC, m3DRC)
		if *verbose {
			fmt.Fprintf(os.Stderr, "mode: classic  m3-tracks=%d\n", m3TrackCount)
		}
	case "full-track":
		m2TrackCount := (ur.X - ll.X) / *m2TrackWidth
		m3TrackCount := (ur.Y - ll.Y) / *m3TrackWidth
		ftc := &canvas.FullTrackCanvas{
			LowerLeft:  ll,
			UpperRight: ur,
			M2Storage:  canvas.NewTrackSegmentStorage(m2TrackCount, *m2TrackWidth),
			M3Storage:  canvas.NewTrackSegmentStorage(m3TrackCount, *m3TrackWidth),
			M2Dir:      parseDir(*m2Dir),
		}
		c, r = ftc, router.NewFullTrackRouter(ftc, parseDir(*m2Dir), m2DRC, m3DRC)
		if *verbose {
			fmt.Fprintf(os.Stderr, "mode: full-track  m2-tracks=%d m3-tracks=%d m2-dir=%s\n",
				m2TrackCount, m3TrackCount, *m2Dir)
		}
	default:
		fmt.Fprintf(os.Stderr, "error: unknown --mode %q (use classic or full-track)\n", *mode)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "canvas: ll=%v ur=%v\n", ll, ur)
		fmt.Fprintf(os.Stderr, "nets: %d to route\n", len(nl.Nets))
		fmt.Fprintf(os.Stderr, "via12: %+v\n", via12)
		fmt.Fprintf(os.Stderr, "via23: %+v\n", via23)
	}

	s := session.NewSession(c, r, nl, via12, via23, m2DRC, m3DRC)
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
