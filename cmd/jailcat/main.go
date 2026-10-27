// Command jailcat is a desktop GUI that builds firejail sandboxes for the applications of
// a Linux system and installs them in the application menu and the bin folder (SPEC §1).
package main

import (
	// The dependencies are held here until the packages that use them exist.
	// TODO(step 4.1): remove these blank imports when main uses the packages.
	_ "github.com/rm4n0s/errors"
	_ "github.com/rm4n0s/glutter/material"
	_ "github.com/rm4n0s/glutter/svg"
	_ "gorm.io/driver/sqlite"
	_ "gorm.io/gorm"
)

func main() {}
