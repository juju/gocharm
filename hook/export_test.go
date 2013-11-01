package hook

func (ctxt *Context) SaveState() error {
	return ctxt.saveState()
}

var HookStateDir = &hookStateDir
