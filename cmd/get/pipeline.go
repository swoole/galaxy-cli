package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	utilpointer "galaxy/pkg/utils/pointer"
	"galaxy/protoc"
	"galaxy/service/pipelines"
	"github.com/dustin/go-humanize"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/gtime"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type printPipeLineTable struct {
	Id       string `table:"ID"`
	PipeName string `table:"流水线"`
	Remark   string `table:"备注"`
	Type     string `table:"类型"`
	Creator  string `table:"创建人"`
	CreateAt string `table:"创建时间"`
}

type optionsPipeLine struct {
	keyword  *string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsPipeLine(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsPipeLine {
	return &optionsPipeLine{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
		keyword:   utilpointer.String(""),
	}
}

func newCmdPipeLine(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsPipeLine(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "pipeline",
		Short:   "流水线",
		Long:    "流水线",
		Aliases: []string{"pl"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringVar(o.keyword, "keyword", "", "搜索关键词.")
	return cmd
}

func (that *optionsPipeLine) Complete(cmd *cobra.Command, args []string) error {

	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	return nil
}

func (that *optionsPipeLine) Validate() error {
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}
func (that *optionsPipeLine) Run() (err error) {

	req := &protoc.PipelineListReq{
		OrgId:                    that.project.OrgId,
		GroupId:                  that.project.GroupId,
		ProjectId:                that.project.ProjectId,
		WithFramework:            true,
		WithFrameworkVersion:     true,
		WithFrameworkLangVersion: true,
		Keyword:                  *that.keyword,
	}
	pipelineRsp, err := pipelines.NewService(that.cfgFlags).List(req)
	if err != nil {
		return err
	}
	if len(pipelineRsp.GetData()) < 1 {
		return gerror.New("未找到流水线")
	}
	var printTables []*printPipeLineTable
	for _, b := range pipelineRsp.GetData() {
		nickname := "系统管理员"
		if len(b.GetCreatorInfo().GetNickname()) > 0 {
			nickname = b.GetCreatorInfo().GetNickname()
		}
		t := &printPipeLineTable{
			Id:       fmt.Sprintf("#%d", b.GetId()),
			PipeName: b.GetTitle(),
			Remark:   gstr.Trim(b.GetRemark()),
			Type:     pipelineTypeString(b.GetType()),
			Creator:  nickname,
			CreateAt: humanize.Time(gtime.NewFromTimeStamp(b.GetCreatedAt()).Time),
		}
		printTables = append(printTables, t)
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}

func pipelineTypeString(expr protoc.PipelineType) string {
	switch expr {
	case protoc.PipelineType_PipelineTypeUDF:
		return "用户自定义"
	case protoc.PipelineType_PipelineTypeDefault:
		return "默认"
	case protoc.PipelineType_PipelineTypeDefaultTPL:
		return "默认流水线模板"
	default:
		return "未知"
	}
}
