"""Fetch and plot signals from a live Virtuoso Maestro session.

Resolves a history's raw PSF directory directly and reads it with plain
OCEAN calls (see _find_psf_dir) rather than going through Maestro's
maeOpenResults, which is unreliable once the GUI's focus has moved past
that history. Requires PYTHONPATH pointing at a virtuoso-bridge-lite
checkout new enough to have client.maestro (the embedded copy under
virtuoso-bridge-lite/ in this repo does not).

    plot_signal("fref")
"""

from __future__ import annotations

import re
import tempfile
import time
import uuid
from pathlib import Path

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.virtuoso.maestro.reader.session import natural_sort_histories
from virtuoso_bridge.virtuoso.skill_output import parse_sexpr

_NUMBER = r"[+-]?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?"
_ROW_RE = re.compile(rf"^\s*({_NUMBER})\s+({_NUMBER})\s*$")
_RESULTS_BASE_RE = re.compile(r"(.*/results/maestro)/")


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


def _safe_filename(signal: str) -> str:
    return re.sub(r"[^\w.-]+", "_", signal).strip("_") or "signal"


def _find_psf_dir(client: VirtuosoClient, history: str | None = None) -> str:
    """Locate a history's raw PSF directory directly, bypassing
    maeOpenResults/asiGetResultsDir's unreliable "current results" pointer.

    maeOpenResults(?history=...) is supposed to switch Cadence's internal
    current-results pointer to the requested history, but that switch is
    flaky -- if the GUI's own focus has moved on (e.g. a newer run started),
    asiGetResultsDir keeps reporting whatever's currently GUI-focused
    regardless of which history was requested, and export_waveform then
    raises a "History mismatch" error. It's also unusable mid-simulation:
    it points at an empty `.tmpADEDir_*` staging area until Maestro
    considers the run "finished".

    The actual `results/maestro/<history>/1/<test>/psf/` directory exists
    and is directly readable via plain OCEAN `openResults()` well before
    that, and regardless of GUI focus -- so resolve it ourselves instead.
    `history=None` picks the most recent history (readable even mid-run);
    an explicit `history` targets that one specifically, whether finished
    or still running.
    """
    r = client.execute_skill('asiGetResultsDir(asiGetCurrentSession())')
    rd = (r.output or "").strip('"')
    m = _RESULTS_BASE_RE.match(rd)
    if not m:
        raise RuntimeError(
            f"Could not determine results/maestro base dir from "
            f"asiGetResultsDir={rd!r}; open a Maestro results/waveform "
            "window first."
        )
    base = m.group(1)

    if history is None:
        r2 = client.execute_skill(f'getDirFiles("{base}")')
        entries = parse_sexpr(r2.output or "") or []
        histories = natural_sort_histories(entries)
        if not histories:
            raise RuntimeError(f"No history directories found under {base}")
        history = histories[-1]

    r3 = client.execute_skill(f'getDirFiles("{base}/{history}/1")')
    run_entries = [e for e in (parse_sexpr(r3.output or "") or [])
                   if e not in (".", "..")]
    if not run_entries:
        raise RuntimeError(f"No run directory found under {base}/{history}/1")
    return f"{base}/{history}/1/{run_entries[0]}/psf"


def _export_waveform_direct(
    client: VirtuosoClient,
    expr: str,
    local_path: str,
    *,
    history: str | None = None,
    analysis: str = "tran",
    precision: int = 16,
    width: int = 26,
) -> None:
    """Export an expression by resolving the PSF directory directly (see
    _find_psf_dir), bypassing maeOpenResults/export_waveform's flaky
    current-results-pointer dependency entirely.

    Used whenever `history` is explicitly known (passed in, or `live=True`
    picking the latest one) -- i.e. whenever we don't need to guess. Works
    on a still-running simulation too, in which case `expr` must already
    be time-bounded (e.g. via clip()): an unbounded ocnPrint against a
    live, still-growing results file times out; a bounded one is instant.
    """
    psf_dir = _find_psf_dir(client, history=history)
    print(f"[waveform] psf_dir={psf_dir}")
    remote_path = f"/tmp/vb_direct_{int(time.time() * 1000)}_{uuid.uuid4().hex[:8]}.txt"

    client.execute_skill(f'openResults("{psf_dir}")')
    client.execute_skill(f'selectResults("{analysis}")')
    client.execute_skill(
        f'ocnPrint({expr} '
        f'?numberNotation \'none ?precision {precision} ?width {width} ?numSpaces 1 '
        f'?output "{remote_path}")')

    client.download_file(remote_path, local_path)
    client.execute_skill(f'deleteFile("{remote_path}")')


