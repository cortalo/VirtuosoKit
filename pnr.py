"""Place-and-route a Virtuoso cell (runs place.py then route.py).

Build the binaries first:
    cd placer    && go build -o bin/placer    ./cmd/placer/
    cd autorouter && go build -o bin/autorouter ./cmd/autorouter/

Usage:
    python pnr.py <lib> <cell> [options]

Example:
    python pnr.py test inv_2 \\
        --process-lib tsmc18 \\
        --ignore-net VDD --ignore-net VSS \\
        --ignore-lib basic
"""

import argparse
import subprocess
import sys
from pathlib import Path

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.models import ExecutionStatus

HERE = Path(__file__).parent


def _skill(client: VirtuosoClient, cmd: str) -> None:
    result = client.execute_skill(cmd, timeout=30)
    if result.status != ExecutionStatus.SUCCESS:
        raise RuntimeError(f"SKILL failed: {result.errors}\n  cmd: {cmd[:120]}")


def remove_pr_boundary(client: VirtuosoClient, lib: str, cell: str) -> None:
    _skill(client,
        f'let((cv) '
        f'cv = dbOpenCellViewByType("{lib}" "{cell}" "layout" "maskLayout" "a") '
        f'foreach(shape cv~>shapes '
        f'  when(car(shape~>lpp) == "prBoundary" '
        f'    dbDeleteObject(shape))) '
        f'dbSave(cv) dbClose(cv))'
    )
    print(f"Removed prBoundary from {lib}/{cell}")


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Place and route a Virtuoso cell."
    )
    p.add_argument("lib",  help="Virtuoso library name")
    p.add_argument("cell", help="Virtuoso cell name")

    # place args
    p.add_argument("--row-height", type=int, default=3920, metavar="NM",
                   help="Standard cell row height in nm (default: 3920)")
    p.add_argument("--row-threshold", type=float, default=1.0, metavar="UNITS",
                   help="Y gap threshold in schematic units for row detection (default: 1.0)")
    p.add_argument("--pr-margin", type=int, default=10000, metavar="NM",
                   help="prBoundary margin in nm (default: 10000 = 10 um)")
    p.add_argument("--place-binary", type=Path, default=HERE / "placer/bin/placer",
                   help="Path to placer binary")

    # route args
    p.add_argument("--m3-track-width", type=int, default=400, metavar="NM",
                   help="M3 track width in nm (default: 400)")
    p.add_argument("--m2-width", type=int, default=280, metavar="NM",
                   help="M2 wire width in nm (default: 280)")
    p.add_argument("--process-lib", default="", metavar="LIB",
                   help="Process library for DRC rules, e.g. tsmc18")
    p.add_argument("--ignore-net", action="append", default=[], metavar="NET",
                   help="Net name to skip routing (repeatable)")
    p.add_argument("--route-binary", type=Path, default=HERE / "autorouter/bin/autorouter",
                   help="Path to autorouter binary")

    # shared
    p.add_argument("--ignore-lib", action="append", default=[], metavar="LIB",
                   help="Library to exclude from both placement and routing (repeatable)")
    p.add_argument("--port", type=int, default=65432,
                   help="Virtuoso bridge TCP port (default: 65432)")
    p.add_argument("--verbose", action="store_true",
                   help="Print progress to stderr")
    return p.parse_args()


def main() -> int:
    args = parse_args()

    place_cmd = [
        sys.executable, str(HERE / "place.py"),
        args.lib, args.cell,
        f"--row-height={args.row_height}",
        f"--row-threshold={args.row_threshold}",
        f"--pr-margin={args.pr_margin}",
        f"--binary={args.place_binary}",
        f"--port={args.port}",
        *[f"--ignore-lib={l}" for l in args.ignore_lib],
        *(["--verbose"] if args.verbose else []),
    ]

    route_cmd = [
        sys.executable, str(HERE / "route.py"),
        args.lib, args.cell,
        f"--m3-track-width={args.m3_track_width}",
        f"--m2-width={args.m2_width}",
        f"--process-lib={args.process_lib}",
        f"--binary={args.route_binary}",
        f"--port={args.port}",
        *[f"--ignore-net={n}" for n in args.ignore_net],
        *[f"--ignore-lib={l}" for l in args.ignore_lib],
        *(["--verbose"] if args.verbose else []),
    ]

    print("--- place ---")
    rc = subprocess.run(place_cmd, text=True).returncode
    if rc != 0:
        return rc

    print("--- route ---")
    rc = subprocess.run(route_cmd, input="y\n", text=True).returncode
    if rc != 0:
        return rc

    client = VirtuosoClient.local(port=args.port)
    remove_pr_boundary(client, args.lib, args.cell)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
