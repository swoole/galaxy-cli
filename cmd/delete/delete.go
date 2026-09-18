package delete

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type Options struct {
	yes      bool
	args     []string
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *Options {
	return &Options{
		cfgFlags:  cfgFlags,
		IOStreams: streams,
	}
}

func NewCmdDelete(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, streams)
	cmd := &cobra.Command{
		Use:   "delete (TYPE [(NAME | -l label | --all)])",
		Short: "删除一个或多个资源",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().BoolVarP(&o.yes, "yes", "y", false, "不需要二次确认")
	cmd.AddCommand(newCmdDomain(cfgFlags, streams))
	cmd.AddCommand(newCmdCertificate(cfgFlags, streams))
	cmd.AddCommand(newCmdRoute(cfgFlags, streams))
	cmd.AddCommand(newCmdProject(cfgFlags, streams))
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	that.args = args
	return nil
}

func (that *Options) Validate() error {
	if len(that.args) <= 0 {
		return fmt.Errorf("您必须指定要获取的资源类型.使用 '%s' 命令获取支持的资源列表", color.GreenString("galaxy delete --help"))
	}
	return nil
}

func (that *Options) Run() (err error) {
	return nil
}
