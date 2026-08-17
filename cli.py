"""Command-line tool for the live Virtuoso session, built on utils/.

Subcommand-based so more functionality can be bolted on later without
reshaping the entry point.

Usage:
    python3 cli.py plot clk_out
    python3 cli.py plot fref --analysis tran --history Interactive.1
"""

import argparse

from virtuoso_bridge import VirtuosoClient

from utils.waveform import plot_signal


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
    plot_p.add_argument("--out-dir", default=None,
                         help="Keep the raw OCEAN dump in this directory instead of "
                              "discarding it after parsing (default: discard)")
    plot_p.set_defaults(func=cmd_plot)

    return p.parse_args()


def main() -> int:
    args = parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
