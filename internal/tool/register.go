package tool

// RegisterFileTools registers all file tools (file_read, file_write, file_edit)
// in the given registry.
func RegisterFileTools(r *Registry) {
	r.Register(FileRead{})
	r.Register(FileWrite{})
	r.Register(FileEdit{})
}
