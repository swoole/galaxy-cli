package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/service/instance"
	"github.com/dustin/go-humanize"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
	"time"
)

type printInstanceTable struct {
	Id          string `table:"ID"`
	Version     string `table:"版本"`
	Remark      string `table:"发布标识"`
	PodName     string `table:"实例名"`
	EnvName     string `table:"环境名"`
	ClusterName string `table:"集群名"`
	Count       string `table:"副本(实际/预期)"`
	FlexMethod  string `table:"伸缩方式"`
	PodStatus   string `table:"实例状态"`
	PodHealth   string `table:"健康状态"`
	CreatedAt   string `table:"创建时间"`
}

type optionsInstance struct {
	domain   string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsInstance(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsInstance {
	return &optionsInstance{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdInstance(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsInstance(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "instance",
		Short:   "实例",
		Long:    "实例",
		Aliases: []string{"inst"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsInstance) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.domain = args[0]
	}
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	return nil
}

func (that *optionsInstance) Validate() error {
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}
func (that *optionsInstance) Run() (err error) {
	pods, err := instance.NewService(that.cfgFlags).Runtimes(that.project.OrgId, that.project.GroupId, that.project.ProjectId)
	if err != nil {
		return err
	}
	if len(pods) < 1 {
		return gerror.New("未找到项目实例")
	}
	var printTables []*printInstanceTable
	for _, pod := range pods {
		t := &printInstanceTable{
			Id: fmt.Sprintf("#%d", pod.ID), PodName: pod.Name, Version: "-", Count: fmt.Sprintf("%d/%d", pod.RunningCount, pod.DesiredCount),
			FlexMethod: "固定副本", PodStatus: pod.Status, PodHealth: pod.Health, CreatedAt: humanize.Time(time.Unix(pod.CreatedAt, 0)),
		}
		if pod.Env != nil {
			t.EnvName = pod.Env.Title
		}
		if pod.Cluster != nil {
			t.ClusterName = pod.Cluster.Title
		}
		if pod.Release != nil {
			t.Version = pod.Release.Version
			t.Remark = gstr.SubStrRune(gstr.Trim(pod.Release.Remark), 0, 40)
		}
		printTables = append(printTables, t)
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}
