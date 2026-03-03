package tool

// DefaultRegistry creates a registry with all core tools.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewReadFile())
	r.Register(NewWriteFile())
	r.Register(NewEditFile())
	r.Register(NewApplyPatch())
	r.Register(NewShell())
	r.Register(NewGrep())
	r.Register(NewGlob())
	return r
}
