package main

import (
	"os"

	"github.com/Sora378/codingplantracker/cmd"
	"github.com/Sora378/codingplantracker/internal/tray"
)

func main() {
	if len(os.Args) > 1 {
		cmd.Execute()
		return
	}

	tray.RunTray()
}
