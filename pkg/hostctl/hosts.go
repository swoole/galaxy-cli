package hostctl

import (
	"galaxy/pkg/hostctl/file"
	"galaxy/pkg/hostctl/types"
	"github.com/gogf/gf/os/gfile"
	"os"
	"os/exec"
	"syscall"
)

func HostCtlCreateRoute(projectName, domain, resolve string) error {
	if syscall.Getppid() == 1 {
		return nil
	}
	filePath := gfile.SelfPath()

	arg0, e := exec.LookPath(filePath)
	if e != nil {
		return e
	}

	cmd := exec.Command("sudo", arg0, "hostctl", "addRoute", "--profile", projectName, "--ip", resolve, "--hostname", domain)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func CreateRoute(projectName, domain, resolve string) error {
	h, err := file.NewFile(GetDefaultHostFile())
	if err != nil {
		return err
	}
	pf, err := h.GetProfile(projectName)
	if err == nil {
		pf.AddRoute(types.NewRoute(resolve, domain))
	} else if err == types.ErrUnknownProfile {
		err = h.AddRoute(projectName, types.NewRoute(resolve, domain))
		if err != nil {
			return err
		}
	}
	err = h.Flush()
	if err != nil {
		return err
	}
	return nil
}
