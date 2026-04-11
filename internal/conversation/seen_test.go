package conversation

import (
	"reflect"
	"sync"
	"testing"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
)

var _ contextpkg.SeenFileLookup = (*SeenFiles)(nil)

func TestSeenFilesAddContainsNormalizesAndKeepsFirstSeenTurn(t *testing.T) {
	seen := NewSeenFiles()

	seen.Add("./internal/auth/service.go", 2)
	seen.Add("internal/auth/service.go/", 7)

	ok, turn := seen.Contains("internal/auth/service.go")
	if !ok {
		t.Fatal("Contains returned ok=false, want true")
	}
	if turn != 2 {
		t.Fatalf("Contains returned turn=%d, want 2", turn)
	}

	ok, turn = seen.Contains("./internal/auth/middleware.go")
	if ok || turn != 0 {
		t.Fatalf("Contains returned (%t, %d), want (false, 0)", ok, turn)
	}
}

func TestSeenFilesPathsAndCountReturnNormalizedUniquePaths(t *testing.T) {
	seen := NewSeenFiles()

	seen.Add("./internal/auth/service.go", 2)
	seen.Add("internal/auth/service.go/", 3)
	seen.Add("cmd/sirtopham/./main.go", 4)

	if got := seen.Count(); got != 2 {
		t.Fatalf("Count returned %d, want 2", got)
	}

	want := []string{"cmd/sirtopham/main.go", "internal/auth/service.go"}
	if got := seen.Paths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Paths returned %v, want %v", got, want)
	}
}

func TestSeenFilesConcurrentAccess(t *testing.T) {
	seen := NewSeenFiles()

	paths := []string{
		"./internal/auth/service.go",
		"internal/auth/middleware.go",
		"cmd/sirtopham/main.go",
	}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		for _, path := range paths {
			wg.Add(1)
			go func(path string, turn int) {
				defer wg.Done()
				seen.Add(path, turn)
				seen.Contains(path)
				seen.Paths()
				seen.Count()
			}(path, i+1)
		}
	}
	wg.Wait()

	if got := seen.Count(); got != len(paths) {
		t.Fatalf("Count returned %d after concurrent access, want %d", got, len(paths))
	}
}
