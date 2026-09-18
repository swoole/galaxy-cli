package galaxycfg

import (
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestLoadUsesProjectServer(t *testing.T) {
	root := t.TempDir()
	projectConfig := &ProjectConfig{
		Server:           "http://192.168.1.4:9501/",
		DefaultProjectId: 9,
		Projects:         []Project{{ProjectId: 9, Title: "laravel-hello"}},
	}
	if err := projectConfig.Save(root); err != nil {
		t.Fatal(err)
	}

	flags := NewConfigFlags()
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.AddFlags(flagSet)
	if err := flagSet.Set("projectroot", root); err != nil {
		t.Fatal(err)
	}
	if err := flagSet.Set("config", filepath.Join(root, "user-config")); err != nil {
		t.Fatal(err)
	}
	if err := flags.Load(); err != nil {
		t.Fatal(err)
	}
	if got := flags.GetAPIServer(); got != "http://192.168.1.4:9501" {
		t.Fatalf("expected project server, got %q", got)
	}
}

func TestExplicitServerOverridesProjectServer(t *testing.T) {
	root := t.TempDir()
	projectConfig := &ProjectConfig{Server: "http://project.example:9501"}
	if err := projectConfig.Save(root); err != nil {
		t.Fatal(err)
	}

	flags := NewConfigFlags()
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.AddFlags(flagSet)
	if err := flagSet.Set("projectroot", root); err != nil {
		t.Fatal(err)
	}
	if err := flagSet.Set("config", filepath.Join(root, "user-config")); err != nil {
		t.Fatal(err)
	}
	if err := flagSet.Set("server", "http://explicit.example:9501"); err != nil {
		t.Fatal(err)
	}
	if err := flags.Load(); err != nil {
		t.Fatal(err)
	}
	if got := flags.GetAPIServer(); got != "http://explicit.example:9501" {
		t.Fatalf("expected explicit server, got %q", got)
	}
}
