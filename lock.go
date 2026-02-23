package main

// Write commands require an exclusive vault lock; all others use a shared lock.
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

func isWriteCommand(cmd string) bool {
	return writeCommands[cmd]
}

// lockFileName is the advisory lock file placed in the vault root.
const lockFileName = ".vlt.lock"
