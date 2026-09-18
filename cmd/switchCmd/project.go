package switchCmd

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/gogf/gf/errors/gerror"
	"github.com/spf13/cobra"
)

type optionsProject struct {
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
	cmd.Flags().StringVar(&o.projectName, "project", "", "传入项目名")
	return cmd
}

func (that *optionsProject) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.projectName = args[0]
	}
	return nil
}

func (that *optionsProject) Validate() error {
	//if len(that.projectName) <= 0 {
	//	return fmt.Errorf("你传入的参数不正确,正确的命令：%s ", color.GreenString("galaxy switch projectName"))
	//}
	return nil
}

func (that *optionsProject) Run() (err error) {
	var project *galaxycfg.Project

	if len(that.projectName) > 0 {
		project = that.cfgFlags.ProjectConfig.GetProjectByName(that.projectName)
		if project == nil {
			return gerror.Newf("未找到你要切换的项目[%s]", that.projectName)
		}
	} else {
		if len(that.cfgFlags.ProjectConfig.Projects) == 1 {
			_, _ = fmt.Fprintf(that.Out, "当前目录下只有一个项目 [%s],不需要切换. \n", that.cfgFlags.ProjectConfig.Projects[0].Title)
			return nil
		}
		project, err = that.askProject("请选择你要切换的项目", that.cfgFlags.ProjectConfig.Projects, that.cfgFlags.ProjectConfig.DefaultProjectId)
		if err != nil {
			return err
		}
	}

	that.cfgFlags.ProjectConfig.DefaultProjectId = project.ProjectId
	err = that.cfgFlags.ProjectConfig.Save(that.cfgFlags.GalaxyProjectRoot())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "切换项目成功,当前项目: [%s]\n", project.Title)
	return nil
}
func (that *optionsProject) askProject(msg string, projects []galaxycfg.Project, defaultProjectId uint32) (*galaxycfg.Project, error) {

	if len(projects) == 0 {
		return nil, fmt.Errorf("请输入你要切换的Project name `%s`", color.GreenString("galaxy switch project projectName"))
	}
	if len(projects) == 1 {
		return &projects[0], nil
	}
	var defaultTitle = ""
	titles := make([]string, len(projects))
	for i, m := range projects {
		titles[i] = m.Title
		if defaultProjectId > 0 && defaultProjectId == m.ProjectId {
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
	return &projects[answerIndex], nil
}
