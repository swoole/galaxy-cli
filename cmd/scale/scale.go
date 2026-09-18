package scale

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/instance"
	"github.com/spf13/cobra"
)

type options struct {
	args     []string
	Replicas int
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *options {
	return &options{
		cfgFlags:  cfgFlags,
		IOStreams: streams,
	}
}

func NewCmdScale(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, streams)

	cmd := &cobra.Command{
		Use:   "scale --replicas=COUNT  TYPE NAME",
		Short: "设置实例的副本数量",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate(cmd, args))
			cmdutil.CheckErr(o.Run(cmd, args))
		},
	}

	cmd.Flags().IntVar(&o.Replicas, "replicas", 0, "期望的副本数量,必传")
	_ = cmd.MarkFlagRequired("replicas")
	return cmd
}

func (that *options) Complete(cmd *cobra.Command, args []string) error {

	that.args = args

	return nil
}

func (that *options) Validate(cmd *cobra.Command, args []string) error {
	if that.Replicas < 0 {
		return fmt.Errorf("必须传入标志 --replicas=COUNT , 且它的值必须大于或等于0")
	}
	return nil
}
func (that *options) Run(cmd *cobra.Command, args []string) (err error) {
	project := that.cfgFlags.ProjectConfig.DefaultProject()
	if project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	preferred := ""
	if len(args) > 0 {
		preferred = args[len(args)-1]
	}
	instanceSvr := instance.NewService(that.cfgFlags)
	runtime, err := instanceSvr.SelectedRuntime("请选择你要扩容/缩容的实例: ", project.OrgId, project.GroupId, project.ProjectId, preferred)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "选中实例: %s\n", runtime.Name)
	err = instanceSvr.ScaleRuntime(project.OrgId, project.GroupId, project.ProjectId, runtime.ID, that.Replicas)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "扩容/缩容 %s 成功,目标副本数目:%d \n", runtime.Name, that.Replicas)
	return nil
}
