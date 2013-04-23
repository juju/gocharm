package hook

func ClearRegistry() {
	hooks = make(map[string]func(ctxt *Context) error)
}
