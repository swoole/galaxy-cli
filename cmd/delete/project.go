package delete

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/protoc"
	"galaxy/service/account"
	"galaxy/service/project"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type optionsProject struct {
	args        []string
	projectName string
	cfgFlags    *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsProject(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsProject {
	return &optionsProject{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdProject(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsProject(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:   "project",
		Short: "项目",
		Long:  "项目",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsProject) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.projectName = args[0]
	}
	return nil
}

func (that *optionsProject) Validate() error {

	if len(that.projectName) <= 0 {
		return fmt.Errorf("请传入你要删除的项目名")
	}
	return nil
}
func (that *optionsProject) Run() (err error) {
	projectSvr := project.NewService(that.cfgFlags)

	var projectList []*protoc.ProjectList
	if len(that.projectName) > 0 {
		projectRsp, err := projectSvr.GetList(&protoc.ProjectListReq{
			OrgId:   that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id,
			Keyword: that.projectName,
		})
		if err != nil {
			return err
		}
		if projectRsp == nil || projectRsp.Total == 0 {
			return fmt.Errorf("未找到名字为'%s'的项目", that.projectName)
		}
		projectList = projectRsp.GetData()
	} else {
		projects := that.cfgFlags.ProjectConfig.Projects
		if len(projects) == 0 {
			return fmt.Errorf("当前目录不是项目目录，无法定位到要删除的项目")
		}

		for _, a := range projects {
			projectList = append(projectList, &protoc.ProjectList{
				Id:    a.ProjectId,
				Title: a.Title,
				OrgId: a.OrgId,
				Group: &protoc.Group{
					Id: a.GroupId,
				},
			})
		}
	}
	projectInfo, err := projectSvr.SelectedProject("选择你要删除的项目：", projectList)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "你将要删除的项目：%s", projectInfo.GetTitle())
	confirm, err := projectSvr.ConfirmRemoveProject(projectInfo)
	if err != nil {
		return err
	}
	if !confirm {
		return nil
	}
	confirmToken, err := account.NewService(that.cfgFlags).IdConfirm()
	if err != nil {
		return err
	}
	err = projectSvr.Delete(&protoc.ProjectDeleteReq{
		OrgId:        projectInfo.GetOrgId(),
		GroupId:      projectInfo.GetGroup().GetId(),
		ProjectId:    projectInfo.GetId(),
		ConfirmToken: confirmToken,
	})
	if err != nil {
		return err
	}
	err = that.cfgFlags.ProjectConfig.RemoveProject(that.cfgFlags.GalaxyProjectRoot(), projectInfo.GetId())
	if err != nil {
		_, _ = fmt.Fprintf(that.Out, "删除项目 '%s' 成功，但移除项目配置文件失败：%v\n", color.BlueString(projectInfo.GetTitle()), err)
	} else {
		_, _ = fmt.Fprintf(that.Out, "删除项目 '%s' 成功\n", color.BlueString(projectInfo.GetTitle()))
	}
	return nil
}
