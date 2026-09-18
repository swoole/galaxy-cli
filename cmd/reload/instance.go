package reload

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/instance"
	"github.com/spf13/cobra"
)

type optionsInstance struct {
	forcePull bool
	project   *galaxycfg.Project
	cfgFlags  *galaxycfg.ConfigFlags
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
	cmd.Flags().BoolVar(&o.forcePull, "force-pull", false, "强制拉取镜像,建议同一镜像在被重新构建的时候使用")
	return cmd
}

func (that *optionsInstance) Complete(cmd *cobra.Command, args []string) error {
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

	instanceSvr := instance.NewService(that.cfgFlags)
	runtime, err := instanceSvr.SelectedRuntime("请选择你要重启的实例: ", that.project.OrgId, that.project.GroupId, that.project.ProjectId, "")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "选中实例: %s\n", runtime.Name)
	if that.forcePull {
		_, _ = fmt.Fprintln(that.Out, "提示: Docker Swarm 重启会按 Service 当前镜像重新创建任务；镜像拉取策略由 Swarm/Registry 决定")
	}
	err = instanceSvr.RestartRuntime(that.project.OrgId, that.project.GroupId, that.project.ProjectId, runtime.ID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "重启实例 %s 成功\n", runtime.Name)
	return nil
}
