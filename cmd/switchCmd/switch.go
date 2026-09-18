package switchCmd

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"github.com/spf13/cobra"
)

type SwitchOptions struct {
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *SwitchOptions {
	return &SwitchOptions{
		cfgFlags:  cfgFlags,
		IOStreams: streams,
	}
}

func NewCmdSwitch(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, streams)
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "切换环境",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate(cmd, args))
			cmdutil.CheckErr(o.Run(cmd, args))
		},
	}
	cmd.AddCommand(newCmdProject(cfgFlags, streams))
	cmd.AddCommand(newCmdOrg(cfgFlags, streams))
	cmd.AddCommand(newCmdGroup(cfgFlags, streams))
	return cmd
}

func (that *SwitchOptions) Complete(cmd *cobra.Command, args []string) error {

	return nil
}

func (that *SwitchOptions) Validate(cmd *cobra.Command, args []string) error {

	return nil
}
func (that *SwitchOptions) Run(cmd *cobra.Command, args []string) (err error) {

	return fmt.Errorf("请执行 galaxy switch --help 查看支持的命令")
}
