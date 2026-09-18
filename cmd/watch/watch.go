package watch

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"github.com/fatih/color"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type Options struct {
	args     []string
	buildId  int32
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func NewCmdWatch(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	var validArgs []string
	for k, _ := range o.distribute() {
		validArgs = append(validArgs, k)
	}
	cmd := &cobra.Command{
		Use:     "watch",
		Short:   "监视日志信息",
		Long:    "监视日志信息",
		Example: `galaxy watch`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate(cmd, args))
			cmdutil.CheckErr(o.Run())
		},
		ValidArgs: validArgs,
	}

	cmd.Flags().Int32Var(&o.buildId, "build-id", 0, "构建Id")
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	that.args = args
	return nil
}

func (that *Options) Validate(cmd *cobra.Command, args []string) error {
	if len(that.args) == 0 {
		return fmt.Errorf("你传入的参数不正确,正确的命令：%s %s", color.GreenString("galaxy watch"), color.GreenString(gstr.Implode("|", cmd.ValidArgs)))
	}
	return nil
}

func (that *Options) Run() error {
	distribute := that.distribute()
	if f, ok := distribute[that.args[0]]; ok {
		err := f()
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("请执行 galaxy watch --help 查看支持的命令")
}

func (that *Options) distribute() map[string]func() error {
	return map[string]func() error{
		"build": that.BuildLogOutput,
	}
}
