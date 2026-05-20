# LangGraph + Virtuoso Bridge

LangGraph agents that execute SKILL in a running Virtuoso session,
built on top of [virtuoso-bridge-lite](https://github.com/Arcadia-1/virtuoso-bridge-lite).

## Clone

This repo uses `virtuoso-bridge-lite` as a git submodule (pinned to commit `eacf3ca`).

Clone with submodules in one step:

```bash
git clone --recurse-submodules https://github.com/Cortalo/langgraph4virtuoso.git
```

If you already cloned without `--recurse-submodules`, initialize the submodule afterwards:

```bash
git submodule update --init
cd virtuoso-bridge-lite
git checkout eacf3ca
cd ..
```

## Environment Variables

Set these in your shell (e.g. `~/.bashrc`) before starting Virtuoso:

```bash
export RB_DAEMON_PATH=/path/to/langgraph4virtuoso/virtuoso-bridge-lite/src/virtuoso_bridge/virtuoso/basic/resources/ramic_bridge_daemon_3.py
export RB_PYTHON_PATH=python3
export RB_PORT=65432
export OPENAI_API_KEY=your_key_here
```

## Python Environment

Requires Python 3.11, 3.12, or 3.13. Check first:

```bash
python3 --version
```

Create a virtual environment and install dependencies:

```bash
python3 -m venv langgraph-env
source langgraph-env/bin/activate
pip install -r requirements.txt
pip install -e virtuoso-bridge-lite
```

## Start the Virtuoso Daemon

1. Source your shell config and start Virtuoso from that shell:
   ```bash
   source ~/.bashrc
   # then launch Virtuoso
   ```

2. In the Virtuoso CIW, load the bridge:
   ```
   load("/abs/path/to/langgraph4virtuoso/virtuoso-bridge-lite/src/virtuoso_bridge/virtuoso/basic/resources/ramic_bridge.il")
   ```
   You should see:
   ```
   [RAMIC Bridge ipc=...] ready: bind=0.0.0.0:65432
   ```

3. The daemon stays alive as long as Virtuoso is running and exits automatically when Virtuoso exits.

To stop the daemon manually from CIW:
```
RBStop()
```

## Run Examples

> **⚠️ WARNING — Demo only.**
> This project is under active development and is not production-ready.
> Do **not** run these scripts on real projects. They may overwrite, corrupt,
> or permanently delete your Virtuoso cells and layout data.

```bash
source langgraph-env/bin/activate
python hello_virtuoso.py
python example/agent_place_demo.py
python example/agent_place_demo.py --debug
```

## Placer

A standard-cell placer (Go) that reads schematic instance positions from
Virtuoso and creates a placed layout.

Build the binary first:

```bash
cd placer
go build -o bin/placer ./cmd/placer/
```

Place a cell:

```bash
python place.py test pfd_mini_delay_1 --ignore-lib basic --row-height 3920
```

Key options: `--row-height` (nm, default 3920), `--pr-margin` (nm, default 10000).

## Autorouter

A two-layer M2/M3 autorouter (Go) that reads layout and schematic data from
Virtuoso, routes nets, and draws the result back into the layout.

Build the binary first:

```bash
cd autorouter
go build -o bin/autorouter ./cmd/autorouter/
```

Route a cell:

```bash
python route.py test pfd_mini_delay_1 \
    --process-lib tsmc18 \
    --ignore-net VDD --ignore-net VSS \
    --ignore-lib basic
```

See [`autorouter/README.md`](autorouter/README.md) for full configuration,
DRC setup (`drcs.toml`, `pins.toml`), the JSON API, and architecture details.

## PnR (Place and Route)

Run placement and routing in one step:

```bash
python pnr.py test pfd_mini_delay_1 \
    --process-lib tsmc18 \
    --ignore-net VDD --ignore-net VSS \
    --ignore-lib basic \
    --row-height 3920
```

`pnr.py` places the cell, routes it, then removes the prBoundary helper shape.
Build both binaries before running.
