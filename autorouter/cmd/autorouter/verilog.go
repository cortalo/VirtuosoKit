package main

import (
	"autorouter/common"
	"fmt"
	"sort"
	"strings"
)

func writeVerilog(moduleName string, nl *common.Netlist) string {
	inputs := map[string]struct{}{}
	outputs := map[string]struct{}{}
	var assigns []string

	for _, net := range nl.Nets {
		if net.Driver == "" {
			continue
		}
		drv := toPortName(net.Driver)
		inputs[drv] = struct{}{}

		for _, pin := range net.Pins {
			if pin.Name == "" || pin.Name == net.Driver {
				continue
			}
			sink := toPortName(pin.Name)
			outputs[sink] = struct{}{}
			assigns = append(assigns, fmt.Sprintf("    assign %s = %s;", sink, drv))
		}
	}

	sortedInputs := sortedKeys(inputs)
	sortedOutputs := sortedKeys(outputs)
	sort.Strings(assigns)

	var b strings.Builder
	totalPorts := len(sortedInputs) + len(sortedOutputs)

	if totalPorts == 0 {
		b.WriteString(fmt.Sprintf("module %s ();\nendmodule\n", moduleName))
		return b.String()
	}

	b.WriteString(fmt.Sprintf("module %s (\n", moduleName))
	all := make([]struct{ dir, name string }, 0, totalPorts)
	for _, n := range sortedInputs {
		all = append(all, struct{ dir, name string }{"input", n})
	}
	for _, n := range sortedOutputs {
		all = append(all, struct{ dir, name string }{"output", n})
	}
	for i, p := range all {
		if i < totalPorts-1 {
			b.WriteString(fmt.Sprintf("    %-6s %s,\n", p.dir, p.name))
		} else {
			b.WriteString(fmt.Sprintf("    %-6s %s\n", p.dir, p.name))
		}
	}
	b.WriteString(");\n")

	if len(assigns) > 0 {
		b.WriteString("\n")
		for _, a := range assigns {
			b.WriteString(a + "\n")
		}
	}

	b.WriteString("\nendmodule\n")
	return b.String()
}

func toPortName(instPin string) string {
	return strings.ReplaceAll(instPin, ".", "_")
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
