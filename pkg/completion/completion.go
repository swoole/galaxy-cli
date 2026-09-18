package completion

import (
	"github.com/gogf/gf/os/gfile"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

const (
	LinuxBash = "/etc/bash_completion.d/galaxy"
	MacBash   = "/usr/local/etc/bash_completion.d/galaxy"
)

type Completion struct {
	os   string
	home string
}

func NewCompletion() *Completion {
	home, _ := gfile.Home()
	return &Completion{
		os:   runtime.GOOS,
		home: home,
	}
}
func (that *Completion) GetPath() string {
	if that.os == "linux" {
		return LinuxBash
	}
	if that.os == "darwin" {
		return MacBash
	}
	return ""
}
func (that *Completion) Save() (string, error) {
	if syscall.Getppid() == 1 {
		return "", nil
	}
	// 将命令行参数中执行文件路径转换成可用路径
	filePath := gfile.SelfPath()

	arg0, e := exec.LookPath(filePath)
	if e != nil {
		return "", e
	}

	cmd := exec.Command(arg0, "completion", "bash")
	cmd.Env = os.Environ()
	shell, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if that.os == "linux" {
		err = gfile.PutBytes(LinuxBash, shell)
		if err != nil {
			return "", err
		}
		return string(shell), nil
	}
	if that.os == "darwin" {
		err = gfile.PutBytes(MacBash, shell)
		if err != nil {
			return "", err
		}
		return string(shell), nil
	}
	return "", nil
}
