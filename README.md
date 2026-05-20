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
     ├──▶ placer    (Go) — schematic-aware standard-cell placement
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

## Usage

### Place

```bash
python place.py test pfd_mini_delay_1 --ignore-lib basic --row-height 3920
```

Key options: `--row-height` (nm, default 3920), `--pr-margin` (nm, default 10000),
`--target-width` (nm, splits and repacks rows to fit; 0 disables).

### Route

```bash
python route.py test pfd_mini_delay_1 \
    --process-lib tsmc18 \
    --ignore-net VDD --ignore-net VSS \
    --ignore-lib basic
```

See [`autorouter/README.md`](autorouter/README.md) for DRC configuration
(`drcs.toml`, `pins.toml`) and the full JSON API.

### Place and Route in one step

```bash
python pnr.py test pfd_mini_delay_1 \
    --process-lib tsmc18 \
    --ignore-net VDD --ignore-net VSS \
    --ignore-lib basic \
    --row-height 3920
```

With Calibre DRC/LVS:

```bash
python pnr.py test pfd_mini_delay_1 \
    --process-lib tsmc18 \
    --ignore-net VDD --ignore-net VSS \
    --ignore-lib basic \
    --row-height 3920 \
    --drc --lvs
```
