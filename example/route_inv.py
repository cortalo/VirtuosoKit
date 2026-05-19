"""Run the Go autorouter on the inverter and draw the routes into Virtuoso.

Build the binary first (from autorouter/):
    go build -o bin/autorouter ./cmd/autorouter/

Then run:
    python example/route_inv.py
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.models import ExecutionStatus
from virtuoso_bridge.virtuoso.layout import parse_layout_geometry_output
from virtuoso_bridge.virtuoso.layout.ops import layout_create_rect, layout_read_geometry
from virtuoso_bridge.virtuoso.ops import save_current_cellview
from virtuoso_bridge.virtuoso.schematic.reader import read_schematic

HERE = Path(__file__).parent
BINARY = HERE / "../autorouter/bin/autorouter"

LIB = "test"
CELL = "pfd_mini_delay_1"

M3_TRACK_WIDTH_NM = 400
M2_WIDTH_NM = 280
M2_LAYER = ("METAL2", "drawing")
M3_LAYER = ("METAL3", "drawing")
IGNORE_NETS = ["VDD", "VSS"]
IGNORE_LIBS = ["basic"]
PROCESS_LIB = "tsmc18"


def nm_to_um(v: int) -> float:
    return v / 1000.0


def _skill(client: VirtuosoClient, cmd: str) -> None:
    result = client.execute_skill(cmd, timeout=30)
    if result.status != ExecutionStatus.SUCCESS:
        raise RuntimeError(f"SKILL failed: {result.errors}\n  cmd: {cmd[:120]}")


def read_layout(client: VirtuosoClient) -> tuple[list[dict], list[dict]]:
    result = client.execute_skill(layout_read_geometry(LIB, CELL), timeout=30)
    raw = result.output or ""
    if raw.startswith('"ERROR') or raw.startswith("ERROR"):
        raise RuntimeError(f"layout read failed: {raw}")
    geometry = result.metadata.get("geometry") or parse_layout_geometry_output(raw)
    shapes, instances = [], []
    for obj in geometry:
        if obj.get("kind") == "instance":
            instances.append({
                "name": obj["name"],
                "lib":  obj["lib"],
                "cell": obj["cell"],
                "xy":   obj["xy"],
            })
        else:
            shapes.append({
                "layer": obj.get("layer"),
                "bbox":  obj.get("bbox"),
            })
    return shapes, instances


LAYER_MAP = {
    "M2":    M2_LAYER,
    "M3":    M3_LAYER,
    "Via12": ("VIA12", "drawing"),
    "Via23": ("VIA23", "drawing"),
}


def draw_routes(client: VirtuosoClient, routes: list[dict]) -> None:
    _skill(client, f'cv = dbOpenCellViewByType("{LIB}" "{CELL}" "layout" "maskLayout" "a")')

    for route in routes:
        if route.get("error"):
            continue

        for seg in route.get("segments", []):
            ll, ur = seg["lower_left"], seg["upper_right"]
            layer = LAYER_MAP[seg["layer"]]
            _skill(client, layout_create_rect(
                *layer,
                nm_to_um(ll["x"]), nm_to_um(ll["y"]),
                nm_to_um(ur["x"]), nm_to_um(ur["y"]),
            ))

    _skill(client, save_current_cellview())


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("-verbose", action="store_true", help="print routing progress to stderr")
    args = parser.parse_args()

    if not BINARY.exists():
        print(
            f"ERROR: binary not found at {BINARY}\n"
            "Build it first:\n"
            "    cd autorouter && go build -o bin/autorouter ./cmd/autorouter/",
            file=sys.stderr,
        )
        return 1

    client = VirtuosoClient.local(port=65432)

    shapes, instances = read_layout(client)

    schem = read_schematic(client, LIB, CELL)
    nets = {name: net["connections"] for name, net in schem["nets"].items()}
    schem_instances = [
        {"name": inst["name"], "lib": inst["lib"]}
        for inst in schem["instances"]
    ]

    payload = {
        "layout":    {"shapes": shapes, "instances": instances},
        "schematic": {"instances": schem_instances, "nets": nets},
    }

    cmd = [
        str(BINARY),
        f"-m3-track-width={M3_TRACK_WIDTH_NM}",
        f"-m2-width={M2_WIDTH_NM}",
        *[f"-ignore-net={n}" for n in IGNORE_NETS],
        *[f"-ignore-lib={l}" for l in IGNORE_LIBS],
        f"-process-lib={PROCESS_LIB}",
        *(["-verbose"] if args.verbose else []),
    ]
    proc = subprocess.run(
        cmd,
        input=json.dumps(payload),
        stdout=subprocess.PIPE,
        stderr=None if args.verbose else subprocess.PIPE,
        text=True,
    )
    if proc.returncode != 0:
        if not args.verbose:
            print(proc.stderr, file=sys.stderr)
        return proc.returncode

    routes = json.loads(proc.stdout)["routes"]

    ok  = [r for r in routes if not r.get("error")]
    err = [r for r in routes if r.get("error")]
    print(f"Routed {len(ok)}/{len(routes)} nets"
          + (f" ({len(err)} failed)" if err else ""))
    for r in err:
        print(f"  net {r['net_id']} FAILED: {r['error']}", file=sys.stderr)

    draw_routes(client, routes)
    print(f"Drew {len(ok) * 3} shapes into {LIB}/{CELL}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
