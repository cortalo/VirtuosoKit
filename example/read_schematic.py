"""Read the inverter schematic and save minimal connectivity as JSON."""

import json
from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.virtuoso.schematic.reader import read_schematic

client = VirtuosoClient.local(port=65432)

data = read_schematic(client, "test", "inv")

minimal = {
    "instances": [
        {"name": inst["name"], "lib": inst["lib"], "cell": inst["cell"], "terms": inst["terms"]}
        for inst in data["instances"]
    ],
    "nets": {
        name: net["connections"]
        for name, net in data["nets"].items()
    },
}

with open("inv_schematic.json", "w") as f:
    json.dump(minimal, f, indent=2)
print(f"Saved {len(minimal['instances'])} instances, {len(minimal['nets'])} nets to inv_schematic.json")
