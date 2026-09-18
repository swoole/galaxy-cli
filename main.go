package main

import (
	"galaxy/cmd"
	"os"
)

func main() {
	c := cmd.NewDefaultGalaxyCommand()
	if err := c.Execute(); err != nil {
		os.Exit(1)
	}
}
