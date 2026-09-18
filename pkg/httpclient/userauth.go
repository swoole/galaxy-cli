package httpclient

import (
	"fmt"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"galaxy/protoc"
	"github.com/fatih/color"
	"github.com/gogf/gf/crypto/gaes"
	"github.com/gogf/gf/encoding/gbase64"
	"github.com/gogf/gf/encoding/gjson"
	"github.com/gogf/gf/frame/g"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
)

func LoginBySecret(cfgFlags *galaxycfg.ConfigFlags) error {
	hostInfo, err := host.Info()
	if err != nil {
		return err
	}
	err = cfgFlags.Load()
	if err != nil {
		return err
	}
	if len(cfgFlags.GalaxyConfig.GetSecret()) == 0 {
		return fmt.Errorf("你还未登录%s,请使用 `%s` 命令登录", color.GreenString("CodeGalaxy"), color.BlueString("galaxy login"))
	}
	username, password, err := UnSecret(cfgFlags.GalaxyConfig.GetSecret(), hostInfo.HostID)
	if err != nil {
		return err
	}
	loginReturn, err := Login(cfgFlags, username, password)
	if err != nil {
		return err
	}
	cfgFlags.GalaxyConfig.SetToken(loginReturn.GetToken())
	cfgFlags.GalaxyConfig.SetOrgId(loginReturn.LastOrg.GetId())
	cfgFlags.GalaxyConfig.SetUser(galaxycfg.User{
		Id:       int(loginReturn.Profile.GetId()),
		Nickname: loginReturn.Profile.GetNickname(),
		Avatar:   loginReturn.Profile.GetAvatar(),
		Email:    loginReturn.Profile.GetEmail(),
	})
	*cfgFlags.BearerToken = cfgFlags.GalaxyConfig.GetToken()
	err = cfgFlags.GalaxyConfig.Save(cfgFlags.GetGalaxyConfigFile())
	if err != nil {
		return err
	}
	return nil
}

func Login(cfgFlags *galaxycfg.ConfigFlags, username, password string) (*protoc.LoginReturn, error) {
	hostInfo, err := host.Info()
	if err != nil {
		return nil, err
	}
	cpuCount, err := cpu.Counts(false)
	if err != nil {
		return nil, err
	}
	cpuInfo, err := cpu.Info()
	if err != nil {
		return nil, err
	}
	cliLogin := protoc.CliLoginReq{
		Account:  username,
		Password: password,
		Hostinfo: &protoc.HostInfo{
			Hostname:        hostInfo.Hostname,
			Hostid:          hostInfo.HostID,
			Os:              hostInfo.OS,
			Platform:        hostInfo.Platform,
			PlatformFamily:  hostInfo.PlatformFamily,
			PlatformVersion: hostInfo.PlatformVersion,
			KernelVersion:   hostInfo.KernelVersion,
			KernelArch:      hostInfo.KernelArch,
			CpuCount:        int32(cpuCount),
			CpuModel:        cpuInfo[0].ModelName,
		},
		Cliinfo: &protoc.CliInfo{
			Version: buildVariable.BuildVersion,
		},
	}
	return NewService(cfgFlags).Login(&cliLogin)
}
func Secret(username, password, key string) (string, error) {

	secret, err := gaes.Encrypt(gjson.New(g.Map{"username": username, "password": password}).MustToJson(), padKey([]byte(key)))
	return gbase64.EncodeToString(secret), err
}

func UnSecret(secret string, key string) (username string, password string, err error) {
	data, err := gbase64.DecodeString(secret)
	if err != nil {
		return "", "", err
	}
	data, err = gaes.Decrypt(data, padKey([]byte(key)))
	if err != nil {
		return "", "", err
	}
	json := gjson.New(data)
	return json.GetString("username"), json.GetString("password"), nil
}

// 填充key
func padKey(key []byte) []byte {
	if len(key) >= 32 {
		return key[0:32]
	}
	padSize := 32 - len(key)
	key = append(key, make([]byte, padSize)...)
	return key[0:32]
}
