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


def get_clk_crossings(
    signal: str = "clk_out",
    threshold: float = 0.5,
    edge: str = "rising",
    *,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    out_dir: str | Path | None = None,
) -> list[float]:
    """Threshold-crossing timestamps (seconds) of a clock-like signal.

    Builds `cross(VT("/<signal>") <threshold> 1 "<edge>" t "time" nil)` --
    edge number is still hardcoded to 1 for now. cross() returns one scalar
    per edge, so ocnPrint dumps identical (time, time) pairs -- the parsed
    "y" column is the crossing-time list returned here.
    """
    if edge not in ("rising", "falling"):
        raise ValueError(f'edge must be "rising" or "falling", got {edge!r}')

    expr = f'cross(VT("/{signal}") {threshold} 1 "{edge}"  t "time"  nil )'
    _, crossings = get_signal_waveform(
        expr, client=client, session=session, history=history,
        analysis="tran", out_dir=out_dir,
    )
    return crossings


def plot_clk_crossings(
    signal: str = "clk_out",
    threshold: float = 0.5,
    edge: str = "rising",
    *,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    out_dir: str | Path | None = None,
) -> None:
    """Plot threshold-crossing times of a clock-like signal.

    e.g. plot_clk_crossings("clk_out", 0.5, "falling")
    """
    crossings = get_clk_crossings(
        signal, threshold, edge,
        client=client, session=session, history=history, out_dir=out_dir,
    )

    import matplotlib.pyplot as plt

    edges = list(range(1, len(crossings) + 1))
    plt.figure(figsize=(8, 4))
    plt.plot(edges, crossings, marker="o")
    plt.xlabel(f"{edge} edge #")
    plt.ylabel("crossing time (s)")
    plt.title(f'{signal} {edge} ({threshold}V) crossing times')
    plt.tight_layout()
    plt.show()


def plot_phase_noise(
    signal: str,
    f_osc: float,
    f_min: float,
    f_max: float,
    threshold: float = 0.5,
    edge: str = "rising",
    *,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    out_dir: str | Path | None = None,
) -> None:
    """Estimate and plot single-sideband phase noise from clock crossings.

    Pipeline: crossing timestamps -> jitter (vs an ideal f_osc clock) ->
    phase (rad) -> FFT -> one-sided PSD -> /2 for SSB -> dBc/Hz, plotted
    against offset frequency in [f_min, f_max] Hz on a log-x axis.

    e.g. plot_phase_noise("clk_out", f_osc=1e9, f_min=1e3, f_max=100e6)
    """
    import numpy as np

    crossings = get_clk_crossings(
        signal, threshold, edge,
        client=client, session=session, history=history, out_dir=out_dir,
    )

    t = np.asarray(crossings)
    n = np.arange(len(t))
    period = 1.0 / f_osc

    t_ideal = t[0] + n * period
    jitter = t - t_ideal                  # seconds
    phase = jitter * 2 * np.pi * f_osc    # radians

    num_samples = len(phase)
    spectrum = np.fft.rfft(phase)
    freqs = np.fft.rfftfreq(num_samples, d=period)

    # One-sided PSD of phase (rad^2/Hz): sample rate is f_osc (one phase
    # sample per ideal period).
    psd = (np.abs(spectrum) ** 2) / (f_osc * num_samples)
    psd[1:-1] *= 2

    # SSB phase noise L(f) = S_phi(f) / 2.
    phase_noise_dbc_hz = 10 * np.log10(psd / 2.0)

    mask = (freqs >= f_min) & (freqs <= f_max)

    import matplotlib.pyplot as plt

    plt.figure(figsize=(8, 4))
    plt.semilogx(freqs[mask], phase_noise_dbc_hz[mask])
    plt.xlabel("offset frequency (Hz)")
    plt.ylabel("phase noise (dBc/Hz)")
    plt.title(f"{signal} phase noise")
    plt.grid(True, which="both", ls=":")
    plt.tight_layout()
    plt.show()
