"""Bayesian optimization of Virtuoso simulation using Ax.

Maximizes the `cost` output by sweeping design variables `x` and `y`.
Open the tb_rc Maestro session in Virtuoso before running.

Usage::

    python ax_opt.py
    python ax_opt.py --port 65432 --trials 30
"""

# ---------------------------------------------------------------------------
# Configure the optimization
# ---------------------------------------------------------------------------
NUM_TRIALS = 30

import argparse

from ax.service.ax_client import AxClient, ObjectiveProperties

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.virtuoso.maestro import find_open_session
from sim_run import simulate


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--port",   type=int, default=65432)
    parser.add_argument("--trials", type=int, default=NUM_TRIALS)
    args = parser.parse_args()

    client = VirtuosoClient.local(port=args.port)
    session = find_open_session(client)
    if not session:
        print("ERROR: no open Maestro session found.")
        return 1
    print(f"Found session: {session}")

    ax_client = AxClient()
    ax_client.create_experiment(
        name="virtuoso_cost_max",
        parameters=[
            {"name": "x", "type": "range", "bounds": [-10.0, 10.0]},
            {"name": "y", "type": "range", "bounds": [-10.0, 10.0]},
        ],
        objectives={"cost": ObjectiveProperties(minimize=False)},
    )

    for i in range(args.trials):
        parameterization, trial_index = ax_client.get_next_trial()
        x_val = parameterization["x"]
        y_val = parameterization["y"]

        outputs = simulate(client, session, {"x": str(x_val), "y": str(y_val)})
        cost = outputs.get("cost")
        if cost is None:
            print(f"Trial {i:>2}: x={x_val:.4f}, y={y_val:.4f}  — simulation failed, skipping")
            ax_client.abandon_trial(trial_index=trial_index)
            continue

        ax_client.complete_trial(
            trial_index=trial_index,
            raw_data={"cost": (cost, None)},
        )
        print(f"Trial {i:>2}: x={x_val:.4f}, y={y_val:.4f},  cost={cost:.4f}")

    best_parameters, metrics = ax_client.get_best_parameters()
    print("\n===== Best result =====")
    print(f"x     = {best_parameters['x']:.4f}")
    print(f"y     = {best_parameters['y']:.4f}")
    print(f"Metrics: {metrics}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
