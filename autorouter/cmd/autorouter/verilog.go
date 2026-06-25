package main

import (
	"autorouter/common"
	"fmt"
	"sort"
	"strings"
)

func writeVerilog(moduleName string, nl *common.Netlist) string {
	ports := map[string]struct{}{}
	var assigns []string

	portNames := make(map[string]struct{}, len(nl.Pins))
	for _, p := range nl.Pins {
		portNames[p.Name] = struct{}{}
	}

	for _, net := range nl.Nets {
		// Collect all identifiers for this net: instance pins first, then top-level port name.
		var idents []string
		for _, pin := range net.Pins {
			if pin.Name != "" {
				idents = append(idents, toPortName(pin.Name))
			}
		}
		if _, isPort := portNames[net.Name]; isPort {
			idents = append(idents, toPortName(net.Name))
		}
		if len(idents) < 2 {
			continue
		}
		drv := idents[0]
		ports[drv] = struct{}{}
		for _, sink := range idents[1:] {
			ports[sink] = struct{}{}
			assigns = append(assigns, fmt.Sprintf("    assign %s = %s;", sink, drv))
		}
	}

	sortedPorts := sortedKeys(ports)
	sort.Strings(assigns)

	var b strings.Builder

	if len(sortedPorts) == 0 {
		b.WriteString(fmt.Sprintf("module %s ();\nendmodule\n", moduleName))
		return b.String()
	}

	b.WriteString(fmt.Sprintf("module %s (\n", moduleName))
	for i, p := range sortedPorts {
		if i < len(sortedPorts)-1 {
			b.WriteString(fmt.Sprintf("    inout %s,\n", p))
		} else {
			b.WriteString(fmt.Sprintf("    inout %s\n", p))
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

func toPortName(s string) string {
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "<", "_")
	s = strings.ReplaceAll(s, ">", "")
	return s
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
