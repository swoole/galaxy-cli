package info

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"galaxy/protoc"
	"galaxy/service/group"
	"galaxy/service/organization"
	"galaxy/service/project"
	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/gogf/gf/container/garray"
	"github.com/gogf/gf/os/gtime"
	"github.com/gogf/gf/text/gstr"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

type Options struct {
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func NewCmdInfo(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "info",
		Short:   "查看配置信息",
		Long:    "查看配置信息",
		Example: `galaxy info`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {

	return nil
}

func (that *Options) Validate() error {

	return nil
}

func (that *Options) Run() error {
	if len(that.cfgFlags.GalaxyConfig.GetSecret()) == 0 {
		_, _ = fmt.Fprintf(that.Out, "你还未登录%s,请使用 `%s` 命令登录\n", color.GreenString("CodeGalaxy"), color.BlueString("galaxy login --username UserName"))
		return nil
	}
	// 1. 首先获取当前目录是否是project目录。
	// 2. 判断该项目是否能被登录的账号所管理。
	// 3. 如果不能被管理，则提示没有权限。
	// 4. 如果当前目录不在项目中，则展示全局的登录信息。
	// 5. 如果当前用户有管理当前项目的权限，则展示全部信息。
	// 6. 全局信息包括：API 版本，CLI 版本，当前登录用户信息(昵称，用户名)，默认项目组(项目组 ID，项目组名称，项目总数，成员总数，创建时间，负责人)，默认组织(组织 ID，组织名，组织类型，组织状态，组织描述，组织角色，加入时间)。
	// 7. 项目信息包括: 项目 ID，项目名，项目仓库地址，所属项目组，所属组织，创建人，创建时间，镜像数量，成员数量，上次活跃时间，版本总数，镜像名称，语言框架版本，默认端口，Pod 数量。
	orgInfo := that.cfgFlags.GalaxyConfig.GetDefaultOrg()
	if orgInfo == nil {
		_, _ = fmt.Fprintln(that.Out, "你还没有加入一个组织，请登录web网页端加入组织吧！https://console.code-galaxy.net/#/org/profile")
		return nil
	}
	orgProfile, err := organization.NewService(that.cfgFlags).Profile(orgInfo.Id)
	if err != nil {
		return err
	}
	var groupProfile *protoc.GroupProfile
	groupInfo := orgInfo.GetDefaultGroup()
	if groupInfo != nil {
		groupProfile, err = group.NewService(that.cfgFlags).Profile(&protoc.GroupProfileReq{OrgId: orgProfile.GetId(), GroupId: groupInfo.Id})
		if err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintln(that.Out, color.GreenString(text.AlignLeft.Apply("全局信息:", 12)))
	_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("API版本", 12), that.cfgFlags.GalaxyConfig.GetApiVersion())
	_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("CLI版本", 12), buildVariable.BuildVersion)
	_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("当前用户", 12), fmt.Sprintf("%s(%s)", gstr.Trim(that.cfgFlags.GalaxyConfig.GetUser().Nickname), that.cfgFlags.GalaxyConfig.GetUser().Email))
	_, _ = fmt.Fprintln(that.Out, color.GreenString(text.AlignLeft.Apply("默认组织信息:", 12)))
	_, _ = fmt.Fprintf(that.Out, "\t%s: %d\n", text.AlignLeft.Apply("组织ID", 12), orgProfile.GetId())
	_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("组织名称", 12), orgProfile.GetTitle())
	_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("组织类型", 12), organization.OrgTYPEString(orgProfile.GetType()))
	_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("认证类型", 12), organization.OrgAUTHString(orgProfile.GetIdAuth()))
	_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("创建时间", 12), humanize.Time(gtime.NewFromTimeStamp(orgProfile.GetCreatedAt()).Time))
	if groupProfile != nil {
		_, _ = fmt.Fprintln(that.Out, color.GreenString(text.AlignLeft.Apply("默认项目组信息:", 12)))
		_, _ = fmt.Fprintf(that.Out, "\t%s: %d\n", text.AlignLeft.Apply("项目组ID", 12), groupProfile.GetId())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("项目组名称", 12), groupProfile.GetTitle())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("创建人", 12), groupProfile.GetCreatorInfo().GetNickname())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("创建时间", 12), humanize.Time(gtime.NewFromTimeStamp(orgProfile.GetCreatedAt()).Time))
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("项目组描述", 12), gstr.SubStrRune(gstr.Trim(groupProfile.GetDesc()), 0, 40))
	}
	// 当前不在项目目录
	if !that.cfgFlags.InProject() {
		return nil
	}
	var findOrgs = garray.NewIntArray()
	for _, org := range that.cfgFlags.GalaxyConfig.GetOrgList() {
		findOrgs.Append(int(org.Id))
	}
	for _, projectInfo := range that.cfgFlags.ProjectConfig.Projects {
		if !findOrgs.Contains(int(projectInfo.OrgId)) {
			continue
		}
		projectProfile, err := project.NewService(that.cfgFlags).Profile(&protoc.ProjectProfileReq{
			OrgId:     projectInfo.OrgId,
			GroupId:   projectInfo.GroupId,
			ProjectId: projectInfo.ProjectId,
		})
		if err != nil {
			return err
		}
		if that.cfgFlags.ProjectConfig.DefaultProjectId == projectProfile.Id {
			_, _ = fmt.Fprintf(that.Out, "%s%s\n", color.GreenString("默认项目信息:"), color.BlueString(fmt.Sprintf("(#%d %s)", projectProfile.GetId(), projectProfile.GetTitle())))
		} else {
			_, _ = fmt.Fprintf(that.Out, "%s%s\n", color.GreenString("其他项目信息:"), color.BlueString(fmt.Sprintf("(#%d %s)", projectProfile.GetId(), projectProfile.GetTitle())))
		}
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("项目名", 12), projectProfile.GetTitle())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("所属项目组", 12), projectProfile.GetGroup().GetTitle())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("创建者", 12), projectProfile.GetCreatorInfo().GetNickname())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("创建时间", 12), gtime.NewFromTimeStamp(projectProfile.GetCreatedAt()).String())
		if len(projectProfile.GetDesc()) > 0 {
			_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("项目描述", 12), projectProfile.GetDesc())
		}
		if len(projectProfile.GetImageName()) == 0 {
			_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("镜像名称", 12), "自动生成")
		} else {
			_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("镜像名称", 12), projectProfile.GetImageName())
		}
		projectType := "已有镜像"
		if projectProfile.GetDevelop() {
			projectType = "代码项目"
		}
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("项目类型", 12), projectType)
		if projectProfile.GetRepository() != nil {
			_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("Git仓库", 12), projectProfile.GetRepository().GetCloneUrl())
		}
		_, _ = fmt.Fprintf(that.Out, "\t%s: %d\n", text.AlignLeft.Apply("默认端口", 12), projectProfile.GetDefaultPort())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %d\n", text.AlignLeft.Apply("成员数量", 12), projectProfile.GetMemberCount())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %s\n", text.AlignLeft.Apply("上次活跃时间", 12), gtime.NewFromTimeStamp(projectProfile.GetLastActive()).String())
		_, _ = fmt.Fprintf(that.Out, "\t%s: %d\n", text.AlignLeft.Apply("实例", 12), projectProfile.GetInstances())
	}

	return nil
}
