"""Read the inverter layout from Virtuoso and save it to inv_layout.json."""

from __future__ import annotations

import json
import sys
from pathlib import Path

from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.virtuoso.layout import parse_layout_geometry_output
from virtuoso_bridge.virtuoso.layout.ops import layout_read_geometry

LIB = "test"
CELL = "inv_2"
VIEW = "layout"
OUT = Path(__file__).parent / "inv_layout.json"


def main() -> int:
    client = VirtuosoClient.local(port=65432)

    result = client.execute_skill(layout_read_geometry(LIB, CELL), timeout=30)
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
        "lib": LIB,
        "cell": CELL,
        "view": VIEW,
        "shapes": shapes,
        "instances": instances,
    }
    OUT.write_text(json.dumps(output, indent=2, ensure_ascii=False))
    print(f"Saved {len(instances)} instances, {len(shapes)} shapes to {OUT}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
