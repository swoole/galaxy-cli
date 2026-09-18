package galaxycfg

import (
	"fmt"
	"galaxy/pkg/logger"
	utilpointer "galaxy/pkg/utils/pointer"
	"github.com/gogf/gf/os/genv"
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/os/glog"
	"github.com/spf13/pflag"
)

const (
	flagGalaxyConfig      = "config"
	flagGalaxyProjectRoot = "projectroot"
	flagBearerToken       = "token"
	flagUsername          = "username"
	flagPassword          = "password"
	flagDebug             = "debug"
	flagAPIServer         = "server"
	flagTimeout           = "request-timeout"
	defaultGalaxyBaseUrl  = "https://api.code-galaxy.net"
)

type ConfigFlags struct {
	galaxyConfigFile *string
	optionsCache     *OptionsCache
	//Username *string
	//Password *string
	Timeout *string

	APIServer *string
	//jwt token
	BearerToken *string
	// 配置信息
	GalaxyConfig *GalaxyConfig
	// 项目配置信息
	ProjectConfig *ProjectConfig
	// 项目的根目录
	galaxyProjectRoot *string

	debug bool
	flags *pflag.FlagSet
}

func NewConfigFlags() *ConfigFlags {

	return &ConfigFlags{
		Timeout:           utilpointer.String("0"),
		galaxyConfigFile:  utilpointer.String(getDefaultConfigFile()),
		galaxyProjectRoot: utilpointer.String(""),
		BearerToken:       utilpointer.String(""),
		APIServer:         utilpointer.String(genv.Get("GALAXY_BASE_URL", defaultGalaxyBaseUrl)),
		ProjectConfig:     NewProjectGalaxy(),
		debug:             false,
	}
}

func (that *ConfigFlags) WithDeprecatedPasswordFlag() *ConfigFlags {
	//that.Username = utilpointer.String("")
	//that.Password = utilpointer.String("")
	return that
}

func (that *ConfigFlags) AddFlags(flags *pflag.FlagSet) {
	that.flags = flags
	if that.galaxyConfigFile != nil {
		flags.StringVar(that.galaxyConfigFile, flagGalaxyConfig, *that.galaxyConfigFile, "保存CLI配置的文件")
	}
	if that.galaxyProjectRoot != nil {
		flags.StringVar(that.galaxyProjectRoot, flagGalaxyProjectRoot, gfile.Pwd(), "项目的根目录")
	}
	if that.BearerToken != nil {
		flags.StringVar(that.BearerToken, flagBearerToken, *that.BearerToken, "API server的认证token")
	}

	//if that.Username != nil {
	//	flags.StringVar(that.Username, flagUsername, *that.Username, "API server基本安全认证用户名")
	//}
	//if that.Password != nil {
	//	flags.StringVar(that.Password, flagPassword, *that.Password, "API server的基本安全认证密码")
	//}

	if that.APIServer != nil {
		flags.StringVarP(that.APIServer, flagAPIServer, "s", *that.APIServer, "API server的地址")
	}
	if that.Timeout != nil {
		flags.StringVar(that.Timeout, flagTimeout, *that.Timeout, "单次请求的超时时长，值为0意味着请求不会超时，非零值应包含相应的时间单位(如1s、2m、3h)")
	}
	flags.BoolVar(&that.debug, flagDebug, that.debug, "Debug模式")
}

func (that *ConfigFlags) Load() error {
	if that.debug {
		glog.SetDebug(that.debug)
	} else {
		glog.SetDebug(false)
	}
	// 项目配置需要先于用户配置载入，因为它可以声明本项目绑定的 API
	// Server。命令行显式传入的 --server 始终拥有最高优先级。
	err := that.ProjectConfig.Load(that.GalaxyProjectRoot())
	if err != nil {
		return err
	}
	serverExplicit := that.flags != nil && that.flags.Changed(flagAPIServer)
	if !serverExplicit && that.ProjectConfig.Server != "" {
		*that.APIServer = that.ProjectConfig.Server
	}

	that.GalaxyConfig = NewGalaxyConfig(that.GetAPIServer())
	err = that.GalaxyConfig.Load(that.GetGalaxyConfigFile())
	if err != nil {
		return err
	}
	defaultOrg := that.GalaxyConfig.GetDefaultOrg()
	if defaultOrg == nil {
		return nil
	}
	// 确保参数没有传入的时候使用配置文件中的token
	if len(*that.BearerToken) == 0 {
		*that.BearerToken = that.GalaxyConfig.GetToken()
	}
	if that.optionsCache == nil {
		that.optionsCache = NewOptionsCache(that)
	}

	return nil
}

// GetAPIServer 获取服务器api地址
func (that *ConfigFlags) GetAPIServer() string {
	if len(*that.APIServer) > 0 {
		return *that.APIServer
	}
	return genv.Get("GALAXY_BASE_URL", defaultGalaxyBaseUrl)
}

// GetGalaxyConfigFile 获取配置文件路径
func (that *ConfigFlags) GetGalaxyConfigFile() string {
	if len(*that.galaxyConfigFile) > 0 {
		return *that.galaxyConfigFile
	}
	return getDefaultConfigFile()
}

// GalaxyProjectRoot 项目的根目录
func (that *ConfigFlags) GalaxyProjectRoot() string {
	return *that.galaxyProjectRoot
}

// InProject 判断当前目录是否是project目录
func (that *ConfigFlags) InProject() bool {
	return that.ProjectConfig.InProject(that.GalaxyProjectRoot())
}

func (that *ConfigFlags) OptionsCache() *OptionsCache {
	if that.optionsCache == nil {
		that.optionsCache = NewOptionsCache(that)
	}

	return that.optionsCache
}

// 获取默认配置文件路径
func getDefaultConfigFile() string {
	home, err := gfile.Home()
	if err != nil {
		logger.Println(err)
		home = gfile.TempDir()
	}

	cfgDir := fmt.Sprintf("%s%s.galaxy", home, gfile.Separator)
	if !gfile.IsDir(cfgDir) {
		_ = gfile.Mkdir(cfgDir)
	}
	return fmt.Sprintf("%s%sconfig", cfgDir, gfile.Separator)
}
