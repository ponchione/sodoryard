package toolgroup

const (
	Brain     = "brain"
	File      = "file"
	FileRead  = "file:read"
	Git       = "git"
	Shell     = "shell"
	Search    = "search"
	Directory = "directory"
	Test      = "test"
	SQLC      = "sqlc"
)

var names = []string{
	Brain,
	File,
	FileRead,
	Git,
	Shell,
	Search,
	Directory,
	Test,
	SQLC,
}

var known = func() map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}()

func Names() []string {
	return append([]string(nil), names...)
}

func IsKnown(name string) bool {
	_, ok := known[name]
	return ok
}

func Message() string {
	return "brain, file, file:read, git, shell, search, directory, test, or sqlc"
}
