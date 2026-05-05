package pathglob

import "testing"

func TestMatch_DoublestarPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{name: "go include nested", pattern: "**/*.go", path: "internal/codeintel/graph/store.go", want: true},
		{name: "yaml include root", pattern: "**/*.yaml", path: "yard.yaml", want: true},
		{name: "exclude node_modules nested", pattern: "**/node_modules/**", path: "web/node_modules/react/index.js", want: true},
		{name: "exclude hidden state dir", pattern: "**/.yard/**", path: ".yard/lancedb/code/0001.lance", want: true},
		{name: "exclude hidden brain", pattern: "**/.brain/**", path: ".brain/notes/hello.md", want: true},
		{name: "minified js", pattern: "**/*.min.js", path: "web/dist/app.min.js", want: true},
		{name: "non-match sibling dir", pattern: "**/node_modules/**", path: "web/src/app.tsx", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Match(tt.pattern, tt.path); got != tt.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchAny_UsesYamlStyleRules(t *testing.T) {
	includes := []string{"**/*.go", "**/*.sql", "**/*.md", "**/*.yaml", "**/*.yml"}
	excludes := []string{"**/.git/**", "**/.yard/**", "**/.brain/**", "**/node_modules/**", "**/vendor/**", "**/dist/**", "**/*.min.js"}

	if !MatchAny(includes, "cmd/tidmouth/serve.go") {
		t.Fatal("expected serve.go to be included")
	}
	if !MatchAny(excludes, "web/node_modules/react/index.js") {
		t.Fatal("expected node_modules path to be excluded")
	}
	if MatchAny(excludes, "internal/server/project.go") {
		t.Fatal("did not expect project.go to be excluded")
	}
}
