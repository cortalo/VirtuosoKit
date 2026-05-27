# VirtuosoKit

A lightweight automation toolkit for Cadence Virtuoso. Currently focused on
standard-cell place and route: it reads schematic data directly from a running
Virtuoso session, runs placement and routing, and writes the result back as a
Virtuoso layout — no LEF/lib/SDC required.

The long-term goal is to integrate with LangGraph to build an AI agent that
can drive the full custom-layout flow from a schematic.

## Architecture

```
Virtuoso CIW
     │  SKILL / virtuoso-bridge-lite
     ▼
Python scripts (place.py / route.py / pnr.py)
     │  JSON over stdin/stdout
     ├──▶ placer     (Go) — schematic-aware standard-cell placement
     └──▶ autorouter (Go) — two-layer M2/M3 net routing
```

## Setup

### 1. Clone

```bash
git clone --recurse-submodules https://github.com/Cortalo/langgraph4virtuoso.git
```

If you already cloned without submodules:

```bash
git submodule update --init
cd virtuoso-bridge-lite && git checkout eacf3ca && cd ..
```

### 2. Python environment

Requires Python 3.11, 3.12, or 3.13.

```bash
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
pip install -e virtuoso-bridge-lite
```

### 3. Environment variables

```bash
export RB_DAEMON_PATH=/path/to/VirtuosoKit/virtuoso-bridge-lite/src/virtuoso_bridge/virtuoso/basic/resources/ramic_bridge_daemon_3.py
export RB_PYTHON_PATH=python3
export RB_PORT=65432
```

### 4. Start the Virtuoso bridge

In the Virtuoso CIW:

```
load("/abs/path/to/VirtuosoKit/virtuoso-bridge-lite/src/virtuoso_bridge/virtuoso/basic/resources/ramic_bridge.il")
```

You should see:
```
[RAMIC Bridge ipc=...] ready: bind=0.0.0.0:65432
```

## Build

```bash
cd placer && go build -o bin/placer ./cmd/placer/ && cd ..
cd autorouter && go build -o bin/autorouter ./cmd/autorouter/ && cd ..
```

## PDK configuration

Before running the scripts you need to supply two PDK-specific values:

1. **`--row-height`** — the physical height of one standard-cell row in nm.
   Matches the height of a single standard cell in your PDK.

2. **Layer map in `route.py`** — add an entry to `_LAYER_MAPS` keyed by the
   name you will pass as `--process-lib`. Map each autorouter internal layer
   name (`M1`, `M2`, `M3`, `Via12`, `Via23`) to the corresponding physical
   layer name in your PDK.

## Usage

### Place

Reads schematic instance positions and places them into a layout cellview with
a prBoundary.

```bash
python place.py <lib> <cell> \
    --row-height <ROW_HEIGHT_NM> \
    --ignore-lib basic
```

Key options:
- `--row-height` (required) — standard cell height in nm
- `--target-width` — maximum row width in nm; splits rows to fit; 0 disables (default: 0)
- `--pr-margin` — prBoundary margin in nm (default: 10000 = 10 um)
- `--ignore-lib` — Cadence infrastructure library with no layout to skip (repeatable, default: basic)

### Route

Reads the placed layout and schematic connectivity, runs the Go autorouter,
and draws the result back into the layout.

```bash
python route.py <lib> <cell> \
    --process-lib <YOUR_PROCESS> \
    --m3-track-width <NM> \
    --m2-width <NM> \
    --ignore-net VDD --ignore-net VSS \
    --ignore-lib basic
```

Key options:
- `--process-lib` (required) — must match a key in `_LAYER_MAPS` in `route.py`
- `--m3-track-width` (required) — M3 routing track pitch in nm
- `--m2-width` (required) — M2 wire width in nm
- `--ignore-net` — net to skip routing, e.g. power rails (repeatable)
- `--power-net` — net to route with the power router (repeatable)
- `--ignore-lib` — Cadence infrastructure library with no layout to skip (repeatable, default: basic)
- `--min-overlap-lib` — library whose pins use minimum M2 overlap (repeatable)
- `--widen-narrow-pins` — widen M1 pins narrower than m2-width to m2-width
- `--drc` / `--lvs` — launch Calibre DRC/LVS after routing

See [`autorouter/README.md`](autorouter/README.md) for DRC configuration
(`drcs.toml`, `pins.toml`) and the full JSON API.

### Place and Route in one step

```bash
python pnr.py <lib> <cell> \
    --row-height <ROW_HEIGHT_NM> \
    --process-lib <YOUR_PROCESS> \
    --m3-track-width <NM> \
    --m2-width <NM> \
    --ignore-net VDD --ignore-net VSS
```

To also extend power rails after routing, pass the PDK layer names and rail geometry:

```bash
python pnr.py <lib> <cell> \
    --row-height <ROW_HEIGHT_NM> \
    --process-lib <YOUR_PROCESS> \
    --m3-track-width <NM> \
    --m2-width <NM> \
    --ignore-net VDD --ignore-net VSS \
    --m1-layer <M1_LAYER> --m2-layer <M2_LAYER> --via12-layer <VIA12_LAYER> \
    --rail-half <NM> --via-cut <UM> --via-spacing-x <UM> --via-spacing-y <UM>
```

With Calibre DRC/LVS:

```bash
python pnr.py <lib> <cell> \
    --row-height <ROW_HEIGHT_NM> \
    --process-lib <YOUR_PROCESS> \
    --m3-track-width <NM> \
    --m2-width <NM> \
    --drc --lvs
```
