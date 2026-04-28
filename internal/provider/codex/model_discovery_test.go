package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverVisibleModelsParsesAppServerModelList(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "codex")
	script := `#!/usr/bin/env python3
import json
import sys

for line in sys.stdin:
    req = json.loads(line)
    if req.get("method") == "initialize":
        print(json.dumps({"jsonrpc":"2.0","id":req["id"],"result":{"serverInfo":{"name":"fake-codex","version":"0.0.0"}}}))
        sys.stdout.flush()
    elif req.get("method") == "model/list":
        print(json.dumps({"jsonrpc":"2.0","id":req["id"],"result":{"items":[
            {"id":"gpt-5.4","displayName":"GPT-5.4","hidden":False},
            {"id":"gpt-5-hidden","displayName":"Hidden","hidden":True},
            {"id":"gpt-5.4-mini","displayName":"GPT-5.4 Mini","hidden":False}
        ]}}))
        sys.stdout.flush()
        break
`
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}

	models, err := discoverVisibleModels(context.Background(), binPath)
	if err != nil {
		t.Fatalf("discoverVisibleModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 visible models, got %d", len(models))
	}
	if models[0].ID != "gpt-5.4" || models[1].ID != "gpt-5.4-mini" {
		t.Fatalf("unexpected model IDs: %+v", models)
	}
}
