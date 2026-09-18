package git

import (
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/test/gtest"
	"testing"
)

func TestBranch(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		git := NewGit(gfile.Pwd())
		branch, err := git.CurrentBranch()
		t.Assert(err, nil)
		t.Log(branch)
		commit, err := git.CurrentCommit()
		t.Assert(err, nil)
		t.Log(commit.Message)
		b, err := git.CheckTag("v1.1.1")
		t.Assert(err, nil)
		t.Log(b)
	})
}

func TestNewRemote(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		err := NewRemote()
		t.Assert(err, nil)
	})
}
