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
	NumRows   int               `json:"num_rows"`
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
	targetWidth := flag.Int("target-width", 0, "maximum row width in nm; 0 disables splitting")
	alignRows := flag.Bool("align-rows", false, "pad each row on the right with filler to match the widest row")
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

	for i := range instances {
		if w, err := db.Query(instances[i].Lib, instances[i].Cell); err == nil {
			instances[i].Width = w
		}
	}

	grouped := rows.Group(instances, *rowThreshold)

	if *targetWidth > 0 {
		grouped = rows.SplitByWidth(grouped, *targetWidth)
		grouped = rows.RepackByWidth(grouped, *targetWidth)
	}

	if fc, ok := db.FillerCell(); ok {
		if fw, err := db.Query(fc.Lib, fc.Cell); err == nil {
			grouped = rows.AddFiller(grouped, db.IsFillerCompatible, fc.Lib, fc.Cell, fw)
			if *alignRows {
				grouped = rows.PadToMaxWidth(grouped, fc.Lib, fc.Cell, fw)
			}
		}
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "instances: %d, rows: %d\n", len(instances), len(grouped))
		for i, row := range grouped {
			fmt.Fprintf(os.Stderr, "  row %d: %d cells\n", i, len(row))
		}
	}

	var tapcell *common.TapcellConfig
	if tc, ok := db.Tapcell(); ok {
		tapcell = &tc
	}

	placed, err := place.Place(grouped, db, *rowHeight, tapcell)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: place: %v\n", err)
		os.Exit(1)
	}

	if err := json.NewEncoder(os.Stdout).Encode(response{Instances: placed, NumRows: len(grouped)}); err != nil {
		fmt.Fprintf(os.Stderr, "error: encode response: %v\n", err)
		os.Exit(1)
	}
}
