"""Fetch and plot signals from a live Virtuoso Maestro session.

Wraps client.maestro.export_waveform() (Calculator-equivalent OCEAN
expressions) plus a matplotlib plot. Requires PYTHONPATH pointing at a
virtuoso-bridge-lite checkout new enough to have client.maestro (the
embedded copy under virtuoso-bridge-lite/ in this repo does not).

    plot_signal("fref")
"""

from __future__ import annotations

import re
import tempfile
from pathlib import Path

from virtuoso_bridge import VirtuosoClient

_NUMBER = r"[+-]?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?"
_ROW_RE = re.compile(rf"^\s*({_NUMBER})\s+({_NUMBER})\s*$")
_HISTORY_RE = re.compile(r"/results/maestro/([^/]+)/")


def _parse_ocean_dump(path: Path) -> tuple[list[float], list[float]]:
    """Pull (x, y) pairs out of an ocnPrint text dump (whitespace-, not
    comma-, separated -- see export_calculator_waveform.py)."""
    xs: list[float] = []
    ys: list[float] = []
    for line in path.read_text().splitlines():
        m = _ROW_RE.match(line)
        if not m:
            continue
        xs.append(float(m.group(1)))
        ys.append(float(m.group(2)))
    return xs, ys


def _autodetect_history(client: VirtuosoClient) -> str:
    """Scan open windows for a `.../results/maestro/<history>/...` path.

    export_waveform's own auto-detect (asiGetResultsDir(asiGetCurrentSession()))
    isn't reliable against a plain GUI session -- it needs OCEAN's current-
    session pointer set, which a maeOpenSetup-opened session doesn't set.
    """
    for window in client.list_windows():
        m = _HISTORY_RE.search(window.get("name", ""))
        if m:
            return m.group(1)
    raise RuntimeError(
        "Could not auto-detect a simulation history from open windows. "
        "Pass history= explicitly (e.g. \"Interactive.36\")."
    )


def _safe_filename(signal: str) -> str:
    return re.sub(r"[^\w.-]+", "_", signal).strip("_") or "signal"


def get_signal_waveform(
    signal: str,
    *,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    analysis: str = "tran",
    out_dir: str | Path | None = None,
) -> tuple[list[float], list[float]]:
    """Fetch a signal's waveform data from the currently open Maestro results.

    `signal` is either a bare net name (wrapped as `VT("/<signal>")`) or a
    full Calculator/OCEAN expression (anything containing "(" is used as-is,
    e.g. `cross(VT("/fref") 0.5 1 "rising" t "time" nil)`).

    The raw OCEAN dump is written to a throwaway temp directory and deleted
    once parsed. Pass `out_dir=` to keep it (e.g. for inspection/debugging).

    Returns (x, y) lists, e.g. (time, voltage).
    """
    client = client or VirtuosoClient.local(port=65432, timeout=120)
    session = session or client.maestro.find_open_session()
    if not session:
        raise RuntimeError(
            "No open Maestro session found; open the Assembler GUI with a "
            "finished simulation history first."
        )
    history = history or _autodetect_history(client)

    expr = signal if "(" in signal else f'VT("/{signal}")'
    filename = f"{_safe_filename(signal)}.txt"

    if out_dir is None:
        with tempfile.TemporaryDirectory(prefix="vb_waveform_") as tmp:
            raw_path = Path(tmp) / filename
            client.maestro.export_waveform(
                session, expr, str(raw_path), analysis=analysis, history=history,
            )
            xs, ys = _parse_ocean_dump(raw_path)
    else:
        out_dir = Path(out_dir)
        out_dir.mkdir(exist_ok=True)
        raw_path = out_dir / filename
        client.maestro.export_waveform(
            session, expr, str(raw_path), analysis=analysis, history=history,
        )
        xs, ys = _parse_ocean_dump(raw_path)

    if not xs:
        raise RuntimeError(f"No numeric rows parsed from {filename}")
    return xs, ys


def plot_signal(
    signal: str,
    *,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    analysis: str = "tran",
    out_dir: str | Path | None = None,
) -> None:
    """Fetch and show a signal's waveform, e.g. plot_signal("fref")."""
    xs, ys = get_signal_waveform(
        signal, client=client, session=session, history=history,
        analysis=analysis, out_dir=out_dir,
    )

    import matplotlib.pyplot as plt

    plt.figure(figsize=(8, 4))
    plt.plot(xs, ys)
    plt.xlabel("time (s)" if analysis == "tran" else analysis)
    plt.ylabel(signal)
    plt.title(f"{signal} vs {'time' if analysis == 'tran' else analysis}")
    plt.tight_layout()
    plt.show()
