package hook

var (
	CtxtGetAllRelationUnit = (*Context).getAllRelationUnit
	CtxtRelationUnits      = (*Context).relationUnits
	CtxtRelationIds        = (*Context).relationIds
	ValidHookName          = validHookName
	ExecHookTools          = &execHookTools
	JujucSymlinks          = &jujucSymlinks
)

type JujucRequest jujucRequest
