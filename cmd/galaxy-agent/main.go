package main

import (
	"os"

	dockercmd "galaxy/cmd/docker"
	"galaxy/pkg/galaxycfg"
)

func main() {
	flags := galaxycfg.NewConfigFlags()
	command := dockercmd.NewStandaloneCmdAgent(flags, galaxycfg.IOStreams{
		In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr,
	})
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
