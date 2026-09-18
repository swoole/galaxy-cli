package get

import (
	"bytes"
	"galaxy/pkg/galaxycfg"
	"strings"
	"testing"
)

func TestListCommandNameAndAlias(t *testing.T) {
	cmd := NewCmdList("galaxy", galaxycfg.NewConfigFlags(), galaxycfg.IOStreams{})
	if cmd.Name() != "list" {
		t.Fatalf("expected command name list, got %q", cmd.Name())
	}
	if !cmd.HasAlias("ls") {
		t.Fatal("expected list command to provide ls alias")
	}
	if cmd.HasAlias("get") {
		t.Fatal("legacy get alias must not remain available")
	}
}

func TestListWithoutArgumentsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	cmd := NewCmdList("galaxy", galaxycfg.NewConfigFlags(), galaxycfg.IOStreams{Out: &out, ErrOut: &out})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "Usage:") || !strings.Contains(text, "Available Commands:") {
		t.Fatalf("expected list help, got %q", text)
	}
}
