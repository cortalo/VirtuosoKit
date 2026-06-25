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
	"math"
	"os"
	"path/filepath"
	"strings"
)

// --- request ---

type request struct {
	Layout       netlist.RawLayout    `json:"layout"`
	RawSchematic netlist.RawSchematic `json:"schematic"`
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

// buildCanvasInstances converts layout instances to canvas.Instance values by
// looking up each cell's pre-existing metals in celldb. Instances whose cell
// has no metals entry are silently skipped.
func buildCanvasInstances(instances []netlist.LayoutInstance, db *celldb.DB) []canvas.Instance {
	var result []canvas.Instance
	for _, inst := range instances {
		metals, err := db.QueryMetals(inst.Lib, inst.Cell)
		if err != nil || len(metals) == 0 {
			continue
		}
		orient, err := canvas.ParseOrient(strings.Trim(inst.Orient, "\""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: instance %s: %v, skipping metals\n", inst.Name, err)
			continue
		}
		shapes := make([]common.Shape, 0, len(metals))
		for _, m := range metals {
			layer, err := common.ParseLayer(m.Layer)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: instance %s metal layer %q: %v, skipping\n", inst.Name, m.Layer, err)
				continue
			}
			shapes = append(shapes, common.Shape{
				LowerLeft:  common.Point{X: m.LL[0], Y: m.LL[1]},
				UpperRight: common.Point{X: m.UR[0], Y: m.UR[1]},
				Layer:      layer,
			})
		}
		if len(shapes) == 0 {
			continue
		}
		result = append(result, canvas.Instance{
			XY:     common.Point{X: int(math.Round(inst.XY[0] * 1000)), Y: int(math.Round(inst.XY[1] * 1000))},
			Orient: orient,
			Metals: shapes,
		})
	}
	return result
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
	var ignoreLibNets ignoreNetFlag
	flag.Var(&ignoreLibNets, "ignore-lib-net", "lib:net pair to skip pins of that net only for instances in that lib (repeatable, e.g. -ignore-lib-net analogLib:VDD)")
	var minOverlapLibs ignoreNetFlag
	flag.Var(&minOverlapLibs, "min-overlap-lib", "lib name whose pins use minimum M2 overlap (repeatable, e.g. -min-overlap-lib stdcellLib)")
	var powerNets ignoreNetFlag
	flag.Var(&powerNets, "power-net", "net name to route with PowerRouter (repeatable, e.g. -power-net VDD -power-net VSS)")
	processLib := flag.String("process-lib", "", "process library name for DRC rules lookup (e.g. tsmc18)")
	widenNarrowPins := flag.Bool("widen-narrow-pins", false, "widen M1 pins narrower than m2-width to m2-width, centered on the pin (classic mode)")
	innovus := flag.Bool("innovus", false, "write Verilog for Innovus instead of routing")
	moduleName        := flag.String("module-name", "", "Verilog module name (required with -innovus)")
	outputPath        := flag.String("output", "", "absolute path for Verilog output file (required with -innovus)")
	pinsOutput        := flag.String("pins-output", "", "absolute path for pin coordinates JSON file (required with -innovus)")
	connectionsOutput := flag.String("connections-output", "", "absolute path for net connections JSON file (required with -innovus)")
	flag.Parse()

	if *innovus {
		for flag, val := range map[string]string{
			"-module-name":        *moduleName,
			"-output":             *outputPath,
			"-pins-output":        *pinsOutput,
			"-connections-output": *connectionsOutput,
		} {
			if val == "" {
				fmt.Fprintf(os.Stderr, "error: %s is required with -innovus\n", flag)
				os.Exit(1)
			}
			if flag != "-module-name" && !filepath.IsAbs(val) {
				fmt.Fprintf(os.Stderr, "error: %s must be an absolute path, got %q\n", flag, val)
				os.Exit(1)
			}
		}
	}

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

	nl, err := netlist.BuildNetsFromData(req.Layout, req.RawSchematic, db,
		[]string(ignoreNets), []string(ignoreLibs), []string(minOverlapLibs), []string(ignoreLibNets), *innovus)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: build nets: %v\n", err)
		os.Exit(1)
	}

	if *innovus {
		if err := os.WriteFile(*outputPath, []byte(writeVerilog(*moduleName, nl)), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: write verilog: %v\n", err)
			os.Exit(1)
		}
		pinsJSON, err := writePinsJSON(nl)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: marshal pins: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*pinsOutput, pinsJSON, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: write pins: %v\n", err)
			os.Exit(1)
		}
		connJSON, err := writeNetConnectionsJSON(nl)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: marshal connections: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*connectionsOutput, connJSON, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: write connections: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ll, ur, err := netlist.PRBoundary(req.Layout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: pr boundary: %v\n", err)
		os.Exit(1)
	}

	m2DRC := loadDRCSpec(drcDB, *processLib, "M2")
	m3DRC := loadDRCSpec(drcDB, *processLib, "M3")
	via12 := loadViaConfig(drcDB, *processLib, "Via12")
	via23 := loadViaConfig(drcDB, *processLib, "Via23")
	var c session.Canvas
	var r session.Router
	var rc router.Canvas
	switch *mode {
	case "classic":
		m3TrackCount := (ur.Y - ll.Y) / *m3TrackWidth
		tlc := &canvas.TwoLayerCanvas{
			LowerLeft:  ll,
			UpperRight: ur,
			M2Storage:  canvas.NewSegmentStore(ll, ur),
			M3Storage:  canvas.NewTrackSegmentStorage(m3TrackCount, *m3TrackWidth),
		}
		tlr := router.NewTwoLayerRouter(tlc, *m2Width, m2DRC, m3DRC)
		tlr.SetWidenNarrowPins(*widenNarrowPins)
		c, r, rc = tlc, tlr, tlc
		if *verbose {
			fmt.Fprintf(os.Stderr, "mode: classic  m3-tracks=%d\n", m3TrackCount)
		}
	case "full-track":
		m2TrackCount := (ur.X - ll.X) / *m2TrackWidth
		m3TrackCount := (ur.Y - ll.Y) / *m3TrackWidth
		canvasInsts := buildCanvasInstances(req.Layout.Instances, db)
		dir := parseDir(*m2Dir)
		ftc, err := canvas.NewFullTrackCanvas(
			ll, ur,
			canvas.NewTrackSegmentStorage(m2TrackCount, *m2TrackWidth),
			canvas.NewTrackSegmentStorage(m3TrackCount, *m3TrackWidth),
			dir,
			canvasInsts,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: build canvas: %v\n", err)
			os.Exit(1)
		}
		c, r, rc = ftc, router.NewFullTrackRouter(ftc, dir, m2DRC, m3DRC), ftc
		if *verbose {
			fmt.Fprintf(os.Stderr, "mode: full-track  m2-tracks=%d m3-tracks=%d m2-dir=%s instances=%d\n",
				m2TrackCount, m3TrackCount, *m2Dir, len(canvasInsts))
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
	if len(powerNets) > 0 {
		pr := router.NewPowerRouter(rc, *m2Width, m2DRC, m3DRC)
		pr.SetWidenNarrowPins(*widenNarrowPins)
		s.SetPowerRouter(pr, powerNets...)
		if *verbose {
			fmt.Fprintf(os.Stderr, "power nets: %s\n", strings.Join(powerNets, ", "))
		}
	}
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
