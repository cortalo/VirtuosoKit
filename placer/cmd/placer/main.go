package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"placer/celldb"
	"placer/common"
	"placer/place"
	"placer/rows"
	"placer/schematic"
	"strings"
)

type response struct {
	Instances []common.Instance `json:"instances"`
}

type ignoreLibFlag []string

func (f *ignoreLibFlag) String() string { return strings.Join(*f, ",") }
func (f *ignoreLibFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func cellsPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "..", "cells.toml"), nil
}

func main() {
	var ignoreLibs ignoreLibFlag
	flag.Var(&ignoreLibs, "ignore-lib", "lib to exclude from placement (repeatable)")
	rowHeight := flag.Int("row-height", 2000, "standard cell row height in nm")
	rowThreshold := flag.Float64("row-threshold", 1.0, "Y gap threshold in schematic units for row detection")
	verbose := flag.Bool("verbose", false, "print placement progress to stderr")
	flag.Parse()

	cellsP, err := cellsPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	db, err := celldb.Load(cellsP)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load cells.toml: %v\n", err)
		os.Exit(1)
	}

	instances, err := schematic.Parse(os.Stdin, ignoreLibs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: parse schematic: %v\n", err)
		os.Exit(1)
	}

	grouped := rows.Group(instances, *rowThreshold)

	if *verbose {
		fmt.Fprintf(os.Stderr, "instances: %d, rows: %d\n", len(instances), len(grouped))
		for i, row := range grouped {
			fmt.Fprintf(os.Stderr, "  row %d: %d cells\n", i, len(row))
		}
	}

	placed, err := place.Place(grouped, db, *rowHeight)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: place: %v\n", err)
		os.Exit(1)
	}

	if err := json.NewEncoder(os.Stdout).Encode(response{Instances: placed}); err != nil {
		fmt.Fprintf(os.Stderr, "error: encode response: %v\n", err)
		os.Exit(1)
	}
}
