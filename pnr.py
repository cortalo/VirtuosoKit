"""Place-and-route a Virtuoso cell (runs place.py then route.py).

Build the binaries first:
    cd placer     && go build -o bin/placer     ./cmd/placer/
    cd autorouter && go build -o bin/autorouter ./cmd/autorouter/

Usage:
    python pnr.py <lib> <cell> --row-height <NM> --process-lib <LIB> [options]

Example:
    python pnr.py mylib myinv \
        --row-height <ROW_HEIGHT_NM> \
        --process-lib <YOUR_PROCESS> \
        --ignore-net VDD --ignore-net VSS
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


def _via_rects(x0: float, y0: float, x1: float, y1: float,
               cut: float, spacing_x: float, spacing_y: float,
               via_layer: str) -> list[str]:
    """Return SKILL dbCreateRect calls for a centered via cut array."""
    cols = max(1, int((x1 - x0 + spacing_x) / (cut + spacing_x)))
    rows = max(1, int((y1 - y0 + spacing_y) / (cut + spacing_y)))
    sx = (x0 + x1) / 2 - (cols * cut + (cols - 1) * spacing_x) / 2
    sy = (y0 + y1) / 2 - (rows * cut + (rows - 1) * spacing_y) / 2
    cmds = []
    for r in range(rows):
        for c in range(cols):
            lx = sx + c * (cut + spacing_x)
            ly = sy + r * (cut + spacing_y)
            cmds.append(
                f'dbCreateRect(cv list("{via_layer}" "drawing") '
                f'list(list({lx:.6f} {ly:.6f}) list({lx + cut:.6f} {ly + cut:.6f})))'
            )
    return cmds


def extend_power_rails(
    client: VirtuosoClient, lib: str, cell: str,
    m1_layer: str, m2_layer: str, via12_layer: str,
    row_height_nm: int, rail_half_nm: int,
    inst_min_x_nm: int, inst_max_x_nm: int,
    inst_min_y_nm: int, num_rows: int,
    extension_um: float, strap_width_um: float,
    via_cut_um: float, via_spacing_x_um: float, via_spacing_y_um: float,
) -> None:
    """Create M1 power rail extensions and M2 vertical straps.

    Draws horizontal M1 rail extensions at each row boundary, then connects
    them to two vertical M2 straps (one for VDD, one for VSS) with via arrays.
    Even row boundary index → VSS (extend right); odd → VDD (extend left).
    Assumes row 0 has VSS at y=0.
    """
    row_h      = row_height_nm / 1000.0
    half_rail  = rail_half_nm  / 1000.0
    inst_min_x = inst_min_x_nm / 1000.0
    inst_max_x = inst_max_x_nm / 1000.0

    k_start = round(inst_min_y_nm / row_height_nm)
    k_end   = k_start + num_rows

    rects = []

    for k in range(k_start, k_end + 1):
        yc = k * row_h
        y0s, y1s = f"{yc - half_rail:.6f}", f"{yc + half_rail:.6f}"
        if k % 2 == 0:  # VSS: extend right
            rects.append(
                f'dbCreateRect(cv list("{m1_layer}" "drawing") '
                f'list(list({inst_min_x:.6f} {y0s}) list({inst_max_x + extension_um:.6f} {y1s})))'
            )
        else:            # VDD: extend left
            rects.append(
                f'dbCreateRect(cv list("{m1_layer}" "drawing") '
                f'list(list({inst_min_x - extension_um:.6f} {y0s}) list({inst_min_x:.6f} {y1s})))'
            )

    m2_half = strap_width_um / 2.0
    y_bot   = k_start * row_h - half_rail
    y_top   = k_end   * row_h + half_rail
    vdd_xc  = inst_min_x - extension_um + m2_half
    vss_xc  = inst_max_x + extension_um - m2_half
    y_mid   = (y_bot + y_top) / 2

    for xc in (vdd_xc, vss_xc):
        rects.append(
            f'dbCreateRect(cv list("{m2_layer}" "drawing") '
            f'list(list({xc - m2_half:.6f} {y_bot:.6f}) list({xc + m2_half:.6f} {y_top:.6f})))'
        )

    for name, xc in (("VDD", vdd_xc), ("VSS", vss_xc)):
        llx, urx = xc - m2_half, xc + m2_half
        rects.append(
            f'leCreatePin(cv list("{m2_layer}" "pin") "rectangle" '
            f'list(list({llx:.6f} {y_bot:.6f}) list({urx:.6f} {y_top:.6f})) '
            f'"{name}" "inputOutput" list("top" "bottom" "left" "right"))'
        )
        rects.append(
            f'dbCreateLabel(cv list("{m2_layer}" "pin") list({xc:.6f} {y_mid:.6f}) '
            f'"{name}" "centerCenter" "R0" "roman" 1.0)'
        )

    for k in range(k_start, k_end + 1):
        yc = k * row_h
        ox0, ox1 = (vss_xc - m2_half, vss_xc + m2_half) if k % 2 == 0 \
               else (vdd_xc - m2_half, vdd_xc + m2_half)
        rects.extend(_via_rects(ox0, yc - half_rail, ox1, yc + half_rail,
                                via_cut_um, via_spacing_x_um, via_spacing_y_um,
                                via12_layer))

    _skill(client, (
        f'let((cv) '
        f'cv = dbOpenCellViewByType("{lib}" "{cell}" "layout" "maskLayout" "a") '
        + ' '.join(rects) +
        f' dbSave(cv) dbClose(cv))'
    ))
    print(f"Extended {k_end - k_start + 1} M1 power rails and 2 M2 straps in {lib}/{cell}")


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
        description="Place and route a Virtuoso cell."
    )
    p.add_argument("lib",  help="Virtuoso library name")
    p.add_argument("cell", help="Virtuoso cell name")

    # place args
    p.add_argument("--row-height", type=int, required=True, metavar="NM",
                   help="Standard cell row height in nm (PDK-specific, matches the physical cell height)")
    p.add_argument("--row-threshold", type=float, default=1.0, metavar="UNITS",
                   help="Y gap threshold in schematic units for row detection (default: 1.0)")
    p.add_argument("--target-width", type=int, default=0, metavar="NM",
                   help="Maximum row width in nm; 0 disables row splitting (default: 0)")
    p.add_argument("--pr-margin", type=int, default=10000, metavar="NM",
                   help="prBoundary margin in nm (default: 10000 = 10 um)")
    p.add_argument("--place-binary", type=Path, default=HERE / "placer/bin/placer",
                   help="Path to placer binary")

    # route args
    p.add_argument("--m3-track-width", type=int, required=True, metavar="NM",
                   help="M3 routing track pitch in nm (PDK-specific)")
    p.add_argument("--m2-width", type=int, required=True, metavar="NM",
                   help="M2 wire width in nm (PDK-specific)")
    p.add_argument("--process-lib", required=True, metavar="LIB",
                   help="Process library name; must match a key in route.py's _LAYER_MAPS")
    p.add_argument("--ignore-net", action="append", default=[], metavar="NET",
                   help="Net name to skip routing, e.g. power rails (repeatable)")
    p.add_argument("--power-net", action="append", default=[], metavar="NET",
                   help="Net name to route with the power router instead of the signal router (repeatable)")
    p.add_argument("--route-binary", type=Path, default=HERE / "autorouter/bin/autorouter",
                   help="Path to autorouter binary")

    # shared
    p.add_argument("--ignore-lib", action="append", default=["basic"], metavar="LIB",
                   help="Cadence infrastructure library (e.g. basic, analoglib) whose instances have no "
                        "layout and must be skipped (repeatable, default: basic)")
    p.add_argument("--min-overlap-lib", action="append", default=[], metavar="LIB",
                   help="Library whose pins use minimum M2 overlap during routing (repeatable)")
    p.add_argument("--widen-narrow-pins", action="store_true",
                   help="Widen M1 pins narrower than m2-width to m2-width, centered on the pin")
    p.add_argument("--port", type=int, default=65432,
                   help="Virtuoso bridge TCP port (default: 65432)")
    p.add_argument("--verbose", action="store_true",
                   help="Print progress to stderr")

    # power rail post-processing — layer names are PDK-specific
    p.add_argument("--m1-layer", default="", metavar="LAYER",
                   help="PDK physical layer name for M1 power rails")
    p.add_argument("--m2-layer", default="", metavar="LAYER",
                   help="PDK physical layer name for M2 power straps")
    p.add_argument("--via12-layer", default="", metavar="LAYER",
                   help="PDK physical layer name for M1-M2 via cuts")
    p.add_argument("--rail-half", type=int, default=0, metavar="NM",
                   help="Half-width of M1 power rail in nm (PDK-specific; set to 0 to skip rail extension)")
    p.add_argument("--rail-extension", type=float, default=3.0, metavar="UM",
                   help="M1 power rail extension length in um beyond the instance block (default: 3.0)")
    p.add_argument("--strap-width", type=float, default=1.0, metavar="UM",
                   help="M2 power strap width in um (default: 1.0)")
    p.add_argument("--via-cut", type=float, default=0.0, metavar="UM",
                   help="Via cut size in um (PDK-specific)")
    p.add_argument("--via-spacing-x", type=float, default=0.0, metavar="UM",
                   help="Via cut-to-cut spacing in X in um (PDK-specific)")
    p.add_argument("--via-spacing-y", type=float, default=0.0, metavar="UM",
                   help="Via cut-to-cut spacing in Y in um (PDK-specific)")

    # calibre
    p.add_argument("--drc", action="store_true",
                   help="Run Calibre DRC after PnR")
    p.add_argument("--lvs", action="store_true",
                   help="Run Calibre LVS after PnR")
    return p.parse_args()


def main() -> int:
    args = parse_args()

    answer = input(f"Run PnR on {args.lib}/{args.cell}? This will DELETE the existing layout. [y/N] ")
    if answer.strip().lower() != "y":
        print("Aborted.")
        return 0

    place_cmd = [
        sys.executable, str(HERE / "place.py"),
        args.lib, args.cell,
        f"--row-height={args.row_height}",
        f"--row-threshold={args.row_threshold}",
        f"--target-width={args.target_width}",
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
        *[f"--power-net={n}" for n in args.power_net],
        *[f"--ignore-lib={l}" for l in args.ignore_lib],
        *[f"--min-overlap-lib={l}" for l in args.min_overlap_lib],
        *(["--widen-narrow-pins"] if args.widen_narrow_pins else []),
        *(["--verbose"] if args.verbose else []),
    ]

    print("--- place ---")
    place_proc = subprocess.run(place_cmd, input="y\n", stdout=subprocess.PIPE, text=True)
    print(place_proc.stdout, end="")
    if place_proc.returncode != 0:
        return place_proc.returncode

    placement: dict[str, int] = {}
    for line in place_proc.stdout.splitlines():
        if line.startswith("PLACEMENT_SUMMARY "):
            for kv in line[len("PLACEMENT_SUMMARY "):].split():
                k, v = kv.split("=")
                placement[k] = int(v)
            break

    print("--- route ---")
    rc = subprocess.run(route_cmd, input="y\n", text=True).returncode
    if rc != 0:
        return rc

    client = VirtuosoClient.local(port=args.port)

    if args.rail_half > 0 and args.m1_layer and args.m2_layer and args.via12_layer:
        extend_power_rails(
            client, args.lib, args.cell,
            m1_layer       = args.m1_layer,
            m2_layer       = args.m2_layer,
            via12_layer    = args.via12_layer,
            row_height_nm  = args.row_height,
            rail_half_nm   = args.rail_half,
            inst_min_x_nm  = placement["inst_min_x"],
            inst_max_x_nm  = placement["inst_max_x"],
            inst_min_y_nm  = placement["inst_min_y"],
            num_rows       = placement["num_rows"],
            extension_um   = args.rail_extension,
            strap_width_um = args.strap_width,
            via_cut_um       = args.via_cut,
            via_spacing_x_um = args.via_spacing_x,
            via_spacing_y_um = args.via_spacing_y,
        )

    remove_pr_boundary(client, args.lib, args.cell)

    if args.drc:
        run_calibre(client, args.lib, args.cell, "DRC")
    if args.lvs:
        run_calibre(client, args.lib, args.cell, "LVS")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
