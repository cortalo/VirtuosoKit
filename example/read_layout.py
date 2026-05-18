"""Read the current Virtuoso layout and print it as JSON.

The output has two top-level lists:
  shapes    – rects, paths, polygons, labels from cv->shapes
  instances – placed sub-cells from cv->instances

Run with a layout open in Virtuoso:
    python example/read_layout.py
    python example/read_layout.py > layout.json
"""

from __future__ import annotations

import json
import sys

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.virtuoso.layout import parse_layout_geometry_output
from virtuoso_bridge.virtuoso.layout.ops import layout_read_geometry


def main() -> int:
    client = VirtuosoClient.local(port=65432)

    lib, cell, view = client.get_current_design()
    if not lib or not cell or view != "layout":
        print("ERROR: open a layout cellview in Virtuoso first.", file=sys.stderr)
        return 1

    result = client.execute_skill(layout_read_geometry(lib, cell), timeout=30)
    raw = result.output or ""
    if raw.startswith('"ERROR') or raw.startswith("ERROR"):
        print(raw, file=sys.stderr)
        return 1

    geometry = result.metadata.get("geometry") or parse_layout_geometry_output(raw)

    shapes: list[dict] = []
    instances: list[dict] = []
    for obj in geometry:
        if obj.get("kind") == "instance":
            instances.append({
                "name": obj.get("name"),
                "lib": obj.get("lib"),
                "cell": obj.get("cell"),
                "view": obj.get("view"),
                "xy": obj.get("xy"),
                "orient": obj.get("orient"),
                "bbox": obj.get("bbox"),
            })
        else:
            shapes.append({
                "objType": obj.get("objType"),
                "layer": obj.get("layer"),
                "purpose": obj.get("purpose"),
                "bbox": obj.get("bbox"),
                "points": obj.get("points"),
                "xy": obj.get("xy"),
                "orient": obj.get("orient"),
                "text": obj.get("text"),
            })

    output = {
        "lib": lib,
        "cell": cell,
        "view": view,
        "shapes": shapes,
        "instances": instances,
    }
    print(json.dumps(output, indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
