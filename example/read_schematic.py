"""Read schematic and dump instances, nets, and pins as JSON."""

import json
from virtuoso_bridge import VirtuosoClient
from virtuoso_bridge.virtuoso.schematic.reader import read_schematic

client = VirtuosoClient.local(port=65432)

data = read_schematic(client, "test", "inv_2")

instances = [
    {"name": inst["name"], "lib": inst["lib"], "cell": inst["cell"]}
    for inst in data["instances"]
]

nets = {name: net["connections"] for name, net in data["nets"].items()}

print(json.dumps({"instances": instances, "nets": nets, "pins": data["pins"]}, indent=2))
