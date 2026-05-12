package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ponchione/sodoryard/internal/projectmemory/contractgen"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("yard-projectmemory-codegen", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	contractOut := fs.String("contract-out", "web/src/generated/yard-project-memory.contract.json", "contract JSON output path")
	typescriptOut := fs.String("typescript-out", "web/src/generated/yard-project-memory.ts", "generated TypeScript output path")
	runtimeImport := fs.String("runtime-import", contractgen.DefaultTypeScriptRuntimeImport, "TypeScript runtime import specifier")
	check := fs.Bool("check", false, "verify generated files are current without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	contractJSON, bindings, err := contractgen.Generate(*runtimeImport)
	if err != nil {
		return err
	}
	if *check {
		if err := checkFile(*contractOut, contractJSON); err != nil {
			return err
		}
		if err := checkFile(*typescriptOut, bindings); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "project memory bindings are current")
		return nil
	}
	if err := writeFile(*contractOut, contractJSON); err != nil {
		return err
	}
	if err := writeFile(*typescriptOut, bindings); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "wrote %s\n", *contractOut)
	fmt.Fprintf(os.Stdout, "wrote %s\n", *typescriptOut)
	return nil
}

func checkFile(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated file %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("generated file %s is stale; run make projectmemory-bindings", path)
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output dir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
