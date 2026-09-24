package git

import (
	"fmt"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/utils"
	"github.com/go-git/go-git/v5"
	ssh2 "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/gfile"
	"golang.org/x/crypto/ssh"
)

func GitClone(io galaxycfg.IOStreams, projectRoot string, gitSrc string) error {
	// 当前目录为空
	if !gfile.IsEmpty(projectRoot) {
		return gerror.Newf("当前目录[%s]不为空，不能初始化已有项目", projectRoot)
	}
	_, _ = fmt.Fprintf(io.Out, "开始Clone仓库: %s\n", gitSrc)
	homeFile, keyfile, err := utils.GetSSHProvideKeyfile()
	if err != nil {
		return err
	}
	if !gfile.Exists(keyfile) {
		return gerror.Newf("未在 %s 中找到用于链接git仓库的私钥", homeFile)
	}
	publicKeys, err := ssh2.NewPublicKeysFromFile("git", keyfile, "")
	if err != nil {
		return err
	}
	// 不做hostkey验证
	publicKeys.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	// clone远端项目到本地
	_, err = git.PlainClone(projectRoot, false,
		&git.CloneOptions{
			URL:      gitSrc,
			Progress: io.Out,
			Auth:     publicKeys,
		},
	)
	if err != nil {
		return err
	}
	return nil
}
