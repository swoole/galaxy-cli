package create

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"github.com/fatih/color"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type options struct {
	args     []string
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *options {
	return &options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func NewCmdCreate(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "create [NAME] [ITEM] [flags]",
		Short:   "创建资源",
		Long:    "创建资源",
		Example: `galaxy create`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate(cmd, args))
			cmdutil.CheckErr(o.Run(cmd, args))
		},
	}
	cmd.AddCommand(newCmdDomain(cfgFlags, ioStreams))
	cmd.AddCommand(newCmdCertificate(cfgFlags, ioStreams))
	cmd.AddCommand(newCmdRoute(cfgFlags, ioStreams))
	return cmd
}

func (that *options) Complete(cmd *cobra.Command, args []string) error {

	that.args = args

	return nil
}

func (that *options) Validate(cmd *cobra.Command, args []string) error {

	if len(that.args) <= 0 {
		return fmt.Errorf("你传入的参数不正确,正确的命令：%s %s", color.GreenString("galaxy create"), color.GreenString(gstr.Implode("|", cmd.ValidArgs)))
	}
	return nil
}
func (that *options) Run(cmd *cobra.Command, args []string) (err error) {
	return nil
}
