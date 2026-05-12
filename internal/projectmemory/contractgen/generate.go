package contractgen

import (
	"fmt"
	"os"

	"github.com/ponchione/shunter"
	"github.com/ponchione/shunter/codegen"

	"github.com/ponchione/sodoryard/internal/projectmemory"
)

const DefaultTypeScriptRuntimeImport = "@shunter/client"

func Generate(runtimeImport string) ([]byte, []byte, error) {
	dataDir, err := os.MkdirTemp("", "sodoryard-projectmemory-contract-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create contract data dir: %w", err)
	}
	defer os.RemoveAll(dataDir)

	rt, err := shunter.Build(projectmemory.NewModule(), shunter.Config{DataDir: dataDir})
	if err != nil {
		return nil, nil, fmt.Errorf("build project memory contract runtime: %w", err)
	}
	defer rt.Close()

	contractJSON, err := rt.ExportContractJSON()
	if err != nil {
		return nil, nil, fmt.Errorf("export project memory contract: %w", err)
	}
	bindings, err := codegen.GenerateFromJSON(contractJSON, codegen.Options{
		Language:                codegen.LanguageTypeScript,
		TypeScriptRuntimeImport: runtimeImport,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("generate project memory TypeScript bindings: %w", err)
	}
	return contractJSON, bindings, nil
}
