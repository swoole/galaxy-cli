package git

import (
	"fmt"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/logger"
	"galaxy/pkg/utils"
	"github.com/go-git/go-git/v5"
	ssh2 "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/gfile"
	giturls "github.com/whilp/git-urls"
	"golang.org/x/crypto/ssh"
	"time"
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

func SshTest(gitSrc string) (bool, error) {
	u, err := giturls.Parse(gitSrc)
	if err != nil {
		return false, err
	}
	var sshAuth []ssh.AuthMethod
	password, ok := u.User.Password()
	if ok {
		sshAuth = append(sshAuth, ssh.Password(password))
	} else {
		homeFile, keyfile, err := utils.GetSSHProvideKeyfile()
		if err != nil {
			return false, err
		}
		if !gfile.Exists(keyfile) {
			return false, gerror.Newf("未在 %s 中找到用于链接git仓库的私钥", homeFile)
		}
		privateKey := gfile.GetBytes(keyfile)
		signer, err := ssh.ParsePrivateKey(privateKey)
		if err != nil {
			return false, err
		}
		sshAuth = append(sshAuth, ssh.PublicKeys(signer))
	}
	cfg := &ssh.ClientConfig{
		User:            u.User.Username(),
		Auth:            sshAuth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}

	_, err = ssh.Dial("tcp", fmt.Sprintf("%s:22", u.Hostname()), cfg)
	if err != nil {
		logger.Println(err)
		return false, nil
	}
	return true, nil
}
