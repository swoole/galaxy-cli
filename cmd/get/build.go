package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/protoc"
	"galaxy/service/build"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/gtime"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type printBuildTable struct {
	Id            string `table:"ID"`
	Version       string `table:"版本"`
	Status        string `table:"状态"`
	ElapsedTime   string `table:"构建耗时"`
	Remark        string `table:"发布说明"`
	PipeName      string `table:"流水线"`
	Creator       string `table:"操作人"`
	BuildCreateAt string `table:"构建时间"`
}

type optionsBuild struct {
	args     []string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsBuild(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsBuild {
	return &optionsBuild{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdBuild(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsBuild(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:   "build",
		Short: "构建记录",
		Long:  "构建记录",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsBuild) Complete(cmd *cobra.Command, args []string) error {
	that.args = args
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	return nil
}

func (that *optionsBuild) Validate() error {
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}
func (that *optionsBuild) Run() (err error) {
	req := protoc.BuildListReq{
		OrgId:     that.project.OrgId,
		GroupId:   that.project.GroupId,
		ProjectId: that.project.ProjectId,
	}
	list, err := build.NewService(that.cfgFlags).BuildList(&req)
	if err != nil {
		return err
	}
	if len(list.GetData()) < 1 {
		return gerror.New("未找到构建记录")
	}
	var printTables []*printBuildTable
	for _, b := range list.GetData() {
		pipelineName := "-"
		if b.GetPipeline() != nil && b.GetPipeline().GetTitle() != "" {
			pipelineName = b.GetPipeline().GetTitle()
		}
		creator := "-"
		if b.GetCreatorInfo() != nil && b.GetCreatorInfo().GetNickname() != "" {
			creator = b.GetCreatorInfo().GetNickname()
		}
		t := &printBuildTable{
			Id:            fmt.Sprintf("#%d", b.GetId()),
			Version:       fmt.Sprintf("%s:%s#%d", b.Branch, gstr.SubStr(b.CommitId, 0, 8), b.GetId()),
			Status:        build.StatusString(b.Status),
			PipeName:      pipelineName,
			Remark:        gstr.SubStrRune(gstr.Trim(b.Remark), 0, 40),
			ElapsedTime:   fmt.Sprintf("%ds", b.EndAt-b.StartAt),
			Creator:       creator,
			BuildCreateAt: gtime.NewFromTimeStamp(b.StartAt).Format("Y-m-d H:i:s"),
		}
		printTables = append(printTables, t)
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}
