package initproject

import (
	"errors"
	"galaxy/pkg/galaxycfg"
	"galaxy/protoc"
	"galaxy/service/project"
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/test/gtest"
	"github.com/gogf/gf/text/gstr"
	"github.com/jiangxin/goconfig"
	giturls "github.com/whilp/git-urls"
	"testing"
)

type fakeExistingProjectService struct {
	basic           *protoc.ProjectBasic
	basicErr        error
	repository      *project.Repository
	repositoryErr   error
	repositoryCalls int
}

func (f *fakeExistingProjectService) Basic(_, _ uint32) (*protoc.ProjectBasic, error) {
	return f.basic, f.basicErr
}

func (f *fakeExistingProjectService) Repository(_, _ uint32) (*project.Repository, error) {
	f.repositoryCalls++
	return f.repository, f.repositoryErr
}

func TestFindGitConfig(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		dir, err := goconfig.FindGitConfig(gfile.Pwd())
		t.Assert(err, nil)
		t.Log(dir)
		t.Log(gstr.Replace(dir, "/.git/config", ""))
	})
}

func TestGitConfig(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		config, err := goconfig.Load(gfile.Pwd())
		t.Assert(err, nil)
		t.Log(config.Get("remote.origin.url"))
	})
}

func TestGitUrlParse(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u, err := giturls.Parse("git@git.code-galaxy.net:code-galaxy-inc/galaxy-cli.git")
		t.Assert(err, nil)
		t.Log(u.User.String())
		t.Log(u.Hostname())
		t.Log(u.Port())
	})
}

func TestGitRepositoryHelpers(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		t.Assert(gitRepositoryName("git@git.example.com:team/galaxy-cli.git"), "galaxy-cli")
		t.Assert(gitRepositoryName("https://git.example.com/team/galaxy-cli.git"), "galaxy-cli")
		t.Assert(sameGitURL("git@git.example.com:team/galaxy-cli.git", "git@git.example.com:team/galaxy-cli"), true)
		t.Assert(sameGitURL("https://git.example.com/team/a.git", "https://git.example.com/team/b.git"), false)
	})
}

func TestConfigureExistingWorkingTreeSkipsRepositoryForBuildDisabledProject(t *testing.T) {
	options := &Options{
		remoteUrl: "git@git.code-galaxy.net:projects/business.swoole.com.git",
		cfgFlags: &galaxycfg.ConfigFlags{
			GalaxyConfig: galaxycfg.NewGalaxyConfig(""),
		},
	}
	service := &fakeExistingProjectService{
		basic:         &protoc.ProjectBasic{Id: 9, Title: "商业平台", Develop: false},
		repositoryErr: errors.New("repository must not be requested"),
	}

	configured, err := options.configureExistingWorkingTree(
		service,
		3,
		&protoc.ProjectSimple{Id: 9, Title: "商业平台"},
	)

	if err != nil {
		t.Fatalf("configureExistingWorkingTree() error = %v", err)
	}
	if service.repositoryCalls != 0 {
		t.Fatalf("Repository() calls = %d, want 0", service.repositoryCalls)
	}
	if configured.ProjectId != 9 || configured.GroupId != 3 || configured.Title != "商业平台" {
		t.Fatalf("unexpected project config: %#v", configured)
	}
	if configured.GitSrc != "" {
		t.Fatalf("GitSrc = %q, want empty", configured.GitSrc)
	}
}

func TestConfigureExistingWorkingTreeStillValidatesRepositoryForBuildEnabledProject(t *testing.T) {
	options := &Options{
		remoteUrl: "git@git.code-galaxy.net:projects/current.git",
		cfgFlags: &galaxycfg.ConfigFlags{
			GalaxyConfig: galaxycfg.NewGalaxyConfig(""),
		},
	}
	service := &fakeExistingProjectService{
		basic:      &protoc.ProjectBasic{Id: 9, Title: "代码项目", Develop: true},
		repository: &project.Repository{CloneURL: "git@git.code-galaxy.net:projects/other.git"},
	}

	_, err := options.configureExistingWorkingTree(
		service,
		3,
		&protoc.ProjectSimple{Id: 9, Title: "代码项目"},
	)

	if err == nil {
		t.Fatal("configureExistingWorkingTree() error = nil, want repository mismatch")
	}
	if service.repositoryCalls != 1 {
		t.Fatalf("Repository() calls = %d, want 1", service.repositoryCalls)
	}
}