def get_signal_waveform(
    signal: str,
    *,
    live: bool = False,
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

    The history's PSF directory is always resolved directly (see
    _find_psf_dir) rather than via Maestro's maeOpenResults, which can
    silently fail to switch away from whatever history the GUI currently
    has focused. `history=None` picks the most recent history; passing it
    explicitly targets that one specifically and stays reliable even after
    newer runs happen in the meantime. `session` is currently unused --
    kept for API compatibility with callers that pass it.

    `live=True` reads from the current (possibly still-running) simulation.
    Requires `signal` to already be time-bounded (e.g. `clip(...)`, which
    get_clk_crossings adds automatically when clip_from/clip_to are given)
    -- an unbounded read against a live, still-growing results file times
    out.

    The raw OCEAN dump is written to a throwaway temp directory and deleted
    once parsed. Pass `out_dir=` to keep it (e.g. for inspection/debugging).

    Returns (x, y) lists, e.g. (time, voltage).
    """
    client = client or VirtuosoClient.local(port=65432, timeout=120)
    print("[waveform] connecting to Virtuoso bridge...")

    expr = signal if "(" in signal else f'VT("/{signal}")'
    if live and "clip(" not in expr:
        raise ValueError(
            "live=True requires a time-bounded expression, e.g. pass "
            "clip_from=/clip_to= (get_clk_crossings et al.) or wrap the "
            "expression in clip(...) yourself -- an unbounded read against "
            "a live, still-growing results file times out."
        )

    filename = f"{_safe_filename(signal)}.txt"
    print(f"[waveform] exporting {expr!r} via ocnPrint (this is the slow step)...")

    def _do_export(raw_path: Path) -> None:
        _export_waveform_direct(client, expr, str(raw_path), history=history,
                                 analysis=analysis)
        if not raw_path.exists():
            # execute_skill/download_file report failure via a status field
            # instead of raising, so a timed-out or failed ocnPrint call
            # silently leaves no file here -- surface that immediately
            # rather than letting a bare FileNotFoundError bubble up from
            # the parser several frames down.
            raise RuntimeError(
                f"ocnPrint export never produced {raw_path} -- likely a "
                f"timeout (large clip window / high edge count?) or a "
                f"signal/expression error. Check the printed SKILL "
                f"expression above, or try a narrower clip_from/clip_to "
                f"or a larger --timeout."
            )

    if out_dir is None:
        with tempfile.TemporaryDirectory(prefix="vb_waveform_") as tmp:
            raw_path = Path(tmp) / filename
            _do_export(raw_path)
            print(f"[waveform] export done, parsing {raw_path}...")
            xs, ys = _parse_ocean_dump(raw_path)
    else:
        out_dir = Path(out_dir)
        out_dir.mkdir(exist_ok=True)
        raw_path = out_dir / filename
        _do_export(raw_path)
        print(f"[waveform] export done, parsing {raw_path}...")
        xs, ys = _parse_ocean_dump(raw_path)

    if not xs:
        raise RuntimeError(f"No numeric rows parsed from {filename}")
    print(f"[waveform] parsed {len(xs)} points")
    return xs, ys


def plot_signal(
    signal: str,
    *,
    live: bool = False,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    analysis: str = "tran",
    out_dir: str | Path | None = None,
) -> None:
    """Fetch and show a signal's waveform, e.g. plot_signal("fref")."""
    xs, ys = get_signal_waveform(
        signal, live=live, client=client, session=session, history=history,
        analysis=analysis, out_dir=out_dir,
    )

    import matplotlib.pyplot as plt

    plt.figure(figsize=(8, 4))
    plt.plot(xs, ys)
    plt.xlabel("time (s)" if analysis == "tran" else analysis)
    plt.ylabel(signal)
    plt.title(f"{signal} vs {'time' if analysis == 'tran' else analysis}")
    plt.tight_layout()
    print("[waveform] opening plot window...")
    plt.show()


