package vlt

// WriteCommands lists CLI commands that require an exclusive vault lock; all others use a shared lock.
var writeCommands = map[string]bool{
	"create":           true,
	"append":           true,
	"prepend":          true,
	"write":            true,
	"patch":            true,
	"move":             true,
	"delete":           true,
	"property:set":     true,
	"property:remove":  true,
	"daily":            true,
	"templates:apply":  true,
	"bookmarks:add":    true,
	"bookmarks:remove": true,
}

// IsWriteCommand returns true if cmd is a write command requiring an exclusive lock.
func IsWriteCommand(cmd string) bool {
	return writeCommands[cmd]
}

// LockFileName is the advisory lock file placed in the vault root.
const LockFileName = ".vlt.lock"
