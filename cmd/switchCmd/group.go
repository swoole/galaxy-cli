package switchCmd

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/protoc"
	"galaxy/service/group"
	"github.com/AlecAivazis/survey/v2"
	"github.com/gogf/gf/errors/gerror"
	"github.com/spf13/cobra"
)

type optionsGroup struct {
	orgName   string
	groupName string
	cfgFlags  *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsGroup(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsGroup {
	return &optionsGroup{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdGroup(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsGroup(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "group",
		Short:   "项目组",
		Long:    "项目组",
		Aliases: []string{"groups"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringVar(&o.orgName, "org", "", "传入组织名")
	cmd.Flags().StringVar(&o.groupName, "group", "", "传入项目组名称")
	return cmd
}

func (that *optionsGroup) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.groupName = args[0]
	}
	return nil
}

func (that *optionsGroup) Validate() error {
	//if len(that.groupName) <= 0 {
	//	return fmt.Errorf("你传入的参数不正确,正确的命令：%s ", color.GreenString("galaxy switch GroupName"))
	//}
	return nil
}

func (that *optionsGroup) Run() error {
	var org *galaxycfg.Organization
	if len(that.orgName) > 0 {
		org = that.cfgFlags.GalaxyConfig.GetOrgByName(that.orgName)
	} else {
		org = that.cfgFlags.GalaxyConfig.GetOrgById(that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id)
	}
	svr := group.NewService(that.cfgFlags)
	groups, err := svr.Simple(&protoc.GroupListReq{OrgId: org.Id})
	if err != nil {
		return err
	}
	if len(groups) < 1 {
		return gerror.Newf("未在组织 [%s] 找到可切换的项目组", org.Title)
	}
	var group *protoc.Group
	if len(that.groupName) == 0 {
		group, err = that.askGroup("选择你要切换的项目组", groups, org.DefaultGroupId)
		if err != nil {
			return err
		}
	} else {
		for _, p := range groups {
			if p.Title == that.groupName {
				group = p
			}
		}
		if org == nil {
			return gerror.Newf("你要切换的项目组 [%s] 不存在", that.groupName)
		}
	}
	org.DefaultGroupId = group.GetId()
	that.cfgFlags.GalaxyConfig.SetOrg(org)
	err = that.cfgFlags.GalaxyConfig.Save(that.cfgFlags.GetGalaxyConfigFile())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "切换项目组成功，当前组织：[%s]，当前项目组：[%s]\n", org.Title, group.GetTitle())
	return nil
}
func (that *optionsGroup) askGroup(msg string, groups []*protoc.Group, defaultGroupId uint32) (*protoc.Group, error) {

	if len(groups) == 1 {
		return groups[0], nil
	}
	var defaultTitle = ""
	titles := make([]string, len(groups))
	for i, m := range groups {
		titles[i] = m.Title
		if defaultGroupId > 0 && defaultGroupId == m.Id {
			defaultTitle = m.Title
		}
	}
	if len(defaultTitle) == 0 {
		defaultTitle = titles[0]
	}
	answerIndex := 0
	err := survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
		Default: defaultTitle,
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return groups[answerIndex], nil
}
