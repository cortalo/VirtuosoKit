"""Run the Go autorouter on the inverter and draw the routes into Virtuoso.

Build the binary first (from autorouter/):
    go build -o bin/autorouter ./cmd/autorouter/

Then run:
    python example/route_inv.py
"""

import json
import subprocess
import sys
from pathlib import Path

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.models import ExecutionStatus
from virtuoso_bridge.virtuoso.layout.ops import layout_create_rect
from virtuoso_bridge.virtuoso.ops import save_current_cellview

HERE = Path(__file__).parent
BINARY = HERE / "../autorouter/bin/autorouter"
LAYOUT_FILE = HERE / "inv_layout.json"
SCHEMATIC_FILE = HERE / "inv_schematic.json"

LIB = "test"
CELL = "inv"

M3_TRACK_WIDTH_NM = 400
M2_WIDTH_NM = 230
M3_TRACK_WIDTH_UM = M3_TRACK_WIDTH_NM / 1000.0
M2_LAYER = ("METAL2", "drawing")
M3_LAYER = ("METAL3", "drawing")


def nm_to_um(v: int) -> float:
    return v / 1000.0


def _skill(client: VirtuosoClient, cmd: str) -> None:
    result = client.execute_skill(cmd, timeout=30)
    if result.status != ExecutionStatus.SUCCESS:
        raise RuntimeError(f"SKILL failed: {result.errors}\n  cmd: {cmd[:120]}")


def draw_routes(client: VirtuosoClient, routes: list[dict], ll_y_um: float) -> None:
    _skill(client, f'cv = dbOpenCellViewByType("{LIB}" "{CELL}" "layout" "maskLayout" "a")')

    for route in routes:
        if route.get("error"):
            continue

        m2f = route["m2_from"]
        m2t = route["m2_to"]
        m3  = route["m3"]

        # M2 via on the from-pin side
        _skill(client, layout_create_rect(
            *M2_LAYER,
            nm_to_um(m2f["lower_left"]["x"]),
            nm_to_um(m2f["lower_left"]["y"]),
            nm_to_um(m2f["upper_right"]["x"]),
            nm_to_um(m2f["upper_right"]["y"]),
        ))

        # M2 via on the to-pin side
        _skill(client, layout_create_rect(
            *M2_LAYER,
            nm_to_um(m2t["lower_left"]["x"]),
            nm_to_um(m2t["lower_left"]["y"]),
            nm_to_um(m2t["upper_right"]["x"]),
            nm_to_um(m2t["upper_right"]["y"]),
        ))

        # M3 horizontal track
        track_y_lo = ll_y_um + m3["track_id"] * M3_TRACK_WIDTH_UM
        track_y_hi = track_y_lo + M3_TRACK_WIDTH_UM
        _skill(client, layout_create_rect(
            *M3_LAYER,
            nm_to_um(m3["start"]),
            track_y_lo,
            nm_to_um(m3["end"]),
            track_y_hi,
        ))

    _skill(client, save_current_cellview())


def main() -> int:
    if not BINARY.exists():
        print(
            f"ERROR: binary not found at {BINARY}\n"
            "Build it first:\n"
            "    cd autorouter && go build -o bin/autorouter ./cmd/autorouter/",
            file=sys.stderr,
        )
        return 1

    layout = json.loads(LAYOUT_FILE.read_text())
    schematic = json.loads(SCHEMATIC_FILE.read_text())

    pr = next((s for s in layout["shapes"] if s["layer"] == "prBoundary"), None)
    if pr is None:
        print("ERROR: prBoundary shape not found in layout JSON", file=sys.stderr)
        return 1
    ll_y_um = pr["bbox"][0][1]

    payload = {
        "layout": {"shapes": layout["shapes"], "instances": layout["instances"]},
        "schematic": {"nets": schematic["nets"]},
    }

    proc = subprocess.run(
        [
            str(BINARY),
            f"-m3-track-width={M3_TRACK_WIDTH_NM}",
            f"-m2-width={M2_WIDTH_NM}",
        ],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        print(proc.stderr, file=sys.stderr)
        return proc.returncode

    routes = json.loads(proc.stdout)["routes"]

    ok  = [r for r in routes if not r.get("error")]
    err = [r for r in routes if r.get("error")]
    print(f"Routed {len(ok)}/{len(routes)} nets"
          + (f" ({len(err)} failed)" if err else ""))
    for r in err:
        print(f"  net {r['net_id']} FAILED: {r['error']}", file=sys.stderr)

    client = VirtuosoClient.local(port=65432)
    draw_routes(client, routes, ll_y_um)
    print(f"Drew {len(ok) * 3} shapes into {LIB}/{CELL}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
