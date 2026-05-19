"""Autoroute a Virtuoso cell using the Go autorouter and draw results back.

Build the binary first:
    cd autorouter && go build -o bin/autorouter ./cmd/autorouter/

Usage:
    python route.py <lib> <cell> [options]

Example:
    python route.py test pfd_mini_delay_1 \\
        --process-lib tsmc18 \\
        --ignore-net VDD --ignore-net VSS \\
        --ignore-lib basic
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
DEFAULT_BINARY = HERE / "autorouter/bin/autorouter"

LAYER_MAP = {
    "M2":    ("METAL2", "drawing"),
    "M3":    ("METAL3", "drawing"),
    "Via12": ("VIA12",  "drawing"),
    "Via23": ("VIA23",  "drawing"),
}


def nm_to_um(v: int) -> float:
    return v / 1000.0


def _skill(client: VirtuosoClient, cmd: str) -> None:
    result = client.execute_skill(cmd, timeout=30)
    if result.status != ExecutionStatus.SUCCESS:
        raise RuntimeError(f"SKILL failed: {result.errors}\n  cmd: {cmd[:120]}")


def read_layout(client: VirtuosoClient, lib: str, cell: str) -> tuple[list[dict], list[dict]]:
    result = client.execute_skill(layout_read_geometry(lib, cell), timeout=30)
    raw = result.output or ""
    if raw.startswith('"ERROR') or raw.startswith("ERROR"):
        raise RuntimeError(f"layout read failed: {raw}")
    geometry = result.metadata.get("geometry") or parse_layout_geometry_output(raw)
    shapes, instances = [], []
    for obj in geometry:
        if obj.get("kind") == "instance":
            instances.append({
                "name":   obj["name"],
                "lib":    obj["lib"],
                "cell":   obj["cell"],
                "xy":     obj["xy"],
                "orient": obj.get("orient", "R0"),
            })
        else:
            shapes.append({
                "layer": obj.get("layer"),
                "bbox":  obj.get("bbox"),
            })
    return shapes, instances


def draw_routes(client: VirtuosoClient, lib: str, cell: str, routes: list[dict]) -> int:
    _skill(client, f'cv = dbOpenCellViewByType("{lib}" "{cell}" "layout" "maskLayout" "a")')
    drawn = 0
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
            drawn += 1
    _skill(client, save_current_cellview())
    return drawn


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Autoroute a Virtuoso cell and draw results back into the layout."
    )
    p.add_argument("lib",  help="Virtuoso library name")
    p.add_argument("cell", help="Virtuoso cell name")
    p.add_argument("--m3-track-width", type=int, default=400, metavar="NM",
                   help="M3 track width in nm (default: 400)")
    p.add_argument("--m2-width", type=int, default=280, metavar="NM",
                   help="M2 wire width in nm (default: 280)")
    p.add_argument("--process-lib", default="", metavar="LIB",
                   help="Process library for DRC rules, e.g. tsmc18")
    p.add_argument("--ignore-net", action="append", default=[], metavar="NET",
                   help="Net name to skip routing (repeatable)")
    p.add_argument("--ignore-lib", action="append", default=[], metavar="LIB",
                   help="Library whose instances are excluded from routing (repeatable)")
    p.add_argument("--port", type=int, default=65432,
                   help="Virtuoso bridge TCP port (default: 65432)")
    p.add_argument("--binary", type=Path, default=DEFAULT_BINARY,
                   help=f"Path to autorouter binary (default: {DEFAULT_BINARY})")
    p.add_argument("--verbose", action="store_true",
                   help="Print routing progress to stderr")
    return p.parse_args()


def main() -> int:
    args = parse_args()

    if not args.binary.exists():
        print(
            f"ERROR: binary not found at {args.binary}\n"
            "Build it first:\n"
            "    cd autorouter && go build -o bin/autorouter ./cmd/autorouter/",
            file=sys.stderr,
        )
        return 1

    client = VirtuosoClient.local(port=args.port)

    shapes, instances = read_layout(client, args.lib, args.cell)

    schem = read_schematic(client, args.lib, args.cell)
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
        str(args.binary),
        f"-m3-track-width={args.m3_track_width}",
        f"-m2-width={args.m2_width}",
        *[f"-ignore-net={n}" for n in args.ignore_net],
        *[f"-ignore-lib={l}" for l in args.ignore_lib],
        f"-process-lib={args.process_lib}",
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
    print(f"Routed {len(ok)}/{len(routes)} nets" + (f" ({len(err)} failed)" if err else ""))
    for r in err:
        print(f"  net {r['net_id']} FAILED: {r['error']}", file=sys.stderr)

    drawn = draw_routes(client, args.lib, args.cell, routes)
    print(f"Drew {drawn} shapes into {args.lib}/{args.cell}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
