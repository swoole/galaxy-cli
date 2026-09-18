package exec

import (
	"galaxy/pkg/galaxycfg"
	"testing"
)

func TestCompleteDefaultsToInteractiveShell(t *testing.T) {
	flags := galaxycfg.NewConfigFlags()
	command := NewCmdExec(flags, galaxycfg.IOStreams{})
	options := newOptions(flags, galaxycfg.IOStreams{})
	if err := options.Complete(command, nil, -1); err != nil {
		t.Fatal(err)
	}
	if len(options.Command) != 1 || options.Command[0] != "sh" || !options.Stdin || !options.TTY {
		t.Fatalf("unexpected defaults: %#v", options)
	}
}

func TestCompleteParsesContainerAndCommand(t *testing.T) {
	flags := galaxycfg.NewConfigFlags()
	command := NewCmdExec(flags, galaxycfg.IOStreams{})
	options := newOptions(flags, galaxycfg.IOStreams{})
	if err := options.Complete(command, []string{"web", "printf", "ok"}, 1); err != nil {
		t.Fatal(err)
	}
	if options.ResourceName != "web" || len(options.Command) != 2 || options.Command[0] != "printf" {
		t.Fatalf("unexpected parse: %#v", options)
	}
}
