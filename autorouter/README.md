# autorouter

A two-layer M2/M3 autorouter for Cadence Virtuoso standard-cell layouts.
It reads layout and schematic data from Virtuoso, routes nets on M2 (vertical)
and M3 (horizontal) layers, places via cuts (VIA12/VIA23), and writes the
result back to the layout.

## Prerequisites

- Go 1.21+
- Python 3.10+ with the `langgraph` conda environment (includes `virtuoso-bridge`)
- Cadence Virtuoso running with the bridge server on port 65432

## Build

```bash
cd autorouter
go build -o bin/autorouter ./cmd/autorouter/
```

## Configuration

Two TOML files sit next to the binary (i.e. in the `autorouter/` directory):

### `pins.toml`

Defines the M1 pin bounding boxes for each cell in each library.
The router uses these to locate where to connect vias from M2 down to M1.

```toml
[<lib>.<cell>]
pins = [
  { name = "A", ll = [xLow, yLow], ur = [xHigh, yHigh] },
]
```

All coordinates are in nm relative to the instance origin.

### `drcs.toml`

Defines DRC rules per process library for metal layers and via types.

```toml
[<process_lib>.M2]
min_area      = 202000   # nm²
end_extension = 60       # nm — min M2 extension past via end-of-line

[<process_lib>.M3]
min_area      = 202000
end_extension = 60

[<process_lib>.Via12]    # M1–M2 via
cut_w   = 260            # cut width  (nm)
cut_h   = 260            # cut height (nm)
space_x = 260            # cut-to-cut X spacing (nm)
space_y = 260            # cut-to-cut Y spacing (nm)

[<process_lib>.Via23]    # M2–M3 via
cut_w   = 260
cut_h   = 260
space_x = 260
space_y = 260
```

## Running

Use the Python script in `example/route.py`. It reads layout and schematic
from Virtuoso, calls the Go binary, and draws the result back:

```bash
python route.py <lib> <cell> [options]
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `--process-lib LIB` | _(none)_ | Process library name for DRC lookup (e.g. `tsmc18`) |
| `--m3-track-width NM` | `400` | M3 track width in nm |
| `--m2-width NM` | `280` | M2 wire width in nm |
| `--ignore-net NET` | _(none)_ | Skip routing this net (repeatable) |
| `--ignore-lib LIB` | _(none)_ | Skip instances from this library (repeatable) |
| `--port PORT` | `65432` | Virtuoso bridge TCP port |
| `--binary PATH` | `autorouter/bin/autorouter` | Path to the Go binary |
| `--verbose` | off | Print per-net routing details to stderr |

### Example

```bash
python route.py test pfd_mini_delay_1 \
    --process-lib tsmc18 \
    --m3-track-width 400 \
    --m2-width 280 \
    --ignore-net VDD --ignore-net VSS \
    --ignore-lib basic \
    --verbose
```

## JSON API

The Go binary reads a JSON request on stdin and writes a JSON response on stdout.
This lets you integrate it with other tools without going through Virtuoso.

### Request

```json
{
  "layout": {
    "shapes":    [{ "layer": ["METAL1","drawing"], "bbox": [[x0,y0],[x1,y1]] }],
    "instances": [{ "name": "I1", "lib": "myLib", "cell": "NAND2", "xy": [x, y] }]
  },
  "schematic": {
    "instances": [{ "name": "I1", "lib": "myLib" }],
    "nets": { "A": ["I1/A", "I2/Z"] }
  }
}
```

### Response

```json
{
  "routes": [
    {
      "net_id": 1,
      "segments": [
        { "layer": "M2",    "lower_left": {"x":100,"y":200}, "upper_right": {"x":140,"y":900}, "net_id": 1 },
        { "layer": "M3",    "lower_left": {"x":100,"y":400}, "upper_right": {"x":800,"y":800}, "net_id": 1 },
        { "layer": "Via12", "lower_left": {"x":110,"y":610}, "upper_right": {"x":370,"y":870}, "net_id": 1 }
      ]
    },
    {
      "net_id": 2,
      "error": "out of bound"
    }
  ]
}
```

Layers in the response: `M2`, `M3`, `Via12` (VIA12 cut, M1–M2), `Via23` (VIA23 cut, M2–M3).
All coordinates are in nm.

## Architecture

```
cmd/autorouter/   CLI entry point — parses flags, loads config, calls session
netlist/          Builds Net objects from layout + schematic JSON
pindb/            Loads pins.toml, maps cell pin names to M1 bounding boxes
drcdb/            Loads drcs.toml, provides metal DRC rules and via configs
canvas/           Tracks occupied M2 segments and M3 tracks (conflict detection)
router/           Two-layer router: picks a free M3 track, extends M2 stubs
session/          Orchestrates routing, extends M2 to cover pins, places via cuts
common/           Shared types (Point, Segment, Layer, Net, ViaConfig, DRCSpec)
```
