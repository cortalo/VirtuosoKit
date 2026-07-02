"""Run the currently-open Maestro simulation and print results.

Attaches to whichever Maestro GUI session is already open in Virtuoso —
no need to specify lib/cell.  Open your tb_rc test in ADE Assembler
before running this script.

Usage::

    python sim_run.py
    python sim_run.py --port 65432
"""

# ---------------------------------------------------------------------------
# Configure simulation parameters and which outputs to print.
# ---------------------------------------------------------------------------
PARAMETERS = {
    "x": "2",   # design variables to set before each run
    "y": "-3",
}
MEASURE_NAMES = ["cost"]  # set to None to print all outputs

import argparse
import os
import time
import uuid

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.virtuoso.maestro import find_open_session, read_results, run_simulation, set_var


def _run_and_wait_local(client: VirtuosoClient, *, session: str = "",
                        timeout: int = 600) -> tuple[str, str]:
    """Like run_and_wait but polls a local /tmp marker file instead of via SSH.

    Works for local Virtuoso connections where no SSH tunnel is active.
    The SKILL callback writes the marker using system(); Python polls it with os.path.exists().
    """
    nonce = uuid.uuid4().hex[:8]
    marker = f"/tmp/vb_sim_done_{nonce}"

    client.execute_skill(f"""
procedure(_vb_sim_done_{nonce}(session runID)
  system(sprintf(nil "echo done > {marker}"))
  printf("[%s sim done] run %L\\n" nth(2 parseString(getCurrentTime())) runID))
""")

    history_raw = run_simulation(client, session=session,
                                 callback=f"_vb_sim_done_{nonce}")
    history = (history_raw or "").strip().strip('"')
    if not history or history == "nil":
        raise RuntimeError(
            "maeRunSimulation returned nil (simulation not started). "
            "Verify at least one analysis is enabled and no modal dialog is blocking."
        )

    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if os.path.exists(marker):
            status = open(marker).read().strip()
            os.remove(marker)
            return history, status or "done"
        time.sleep(2)

    raise TimeoutError(f"Simulation did not finish within {timeout}s")


def read_outputs(client: VirtuosoClient, session: str, history: str) -> dict[str, float]:
    """Return {output_name: float_value} for all Maestro outputs after a run."""
    import csv as _csv
    results = read_results(client, session, history=history, include_raw=True)
    points = results.get("points", [])

    # Fallback: single-point CSV has no "Point" column so _parse_detail_csv skips
    # every row.  Parse the "Test,Output,Nominal,..." header format directly.
    if not points and (raw_csv := results.get("raw_csv")):
        rdr = _csv.reader(raw_csv.splitlines())
        in_data = False
        raw_outputs: dict = {}
        for row in rdr:
            if not row or not any(c.strip() for c in row):
                continue
            if row[0].strip() == "Test" and len(row) >= 2 and row[1].strip() == "Output":
                in_data = True
                continue
            if in_data and len(row) >= 3 and row[1].strip():
                name, value = row[1].strip(), row[2].strip()
                raw_outputs[name] = {"value": value, "spec": row[3].strip() if len(row) > 3 else "", "pass_fail": ""}
        if raw_outputs:
            points = [{"point": 1, "parameters": {}, "outputs": raw_outputs}]

    if not points:
        return {}

    outputs: dict[str, float] = {}
    for name, info in (points[0].get("outputs") or {}).items():
        try:
            outputs[name] = float(info["value"])
        except (ValueError, KeyError):
            pass
    return outputs


def simulate(client: VirtuosoClient, session: str, parameters: dict[str, str],
             timeout: int = 300) -> dict[str, float]:
    """Set design variables, run simulation, return {output_name: value}.

    Args:
        parameters: mapping of variable name to value string, e.g. {"R": "200k"}
    """
    for name, value in parameters.items():
        set_var(client, name, value)
    history, _ = _run_and_wait_local(client, session=session, timeout=timeout)
    return read_outputs(client, session, history)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, default=65432)
    args = parser.parse_args()

    client = VirtuosoClient.local(port=args.port)

    session = find_open_session(client)
    if not session:
        print("ERROR: no open Maestro session found. Open tb_rc in ADE Assembler first.")
        return 1
    print(f"Found session: {session}")

    for name, value in PARAMETERS.items():
        set_var(client, name, value)
        print(f"Set {name} = {value}")

    print("Running simulation ...")
    history, status = _run_and_wait_local(client, session=session, timeout=300)
    print(f"Done: history={history!r}  status={status!r}")


    print("\n=== Results ===")
    outputs = read_outputs(client, session, history)
    if not outputs:
        print("  (no results)")
        return 0

    for name, value in outputs.items():
        if MEASURE_NAMES is not None and name not in MEASURE_NAMES:
            continue
        print(f"  {name} = {value}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
