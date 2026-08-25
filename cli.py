"""Command-line tool for the live Virtuoso session, built on utils/.

Subcommand-based so more functionality can be bolted on later without
reshaping the entry point.

Usage:
    python3 cli.py plot clk_out
    python3 cli.py plot fref --analysis tran --history Interactive.1
    python3 cli.py phase_noise clk_out --f_osc 200e6
"""

import argparse

from virtuoso_bridge import VirtuosoClient

from utils.waveform import plot_phase_noise, plot_signal


def cmd_plot(args: argparse.Namespace) -> int:
    client = VirtuosoClient.local(port=args.port, timeout=args.timeout)
    plot_signal(
        args.signal,
        client=client,
        session=args.session,
        history=args.history,
        analysis=args.analysis,
        out_dir=args.out_dir,
    )
    return 0


def cmd_phase_noise(args: argparse.Namespace) -> int:
    client = VirtuosoClient.local(port=args.port, timeout=args.timeout)
    plot_phase_noise(
        args.signal,
        args.f_osc,
        args.f_min,
        args.f_max,
        args.threshold,
        args.edge,
        client=client,
        session=args.session,
        history=args.history,
        out_dir=args.out_dir,
    )
    return 0


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Virtuoso Bridge command-line tool.")
    p.add_argument("--port", type=int, default=65432,
                   help="Virtuoso bridge TCP port (default: 65432)")
    p.add_argument("--timeout", type=int, default=120,
                   help="Per-call SKILL timeout in seconds (default: 120)")
    sub = p.add_subparsers(dest="command", required=True)

    plot_p = sub.add_parser("plot", help="Plot a transient signal, e.g. `plot clk_out`")
    plot_p.add_argument("signal",
                         help='Signal/net name (e.g. "clk_out") or a full Calculator '
                              'expression (e.g. \'cross(VT("/fref") 0.5 1 "rising" t "time" nil)\')')
    plot_p.add_argument("--analysis", default="tran",
                         help="Analysis to select (default: tran)")
    plot_p.add_argument("--history", default=None,
                         help="Explicit history, e.g. Interactive.1; auto-detected "
                              "from open windows if omitted")
    plot_p.add_argument("--session", default=None,
                         help="Explicit Maestro session; auto-detected if omitted")
    plot_p.add_argument("--out_dir", default=None,
                         help="Keep the raw OCEAN dump in this directory instead of "
                              "discarding it after parsing (default: discard)")
    plot_p.set_defaults(func=cmd_plot)

    pn_p = sub.add_parser("phase_noise",
                           help="Plot SSB phase noise (dBc/Hz) from clock crossings, e.g. `phase_noise clk_out --f_osc 200e6`")
    pn_p.add_argument("signal", help='Signal/net name, e.g. "clk_out"')
    pn_p.add_argument("--f_osc", type=float, required=True, metavar="HZ",
                       help="Nominal oscillator frequency in Hz")
    pn_p.add_argument("--f_min", type=float, default=1e3, metavar="HZ",
                       help="Minimum offset frequency to plot (default: 1e3)")
    pn_p.add_argument("--f_max", type=float, default=100e6, metavar="HZ",
                       help="Maximum offset frequency to plot (default: 100e6)")
    pn_p.add_argument("--threshold", type=float, default=0.5,
                       help="Crossing threshold voltage (default: 0.5)")
    pn_p.add_argument("--edge", choices=["rising", "falling"], default="rising",
                       help="Crossing edge direction (default: rising)")
    pn_p.add_argument("--history", default=None,
                       help="Explicit history, e.g. Interactive.1; auto-detected "
                            "from open windows if omitted")
    pn_p.add_argument("--session", default=None,
                       help="Explicit Maestro session; auto-detected if omitted")
    pn_p.add_argument("--out_dir", default=None,
                       help="Keep the raw OCEAN dump in this directory instead of "
                            "discarding it after parsing (default: discard)")
    pn_p.set_defaults(func=cmd_phase_noise)

    return p.parse_args()


def main() -> int:
    args = parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
