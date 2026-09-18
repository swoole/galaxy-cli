package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/protoc"
	"galaxy/service/image"
	"github.com/dustin/go-humanize"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/gtime"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type printImageTable struct {
	Id        string `table:"ID"`
	ShortName string `table:"镜像名"`
	Version   string `table:"版本"`
	Remark    string `table:"备注"`
	Size      string `table:"镜像大小"`
	Instance  string `table:"已部署/总实例"`
	Creator   string `table:"构建人"`
	CreateAt  string `table:"构建时间"`
}

type optionsImage struct {
	domain   string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsImage(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsImage {
	return &optionsImage{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdImage(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsImage(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "image",
		Short:   "镜像",
		Long:    "镜像",
		Aliases: []string{"img", "images"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsImage) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.domain = args[0]
	}
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	return nil
}

func (that *optionsImage) Validate() error {
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}
func (that *optionsImage) Run() (err error) {

	req := protoc.ImageListReq{
		OrgId:     that.project.OrgId,
		GroupId:   that.project.GroupId,
		ProjectId: that.project.ProjectId,
	}
	imageListRsp, err := image.NewService(that.cfgFlags).Artifacts(&req)
	if err != nil {
		return err
	}
	if len(imageListRsp.Data) < 1 {
		return gerror.New("未找到镜像数据")
	}
	var printTables []*printImageTable
	for _, b := range imageListRsp.Data {
		buildInfo := b.Build
		if buildInfo == nil {
			buildInfo = &image.ArtifactBuild{}
		}
		creator := ""
		if buildInfo.CreatorInfo != nil {
			creator = buildInfo.CreatorInfo.Nickname
		}
		t := &printImageTable{
			Id: fmt.Sprintf("#%d", b.ID), ShortName: b.Reference,
			Version: fmt.Sprintf("%s:%s", buildInfo.Branch, gstr.SubStr(buildInfo.CommitID, 0, 8)),
			Remark:  gstr.SubStrRune(gstr.Trim(buildInfo.Remark), 0, 40), Creator: creator,
			CreateAt: humanize.Time(gtime.NewFromTimeStamp(b.CreatedAt).Time), Size: humanize.Bytes(b.Size), Instance: "-",
		}
		printTables = append(printTables, t)
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}
