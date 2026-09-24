package reload

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"github.com/spf13/cobra"
)

type options struct {
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *options {
	return &options{
		cfgFlags:  cfgFlags,
		IOStreams: streams,
	}
}

func NewCmdReload(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, streams)
	cmd := &cobra.Command{
		Use:   "reload [NAME] [flags]",
		Short: "重启资源",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.AddCommand(newCmdInstance(cfgFlags, streams))
	return cmd
}

func (that *options) Complete(cmd *cobra.Command, args []string) error {

	return nil
}

func (that *options) Validate() error {

	return nil
}
func (that *options) Run() (err error) {

	return fmt.Errorf("请执行 galaxy reload --help 查看支持的命令")
}
