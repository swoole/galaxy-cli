package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/protoc"
	"galaxy/service/project"
	"github.com/dustin/go-humanize"
	"github.com/gogf/gf/os/gtime"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type printProjectTable struct {
	Id          string `table:"ID"`
	Title       string `table:"名称"`
	GroupTitle  string `table:"所属项目组"`
	ProjectType string `table:"类型"`
	MemberCount int    `table:"成员总数"`
	LastActive  string `table:"最后活跃时间"`
	Instances   uint32 `table:"实例"`
}

type optionsProject struct {
	args     []string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
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
		Use:     "project",
		Short:   "项目",
		Long:    "项目",
		Aliases: []string{"projects"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsProject) Complete(cmd *cobra.Command, args []string) error {
	that.args = args
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	return nil
}

func (that *optionsProject) Validate() error {
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}
func (that *optionsProject) Run() (err error) {
	svr := project.NewService(that.cfgFlags)
	var projectsRsp *protoc.ProjectListRsp
	var projectsList []*protoc.ProjectList
	if len(that.args) == 0 {
		projectsRsp, err = svr.GetList(&protoc.ProjectListReq{
			OrgId: that.project.OrgId, GroupId: that.project.GroupId,
		})
		if err != nil {
			return err
		}
		if len(projectsRsp.GetData()) < 1 {
			return fmt.Errorf("你还没有项目,赶快创建吧!")
		}
		projectsList = projectsRsp.GetData()
	}
	if len(that.args) >= 1 {
		projectNames := that.args
		for _, projectName := range projectNames {
			projectsRsp, err = svr.GetList(&protoc.ProjectListReq{
				OrgId:   that.project.OrgId,
				GroupId: that.project.GroupId,
				Keyword: projectName,
			})
			if err != nil {
				return err
			}
			projectsList = append(projectsList, projectsRsp.GetData()...)
		}
		if len(projectsList) == 0 {
			return fmt.Errorf("未找到Project %s 的信息", gstr.Implode(",", projectNames))
		}
	}

	var printTables []*printProjectTable
	for _, a := range projectsList {
		title := a.GetTitle()
		projectType := "已有镜像"
		if a.GetDevelop() {
			projectType = "代码项目"
		}
		if that.cfgFlags.ProjectConfig.DefaultProject() != nil {
			if a.Id == that.cfgFlags.ProjectConfig.DefaultProject().ProjectId {
				title = fmt.Sprintf("%s(当前)", a.GetTitle())
			}
		}
		t := &printProjectTable{
			Id:          fmt.Sprintf("#%d", a.GetId()),
			Title:       title,
			GroupTitle:  a.GetGroup().Title,
			ProjectType: projectType,
			MemberCount: int(a.GetMemberCount()),
			LastActive:  humanize.Time(gtime.NewFromTimeStamp(a.GetLastActive()).Time),
			Instances:   a.GetInstances(),
		}
		printTables = append(printTables, t)
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}
