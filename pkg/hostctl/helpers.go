package hostctl

import (
	"os"
	"runtime"
)

func GetDefaultHostFile() string {
	if runtime.GOOS == "linux" {
		return "/etc/hosts" //nolint: goconst
	}

	envHostFile := os.Getenv("HOSTCTL_FILE")
	if envHostFile != "" {
		return envHostFile
	}

	if runtime.GOOS == "windows" {
		return `C:/Windows/System32/Drivers/etc/hosts`
	}

	return "/etc/hosts"
}
