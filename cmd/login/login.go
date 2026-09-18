package login

import (
	"bufio"
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"galaxy/service/group"
	"galaxy/service/organization"
	"github.com/AlecAivazis/survey/v2"
	"github.com/gogf/gf/util/gconv"
	"github.com/shirou/gopsutil/host"
	"github.com/spf13/cobra"
	"os"
)

type Options struct {
	username string
	password string
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func NewCmdLogin(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "login",
		Short:   "登录 CodeGalaxy 平台",
		Example: `galaxy login --username UserName`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringP("username", "u", "", "用户名")
	cmd.Flags().StringP("password", "p", "", "密码")
	cmd.Flags().Bool("password-stdin", false, "从标准输入(stdin)获取密码")
	return cmd
}
func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	if len(cmd.Flag("username").Value.String()) <= 0 {
		return fmt.Errorf("请执行 `galaxy login --username UserName` 命令登录")
	}
	that.username = cmd.Flag("username").Value.String()
	that.password = cmd.Flag("password").Value.String()
	// 是否使用标准输入读取密码
	passwordStdin := cmd.Flag("password-stdin").Value.String()

	// 如果要使用标准输入读取密码
	if passwordStdin == "true" {
		reader := bufio.NewReader(os.Stdin)
		line, _, err := reader.ReadLine()
		if err != nil {
			return err
		}
		that.password = gconv.String(line)
	}
	// 如果没有找到输入的密码，则提示用户，让用户输入密码
	if len(that.password) <= 0 {
		prompt := &survey.Password{
			Message: "Please type your password:",
		}
		err := survey.AskOne(prompt, &that.password, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (that *Options) Validate() error {

	return nil
}

func (that *Options) Run() error {
	loginReturn, err := httpclient.Login(that.cfgFlags, that.username, that.password)
	if err != nil {
		return fmt.Errorf("login fail，err:%v", err)
	}
	err = that.LoginSaveGalaxyConfig(loginReturn)
	if err != nil {
		return fmt.Errorf("login fail，err:%v", err)
	}
	_, _ = fmt.Fprintln(that.Out, "Login Success.")
	return nil
}

// LoginSaveGalaxyConfig 登录成功后保存配置信息
func (that *Options) LoginSaveGalaxyConfig(loginReturn *protoc.LoginReturn) error {
	var cfg = that.cfgFlags.GalaxyConfig
	svr := cfg.GetServer()
	svr.ApiVersion = buildVariable.ApiVersion
	svr.Token = loginReturn.GetToken()
	svr.DefaultOrgId = loginReturn.LastOrg.GetId()
	svr.User = galaxycfg.User{
		Id:       int(loginReturn.Profile.GetId()),
		Nickname: loginReturn.Profile.GetNickname(),
		Avatar:   loginReturn.Profile.GetAvatar(),
		Email:    loginReturn.Profile.GetEmail(),
	}
	*that.cfgFlags.BearerToken = svr.Token
	var orgs []*protoc.Organization
	orgs, err := organization.NewService(that.cfgFlags).Simple()
	if err != nil {
		return err
	}
	svr.Orgs = nil
	if len(orgs) > 0 {
		for _, org := range orgs {
			remoteGroups, err := group.NewService(that.cfgFlags).Simple(&protoc.GroupListReq{OrgId: org.GetId()})
			if err != nil {
				return err
			}
			var groups []galaxycfg.Group
			for _, remoteGroup := range remoteGroups {
				groups = append(groups, galaxycfg.Group{Id: remoteGroup.GetId(), Title: remoteGroup.GetTitle()})
			}
			var defaultGroupId uint32 = 0
			if len(groups) > 0 {
				defaultGroupId = groups[0].Id
			}
			svr.Orgs = append(svr.Orgs, galaxycfg.Organization{
				Id:             org.GetId(),
				Title:          org.GetTitle(),
				DefaultGroupId: defaultGroupId,
				Groups:         groups,
			})
		}
	}
	hostInfo, err := host.Info()
	if err != nil {
		return err
	}
	s, err := httpclient.Secret(that.username, that.password, hostInfo.HostID)
	if err != nil {
		return err
	}
	svr.Secret = s
	err = cfg.Save(that.cfgFlags.GetGalaxyConfigFile())
	if err != nil {
		return err
	}
	return err
}
