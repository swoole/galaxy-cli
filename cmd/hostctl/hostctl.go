package hostctl

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/hostctl"
	utilpointer "galaxy/pkg/utils/pointer"
	"github.com/gogf/gf/net/gipv4"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type Options struct {
	args     []string
	profile  *string
	ip       *string
	hostname *string
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
		profile:   utilpointer.String(""),
		hostname:  utilpointer.String(""),
		ip:        utilpointer.String(""),
	}
}

func NewCmdHostCtl(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "hostctl",
		Short:   "控制本地host",
		Example: `galaxy hostctl`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
		ValidArgs: []string{"addRoute"},
	}
	cmd.Flags().StringVar(o.profile, "profile", "galaxy", "hosts的配置分组")
	cmd.Flags().StringVar(o.ip, "ip", "", "ip地址")
	cmd.Flags().StringVar(o.hostname, "hostname", "", "host名称")
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	that.args = args
	return nil
}

func (that *Options) Validate() error {
	if len(that.args) == 0 {
		return fmt.Errorf("请输入操作命令")
	}
	if gstr.ToLower(that.args[0]) == "addroute" {
		if len(*that.ip) == 0 || gipv4.Ip2long(*that.ip) == 0 {
			return fmt.Errorf("传入的ip有误")
		}
		if len(*that.hostname) == 0 {
			return fmt.Errorf("传入的hostname有误")
		}
	}
	return nil
}

func (that *Options) Run() error {
	switch gstr.ToLower(that.args[0]) {
	case "addroute":
		err := hostctl.CreateRoute(*that.profile, *that.hostname, *that.ip)
		if err != nil {
			return fmt.Errorf("添加host失败,error:%v", err)
		}
	}
	return nil
}
