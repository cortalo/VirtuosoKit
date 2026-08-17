"""Export a Calculator-style expression (VT("/fref")) via export_waveform,
convert the OCEAN text dump to CSV, then plot it with matplotlib.

Requires a Maestro session that already has simulation results open (either
a GUI Assembler window with a finished run, or a background session pointed
at a history that has results on disk). This script reuses whichever
session is currently open in Virtuoso -- it does not run a new simulation.

Usage:
    python3 export_calculator_waveform.py
"""

from __future__ import annotations

import csv
import re
import sys
from pathlib import Path

from virtuoso_bridge import VirtuosoClient

EXPRESSION = 'VT("/clk_out")'
ANALYSIS = "tran"
HISTORY = "Interactive.1"  # auto-detect via asiGetResultsDir needs OCEAN's
# current-session pointer set, which isn't guaranteed for a GUI-opened
# session -- pass the history explicitly (seen in the open results window).

# clk_out is a fast PLL output -- its ocnPrint dump is much bigger than
# fref's and can take longer than the client's default 30s per-call
# timeout, so open the client with a bigger default.
CLIENT_TIMEOUT = 120

OUT_DIR = Path(__file__).parent / "output"
RAW_TXT = OUT_DIR / "clk_out.txt"
CSV_PATH = OUT_DIR / "clk_out.csv"
PLOT_PATH = OUT_DIR / "clk_out.png"

_NUMBER = r"[+-]?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?"
_ROW_RE = re.compile(rf"^\s*({_NUMBER})\s+({_NUMBER})\s*$")


def parse_ocean_dump(path: Path) -> tuple[list[float], list[float]]:
    """Pull (x, y) pairs out of an ocnPrint text dump.

    ocnPrint emits a header line plus whitespace-separated numeric rows --
    not comma-separated -- so this skips any line that isn't exactly two
    numbers before handing the data to csv.writer.
    """
    xs: list[float] = []
    ys: list[float] = []
    for line in path.read_text().splitlines():
        m = _ROW_RE.match(line)
        if not m:
            continue
        xs.append(float(m.group(1)))
        ys.append(float(m.group(2)))
    return xs, ys


def main() -> int:
    OUT_DIR.mkdir(exist_ok=True)
    client = VirtuosoClient.local(port=65432, timeout=CLIENT_TIMEOUT)

    session = client.maestro.find_open_session()
    if not session:
        print(
            "No open Maestro session found. Open the Assembler GUI with a "
            "finished simulation history before running this script.",
            file=sys.stderr,
        )
        return 1

    client.maestro.export_waveform(
        session, EXPRESSION, str(RAW_TXT),
        analysis=ANALYSIS, history=HISTORY,
    )
    if not RAW_TXT.exists():
        print(
            f"{RAW_TXT} was never written -- one of the SKILL round-trips "
            f"inside export_waveform likely timed out silently (returns an "
            f"ERROR-status result instead of raising). Try a longer "
            f"CLIENT_TIMEOUT.",
            file=sys.stderr,
        )
        return 1
    print(f"Raw OCEAN dump saved to {RAW_TXT}")

    xs, ys = parse_ocean_dump(RAW_TXT)
    if not xs:
        print(f"No numeric rows parsed from {RAW_TXT}", file=sys.stderr)
        return 1

    with CSV_PATH.open("w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["time", EXPRESSION])
        writer.writerows(zip(xs, ys))
    print(f"Saved {len(xs)} points to {CSV_PATH}")

    import matplotlib.pyplot as plt

    plt.figure(figsize=(8, 4))
    plt.plot(xs, ys)
    plt.xlabel("time (s)")
    plt.ylabel(EXPRESSION)
    plt.title(f"{EXPRESSION} vs time")
    plt.tight_layout()
    plt.show()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
