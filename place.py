"""Place a Virtuoso cell using the Go placer and create the layout.

Build the binary first:
    cd placer && go build -o bin/placer ./cmd/placer/

Usage:
    python place.py <lib> <cell> [options]

Example:
    python place.py test inv_2 --ignore-lib basic
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.models import ExecutionStatus
from virtuoso_bridge.virtuoso.layout.ops import layout_create_param_inst, layout_create_rect
from virtuoso_bridge.virtuoso.ops import save_current_cellview
from virtuoso_bridge.virtuoso.schematic.reader import read_schematic

HERE = Path(__file__).parent
DEFAULT_BINARY = HERE / "placer/bin/placer"


def nm_to_um(v: int) -> float:
    return v / 1000.0


def _skill(client: VirtuosoClient, cmd: str) -> str:
    result = client.execute_skill(cmd, timeout=30)
    if result.status != ExecutionStatus.SUCCESS:
        raise RuntimeError(f"SKILL failed: {result.errors}\n  cmd: {cmd[:120]}")
    return result.output or ""


def layout_exists(client: VirtuosoClient, lib: str, cell: str) -> bool:
    out = _skill(client,
        f'let((cv) cv = dbOpenCellViewByType("{lib}" "{cell}" "layout" "maskLayout" "r") '
        f'if(cv then dbClose(cv) "yes" else "no"))')
    return "yes" in out


def delete_layout(client: VirtuosoClient, lib: str, cell: str) -> None:
    _skill(client,
        f'let((ddcv) ddcv = ddGetObj("{lib}" "{cell}" "layout") '
        f'if(ddcv then ddDeleteObj(ddcv) "deleted" else "not found"))')


def create_layout(client: VirtuosoClient, lib: str, cell: str, instances: list[dict], pr_margin_nm: int) -> None:
    _skill(client, f'cv = dbOpenCellViewByType("{lib}" "{cell}" "layout" "maskLayout" "w")')
    for inst in instances:
        _skill(client, layout_create_param_inst(
            inst["lib"], inst["cell"], "layout",
            inst["name"],
            nm_to_um(inst["x"]), nm_to_um(inst["y"]),
            inst["orient"],
        ))
    xs = [inst["x"] for inst in instances]
    ys = [inst["y"] for inst in instances]
    llx = nm_to_um(min(xs) - pr_margin_nm)
    lly = nm_to_um(min(ys) - pr_margin_nm)
    urx = nm_to_um(max(xs) + pr_margin_nm)
    ury = nm_to_um(max(ys) + pr_margin_nm)
    _skill(client, layout_create_rect("prBoundary", "drawing", llx, lly, urx, ury))
    _skill(client, save_current_cellview())


def bbox_center(bbox):
    if not bbox or len(bbox) < 2:
        return None
    return [(bbox[0][0] + bbox[1][0]) / 2, (bbox[0][1] + bbox[1][1]) / 2]


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Place a Virtuoso cell and create the layout."
    )
    p.add_argument("lib",  help="Virtuoso library name")
    p.add_argument("cell", help="Virtuoso cell name")
    p.add_argument("--row-height", type=int, default=3920, metavar="NM",
                   help="Standard cell row height in nm (default: 3920)")
    p.add_argument("--row-threshold", type=float, default=1.0, metavar="UNITS",
                   help="Y gap threshold in schematic units for row detection (default: 1.0)")
    p.add_argument("--pr-margin", type=int, default=10000, metavar="NM",
                   help="prBoundary margin around instances in nm (default: 10000 = 10 um)")
    p.add_argument("--ignore-lib", action="append", default=[], metavar="LIB",
                   help="Library whose instances are excluded from placement (repeatable)")
    p.add_argument("--port", type=int, default=65432,
                   help="Virtuoso bridge TCP port (default: 65432)")
    p.add_argument("--binary", type=Path, default=DEFAULT_BINARY,
                   help=f"Path to placer binary (default: {DEFAULT_BINARY})")
    p.add_argument("--verbose", action="store_true",
                   help="Print placement progress to stderr")
    return p.parse_args()


def main() -> int:
    args = parse_args()

    if not args.binary.exists():
        print(
            f"ERROR: binary not found at {args.binary}\n"
            "Build it first:\n"
            "    cd placer && go build -o bin/placer ./cmd/placer/",
            file=sys.stderr,
        )
        return 1

    client = VirtuosoClient.local(port=args.port)

    # Check if layout already exists before doing any work
    if layout_exists(client, args.lib, args.cell):
        answer = input(f"Layout {args.lib}/{args.cell} already exists. Delete and recreate? [y/N] ")
        if answer.strip().lower() != "y":
            print("Aborted.")
            return 0

    # Read schematic instance positions
    data = read_schematic(client, args.lib, args.cell, include_positions=True)
    payload = {
        "instances": [
            {
                "name": inst["name"],
                "lib":  inst["lib"],
                "cell": inst["cell"],
                "xy":   bbox_center(inst.get("bBox")),
            }
            for inst in data["instances"]
            if bbox_center(inst.get("bBox")) is not None
        ]
    }

    # Run placer
    cmd = [
        str(args.binary),
        f"-row-height={args.row_height}",
        f"-row-threshold={args.row_threshold}",
        *[f"-ignore-lib={l}" for l in args.ignore_lib],
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

    instances = json.loads(proc.stdout)["instances"]
    print(f"Placed {len(instances)} instances")
    if args.verbose:
        for inst in instances:
            print(f"  {inst['name']:20s}  ({nm_to_um(inst['x']):.3f}, {nm_to_um(inst['y']):.3f}) um  {inst['orient']}", file=sys.stderr)

    # Delete old layout if it existed
    if layout_exists(client, args.lib, args.cell):
        delete_layout(client, args.lib, args.cell)

    # Create layout
    create_layout(client, args.lib, args.cell, instances, args.pr_margin)
    print(f"Created layout {args.lib}/{args.cell} with {len(instances)} instances.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
