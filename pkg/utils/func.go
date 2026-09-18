package utils

import (
	"fmt"
	"github.com/gogf/gf/os/gfile"
)

func GetSSHProvideKeyfile() (string, string, error) {
	// 私钥的名称
	var keyFiles = []string{
		"id_rsa",
		"id_ed25519",
	}
	homePath, err := gfile.Home(".ssh")
	if err != nil {
		return "", "", err
	}
	for _, k := range keyFiles {
		keyfile := fmt.Sprintf("%s%s%s", homePath, gfile.Separator, k)
		if gfile.Exists(keyfile) {
			return homePath, keyfile, nil
		}
	}
	return homePath, "", nil
}
