package build

import (
	"github.com/gogf/gf/test/gtest"
	"github.com/spf13/cobra"
	"testing"
)

func TestValidArgsFunc(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		a, err := ValidArgsFunc(nil, []string{}, "")
		t.Assert(err, cobra.ShellCompDirectiveNoFileComp)
		t.Log(a)
	})
}

func TestTagCompletion(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		a, err := tagCompletion(nil, []string{}, "")
		t.Assert(err, cobra.ShellCompDirectiveDefault)
		t.Log(a)
	})
}

func TestPipelineCompletion(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		a, err := pipelineCompletion(nil, []string{}, "")
		t.Assert(err, cobra.ShellCompDirectiveDefault)
		t.Log(a)
	})
}