def get_clk_crossings(
    signal: str = "clk_out",
    threshold: float = 0.5,
    edge: str = "rising",
    *,
    clip_from: float | None = None,
    clip_to: float | None = None,
    live: bool = False,
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

    If both `clip_from` and `clip_to` are given, the signal is clipped to
    that time window first: `clip(VT("/<signal>") clip_from clip_to)`. If
    either is omitted, no clip is applied.

    `live=True` reads from a still-running simulation (see
    get_signal_waveform) -- requires clip_from/clip_to to be set.
    """
    if edge not in ("rising", "falling"):
        raise ValueError(f'edge must be "rising" or "falling", got {edge!r}')
    if live and (clip_from is None or clip_to is None):
        raise ValueError("live=True requires both clip_from and clip_to")

    net_expr = f'VT("/{signal}")'
    if clip_from is not None and clip_to is not None:
        net_expr = f'clip({net_expr} {clip_from} {clip_to})'

    expr = f'cross({net_expr} {threshold} 1 "{edge}"  t "time"  nil )'
    _, crossings = get_signal_waveform(
        expr, live=live, client=client, session=session, history=history,
        analysis="tran", out_dir=out_dir,
    )
    return crossings


def plot_clk_crossings(
    signal: str = "clk_out",
    threshold: float = 0.5,
    edge: str = "rising",
    *,
    clip_from: float | None = None,
    clip_to: float | None = None,
    live: bool = False,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    out_dir: str | Path | None = None,
) -> None:
    """Plot threshold-crossing times of a clock-like signal.

    e.g. plot_clk_crossings("clk_out", 0.5, "falling")
    """
    crossings = get_clk_crossings(
        signal, threshold, edge, clip_from=clip_from, clip_to=clip_to, live=live,
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
    print("[waveform] opening plot window...")
    plt.show()


def get_jitter(
    signal: str,
    f_osc: float,
    threshold: float = 0.5,
    edge: str = "rising",
    *,
    clip_from: float | None = None,
    clip_to: float | None = None,
    live: bool = False,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    out_dir: str | Path | None = None,
):
    """Per-edge timing jitter (seconds) vs. an ideal f_osc clock.

    jitter[n] = crossing[n] - (crossing[0] + n / f_osc) -- referenced to the
    first captured edge, so jitter[0] is always 0.
    """
    import numpy as np

    crossings = get_clk_crossings(
        signal, threshold, edge, clip_from=clip_from, clip_to=clip_to, live=live,
        client=client, session=session, history=history, out_dir=out_dir,
    )

    t = np.asarray(crossings)
    n = np.arange(len(t))
    period = 1.0 / f_osc
    t_ideal = t[0] + n * period
    return t - t_ideal


def compute_rms_jitter(
    signal: str,
    f_osc: float,
    threshold: float = 0.5,
    edge: str = "rising",
    *,
    clip_from: float | None = None,
    clip_to: float | None = None,
    live: bool = False,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    out_dir: str | Path | None = None,
) -> float:
    """RMS timing jitter (seconds): std() of get_jitter()."""
    import numpy as np

    jitter = get_jitter(
        signal, f_osc, threshold, edge, clip_from=clip_from, clip_to=clip_to, live=live,
        client=client, session=session, history=history, out_dir=out_dir,
    )
    return float(np.std(jitter))


def plot_phase_noise(
    signal: str,
    f_osc: float,
    f_min: float,
    f_max: float,
    threshold: float = 0.5,
    edge: str = "rising",
    *,
    points_per_decade: float = 20,
    clip_from: float | None = None,
    clip_to: float | None = None,
    live: bool = False,
    client: VirtuosoClient | None = None,
    session: str | None = None,
    history: str | None = None,
    out_dir: str | Path | None = None,
) -> None:
    """Estimate and plot single-sideband phase noise from clock crossings.

    Pipeline: crossing timestamps -> jitter (vs an ideal f_osc clock) ->
    phase (rad) -> FFT -> one-sided PSD -> /2 for SSB -> dBc/Hz, plotted
    against offset frequency in [f_min, f_max] Hz on a log-x axis.

    The raw per-bin PSD is noisy (single FFT, no averaging), so a
    log-frequency-binned average is overlaid on top -- averaged in linear
    power (not naive dB averaging) at `points_per_decade` bins, to make the
    noise floor visible under the point-to-point fluctuation.

    If both `clip_from` and `clip_to` are given, crossings are only taken
    from that time window (see get_clk_crossings); otherwise the full
    capture is used.

    e.g. plot_phase_noise("clk_out", f_osc=1e9, f_min=1e3, f_max=100e6)
    """
    import numpy as np

    jitter = get_jitter(
        signal, f_osc, threshold, edge, clip_from=clip_from, clip_to=clip_to, live=live,
        client=client, session=session, history=history, out_dir=out_dir,
    )
    print(f"[phase_noise] got {len(jitter)} edges, computing FFT...")
    phase = jitter * 2 * np.pi * f_osc    # radians

    period = 1.0 / f_osc
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
    f_plot = freqs[mask]
    l_db_plot = phase_noise_dbc_hz[mask]

    print(f"[phase_noise] FFT done, smoothing to {points_per_decade:.0f} points/decade...")

    # Log-frequency-bin average, computed in linear power (10^(dB/10)) then
    # converted back to dB -- averaging dB values directly would understate
    # the true average power.
    num_bins = max(1, int(round(points_per_decade * np.log10(f_max / f_min))))
    bin_edges = np.logspace(np.log10(f_min), np.log10(f_max), num_bins + 1)
    bin_idx = np.digitize(f_plot, bin_edges)
    linear_power = 10 ** (l_db_plot / 10)

    smooth_f, smooth_db = [], []
    for b in range(1, len(bin_edges)):
        sel = bin_idx == b
        if not np.any(sel):
            continue
        smooth_f.append(np.sqrt(bin_edges[b - 1] * bin_edges[b]))
        smooth_db.append(10 * np.log10(linear_power[sel].mean()))

    import matplotlib.pyplot as plt

    plt.figure(figsize=(8, 4))
    plt.semilogx(f_plot, l_db_plot, color="0.75", linewidth=0.7, label="raw PSD")
    plt.semilogx(smooth_f, smooth_db, color="C0", linewidth=2,
                 label=f"average ({points_per_decade:.0f}/decade)")
    rms_jitter = np.std(jitter)
    plt.xlabel("offset frequency (Hz)")
    plt.ylabel("phase noise (dBc/Hz)")
    plt.title(f"{signal} phase noise (RMS jitter: {rms_jitter * 1e15:.1f} fs)")
    plt.grid(True, which="both", ls=":")
    plt.legend()
    plt.tight_layout()
    print("[phase_noise] opening plot window...")
    plt.show()
