package git

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gogf/gf/os/gfile"
	"github.com/jiangxin/goconfig"
)

type Git struct {
	cfgPath     string
	projectRoot string
	repo        *git.Repository
}

func NewGit(projectRoot string) *Git {
	return &Git{
		projectRoot: projectRoot,
	}
}

func (that *Git) init() error {
	if len(that.cfgPath) > 0 {
		return nil
	}
	gitCfgFile, err := goconfig.FindGitConfig(that.projectRoot)
	if err != nil {
		return err
	}
	that.cfgPath = gitCfgFile
	return nil
}

// CurrentBranch 获取当前分支
func (that *Git) CurrentBranch() (string, error) {
	err := that.init()
	if err != nil {
		return "", err
	}
	r, err := that.Repository()
	if err != nil {
		return "", err
	}
	h, err := r.Head()
	if err != nil {
		return "", err
	}

	return h.Name().Short(), nil
}

func (that *Git) CurrentCommit() (*object.Commit, error) {
	err := that.init()
	if err != nil {
		return nil, err
	}
	r, err := that.Repository()
	if err != nil {
		return nil, err
	}
	h, err := r.Head()
	if err != nil {
		return nil, err
	}
	commit, err := r.CommitObject(h.Hash())
	return commit, err
}

// CheckTag 检查tag是否存在
func (that *Git) CheckTag(tagName string) (*plumbing.Reference, error) {
	err := that.init()
	if err != nil {
		return nil, err
	}
	r, err := that.Repository()
	if err != nil {
		return nil, err
	}
	ref, err := r.Tag(tagName)
	if err != nil && err.Error() == "tag not found" {
		return nil, err
	}
	return ref, nil
}

// CommitInfo 通过commitId获取到commit信息
func (that *Git) CommitInfo(commitId string) (*object.Commit, error) {
	r, err := that.Repository()
	if err != nil {
		return nil, err
	}
	hash, err := r.ResolveRevision(plumbing.Revision(commitId))
	if err != nil {
		return nil, err
	}
	return r.CommitObject(*hash)
}

func (that *Git) Repository() (*git.Repository, error) {
	err := that.init()
	if err != nil {
		return nil, err
	}
	if that.repo != nil {
		return that.repo, nil
	}
	r, err := git.PlainOpen(that.projectRoot)
	if err != nil {
		return nil, err
	}
	that.repo = r
	return r, nil
}

func NewRemote() error {
	gitCfg, err := goconfig.Load(gfile.Pwd())
	if err != nil {
		return err
	}
	r := git.NewRemote(nil,
		&config.RemoteConfig{Name: gitCfg.Get("origin"), URLs: []string{gitCfg.Get("remote.origin.url")}},
	)
	r1, err := r.List(&git.ListOptions{})
	if err != nil {
		return err
	}
	fmt.Println(r1[0].Hash())
	fmt.Println(r1[1].Hash())
	return nil
}
