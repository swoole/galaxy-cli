package get

import (
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/utils/templates"
	"github.com/spf13/cobra"
)

type Options struct {
	PrintFlags *PrintFlags
	CmdParent  string
	NoHeaders  bool
	cfgFlags   *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

var (
	listLong    = templates.LongDesc(`列出一个或多个资源的重要信息.`)
	listExample = templates.Examples(`
	# 组织简单信息列表
	galaxy list organization|org
	# 项目组简单信息列表
	galaxy list groups|group
	# 项目简单信息列表
	galaxy list projects|project
	# 构建记录
	galaxy list build
	# 流水线列表
	galaxy list pipeline|pl
	# 镜像列表
	galaxy list images|image|img
	# 集群列表
	galaxy list cluster|clu
	# 发布记录
	galaxy list deploy|dp
	# 获取运行实例
	galaxy list instance|inst
	# 获取项目实例下的容器
	galaxy list container [instance|service]
	# 域名接入
	galaxy list domain|route
`)
)

func newOptions(parent string, cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *Options {
	return &Options{
		PrintFlags: NewGetPrintFlags(),
		CmdParent:  parent,
		cfgFlags:   cfgFlags,
		IOStreams:  streams,
	}
}

func NewCmdList(parent string, cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(parent, cfgFlags, streams)

	cmd := &cobra.Command{
		Use:     "list [RESOURCE] [flags]",
		Short:   "列出一个或多个资源",
		Long:    listLong,
		Example: listExample,
		Aliases: []string{"ls"},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	o.PrintFlags.AddFlags(cmd)
	cmd.AddCommand(newCmdDomain(cfgFlags, streams))
	cmd.AddCommand(newCmdProject(cfgFlags, streams))
	cmd.AddCommand(newCmdCertificate(cfgFlags, streams))
	cmd.AddCommand(newCmdRoute(cfgFlags, streams))
	cmd.AddCommand(newCmdBuild(cfgFlags, streams))
	cmd.AddCommand(newCmdCluster(cfgFlags, streams))
	cmd.AddCommand(newCmdDeploy(cfgFlags, streams))
	cmd.AddCommand(newCmdImage(cfgFlags, streams))
	cmd.AddCommand(newCmdInstance(cfgFlags, streams))
	cmd.AddCommand(newCmdContainer(cfgFlags, streams))
	cmd.AddCommand(newCmdOrg(cfgFlags, streams))
	cmd.AddCommand(newCmdPipeLine(cfgFlags, streams))
	cmd.AddCommand(newCmdGroup(cfgFlags, streams))
	return cmd
}
