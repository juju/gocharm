package hook

func (ctxt *Context) SaveState() error {
	return ctxt.saveState()
}

func (ctxt *internalContext) Close() error {
	return ctxt.close()
}

var HookStateDir = &hookStateDir

var NewContext = newContext
