"""Autoroute a Virtuoso cell using the Go autorouter and draw results back.

Build the binary first:
    cd autorouter && go build -o bin/autorouter ./cmd/autorouter/

Usage:
    python route.py <lib> <cell> --process-lib <LIB> [options]

Example:
    python route.py mylib myinv \
        --process-lib <YOUR_PROCESS> \
        --ignore-net VDD --ignore-net VSS \
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
from virtuoso_bridge.virtuoso.layout.ops import layout_create_label, layout_create_rect, layout_read_geometry
from virtuoso_bridge.virtuoso.ops import save_current_cellview
from virtuoso_bridge.virtuoso.schematic.reader import read_schematic

HERE = Path(__file__).parent
DEFAULT_BINARY = HERE / "autorouter/bin/autorouter"

# Map from autorouter internal layer names to PDK physical layer names.
# Add an entry keyed by the --process-lib name you pass on the command line.
# Physical layer names are PDK-specific — consult your PDK's layer table.
_LAYER_MAPS: dict[str, dict[str, tuple[str, str]]] = {
    # "myprocess": {
    #     "M1":    ("<M1_layer_name>",    "drawing"),
    #     "M2":    ("<M2_layer_name>",    "drawing"),
    #     "M3":    ("<M3_layer_name>",    "drawing"),
    #     "Via12": ("<Via12_layer_name>", "drawing"),
    #     "Via23": ("<Via23_layer_name>", "drawing"),
    # },
}


def nm_to_um(v: int) -> float:
    return v / 1000.0


def layout_create_pin(layer: str, name: str, direction: str,
                      llx: float, lly: float, urx: float, ury: float) -> str:
    cx = (llx + urx) / 2
    cy = (lly + ury) / 2
    pin_skill = (
        f'leCreatePin(cv list("{layer}" "pin") "rectangle" '
        f'list(list({llx:g} {lly:g}) list({urx:g} {ury:g})) '
        f'"{name}" "{direction}" '
        f'list("top" "bottom" "left" "right"))'
    )
    label_skill = layout_create_label(layer, "pin", cx, cy, name, "centerCenter", "R0", "roman", 0.1)
    return f'prog(nil {pin_skill} {label_skill})'


def _skill(client: VirtuosoClient, cmd: str) -> None:
    result = client.execute_skill(cmd, timeout=30)
    if result.status != ExecutionStatus.SUCCESS:
        raise RuntimeError(f"SKILL failed: {result.errors}\n  cmd: {cmd[:120]}")


def _skill_read_boundaries(lib: str, cell: str) -> str:
    return (
        'let((cv b) '
        f'cv = dbOpenCellViewByType("{lib}" "{cell}" "layout" "maskLayout" "r") '
        'unless(cv return("")) '
        'b = cv~>prBoundary '
        'unless(b return("")) '
        'sprintf(nil "shape\\tobjType=%s\\tlayer=prBoundary\\tpurpose=drawing\\tbbox=%L\\n" '
        'b~>objType b~>bBox))'
    )


def _skill_read_instances(lib: str, cell: str) -> str:
    return (
        'prog((cv out) '
        f'cv = dbOpenCellViewByType("{lib}" "{cell}" "layout" "maskLayout" "r") '
        'unless(cv return("")) '
        'out = "" '
        'foreach(inst cv~>instances '
        'out = strcat(out sprintf(nil '
        '"instance\\tname=%s\\tlib=%s\\tcell=%s\\tview=layout\\txy=%L\\torient=%L\\n" '
        'inst~>name inst~>libName inst~>cellName inst~>xy inst~>orient))) '
        'return(out))'
    )


def read_layout(client: VirtuosoClient, lib: str, cell: str) -> tuple[list[dict], list[dict]]:
    result = client.execute_skill(layout_read_geometry(lib, cell), timeout=30)
    raw = result.output or ""
    if raw.startswith('"ERROR') or raw.startswith("ERROR"):
        raise RuntimeError(f"layout read failed: {raw}")
    geometry = result.metadata.get("geometry") or parse_layout_geometry_output(raw)
    shapes = [
        {"layer": obj.get("layer"), "bbox": obj.get("bbox")}
        for obj in geometry
        if obj.get("kind") != "instance"
    ]

    bnd_result = client.execute_skill(_skill_read_boundaries(lib, cell), timeout=30)
    shapes += [
        {"layer": obj.get("layer"), "bbox": obj.get("bbox")}
        for obj in parse_layout_geometry_output(bnd_result.output or "")
    ]

    inst_result = client.execute_skill(_skill_read_instances(lib, cell), timeout=30)
    instances = [
        {
            "name":   obj["name"],
            "lib":    obj["lib"],
            "cell":   obj["cell"],
            "xy":     obj["xy"],
            "orient": obj.get("orient", "R0"),
        }
        for obj in parse_layout_geometry_output(inst_result.output or "")
        if obj.get("kind") == "instance"
    ]
    return shapes, instances


def draw_routes(client: VirtuosoClient, lib: str, cell: str,
                routes: list[dict], layer_map: dict) -> int:
    _skill(client, f'cv = dbOpenCellViewByType("{lib}" "{cell}" "layout" "maskLayout" "a")')
    drawn = 0
    for route in routes:
        if route.get("error"):
            continue
        for shape in route.get("shapes", []):
            ll, ur = shape["lower_left"], shape["upper_right"]
            metal_layer, _ = layer_map[shape["layer"]]
            if shape.get("purpose") == "pin" and shape.get("name"):
                _skill(client, layout_create_pin(
                    metal_layer, shape["name"], "inputOutput",
                    nm_to_um(ll["x"]), nm_to_um(ll["y"]),
                    nm_to_um(ur["x"]), nm_to_um(ur["y"]),
                ))
            else:
                _skill(client, layout_create_rect(
                    metal_layer, "drawing",
                    nm_to_um(ll["x"]), nm_to_um(ll["y"]),
                    nm_to_um(ur["x"]), nm_to_um(ur["y"]),
                ))
            drawn += 1
    _skill(client, save_current_cellview())
    return drawn


def run_calibre(client: VirtuosoClient, lib: str, cell: str, check: str) -> None:
    _skill(client, f'dbOpenCellViewByType("{lib}" "{cell}" "schematic" "schematic" "r")')
    skill_cmd = (
        f'mgc_custom_menus_run_menu_cmd("{check}" '
        f'"::CalibreInterface::execCalibre {check}" \'nil ?code "")'
    )
    print(f"Starting {check} GUI, please wait for the window to pop up...")
    client.execute_skill(skill_cmd, timeout=30)


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Autoroute a Virtuoso cell and draw results back into the layout."
    )
    p.add_argument("lib",  help="Virtuoso library name")
    p.add_argument("cell", help="Virtuoso cell name")
    p.add_argument("--m3-track-width", type=int, required=True, metavar="NM",
                   help="M3 routing track pitch in nm (PDK-specific)")
    p.add_argument("--m2-width", type=int, required=True, metavar="NM",
                   help="M2 wire width in nm (PDK-specific)")
    p.add_argument("--process-lib", required=True, metavar="LIB",
                   help="Process library name; must match a key in _LAYER_MAPS")
    p.add_argument("--ignore-net", action="append", default=[], metavar="NET",
                   help="Net name to skip routing, e.g. power rails (repeatable)")
    p.add_argument("--power-net", action="append", default=[], metavar="NET",
                   help="Net name to route with the power router instead of the signal router (repeatable)")
    p.add_argument("--ignore-lib", action="append", default=["basic"], metavar="LIB",
                   help="Cadence infrastructure library (e.g. basic, analoglib) whose instances have no "
                        "layout and must be skipped during routing (repeatable, default: basic)")
    p.add_argument("--min-overlap-lib", action="append", default=[], metavar="LIB",
                   help="Library whose pins use minimum M2 overlap during routing (repeatable)")
    p.add_argument("--widen-narrow-pins", action="store_true",
                   help="Widen M1 pins narrower than m2-width to m2-width, centered on the pin")
    p.add_argument("--port", type=int, default=65432,
                   help="Virtuoso bridge TCP port (default: 65432)")
    p.add_argument("--binary", type=Path, default=DEFAULT_BINARY,
                   help=f"Path to autorouter binary (default: {DEFAULT_BINARY})")
    p.add_argument("--verbose", action="store_true",
                   help="Print routing progress to stderr")
    p.add_argument("--drc", action="store_true",
                   help="Run Calibre DRC after routing")
    p.add_argument("--lvs", action="store_true",
                   help="Run Calibre LVS after routing")
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

    layer_map = _LAYER_MAPS.get(args.process_lib)
    if layer_map is None:
        print(
            f"ERROR: unknown --process-lib '{args.process_lib}'.\n"
            "Add an entry to _LAYER_MAPS in route.py for your PDK.",
            file=sys.stderr,
        )
        return 1

    client = VirtuosoClient.local(port=args.port)

    answer = input(f"Add routes to layout {args.lib}/{args.cell}? [y/N] ")
    if answer.strip().lower() != "y":
        print("Aborted.")
        return 0

    shapes, instances = read_layout(client, args.lib, args.cell)

    schem = read_schematic(client, args.lib, args.cell)
    nets = {name: net["connections"] for name, net in schem["nets"].items()}
    schem_instances = [
        {"name": inst["name"], "lib": inst["lib"]}
        for inst in schem["instances"]
    ]

    payload = {
        "layout":    {"shapes": shapes, "instances": instances},
        "schematic": {"instances": schem_instances, "nets": nets, "pins": schem.get("pins", {})},
    }

    cmd = [
        str(args.binary),
        f"-m3-track-width={args.m3_track_width}",
        f"-m2-width={args.m2_width}",
        f"-process-lib={args.process_lib}",
        *[f"-ignore-net={n}" for n in args.ignore_net],
        *[f"-power-net={n}" for n in args.power_net],
        *[f"-ignore-lib={l}" for l in args.ignore_lib],
        *[f"-min-overlap-lib={l}" for l in args.min_overlap_lib],
        *(["-widen-narrow-pins"] if args.widen_narrow_pins else []),
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
        net_label = r.get("net_name") or r["net_id"]
        print(f"  net {net_label} FAILED: {r['error']}", file=sys.stderr)

    drawn = draw_routes(client, args.lib, args.cell, routes, layer_map)
    print(f"Drew {drawn} shapes into {args.lib}/{args.cell}")

    if args.drc:
        run_calibre(client, args.lib, args.cell, "DRC")
    if args.lvs:
        run_calibre(client, args.lib, args.cell, "LVS")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
