package version

import (
	"bytes"
	"galaxy/pkg/galaxycfg"
	"strings"
	"testing"
)

func TestVersionContainsCompleteMetadata(t *testing.T) {
	var out bytes.Buffer
	o := newOptions(galaxycfg.IOStreams{Out: &out})
	if err := o.Run(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Git Version", "Git Commit", "Git Commit Time", "Git Tree State", "Go Version",
		"Compiler", "Build Time", "Platform", "上海识沃网络科技有限公司",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("version output is missing %q: %s", expected, out.String())
		}
	}
}
