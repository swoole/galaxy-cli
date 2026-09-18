package cmd

import (
	"galaxy/cmd/autocompletion"
	"galaxy/cmd/build"
	"galaxy/cmd/cp"
	"galaxy/cmd/create"
	deleteCmd "galaxy/cmd/delete"
	"galaxy/cmd/deploy"
	dockercmd "galaxy/cmd/docker"
	"galaxy/cmd/edit"
	"galaxy/cmd/exec"
	"galaxy/cmd/get"
	hostctl2 "galaxy/cmd/hostctl"
	"galaxy/cmd/info"
	"galaxy/cmd/initproject"
	"galaxy/cmd/login"
	"galaxy/cmd/logout"
	"galaxy/cmd/projectdiff"
	"galaxy/cmd/reload"
	"galaxy/cmd/rollback"
	"galaxy/cmd/route"
	"galaxy/cmd/scale"
	"galaxy/cmd/switchCmd"
	"galaxy/cmd/upgrade"
	"galaxy/cmd/version"
	"galaxy/cmd/watch"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/utils/templates"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

const galaxyCmdHeaders = "GALAXY_COMMAND_HEADERS"

type GalaxyOptions struct {
	Arguments   []string
	ConfigFlags *galaxycfg.ConfigFlags

	galaxycfg.IOStreams
}

var defaultConfigFlags = galaxycfg.NewConfigFlags()

func NewDefaultGalaxyCommand() *cobra.Command {
	return NewDefaultGalaxyCommandWithArgs(GalaxyOptions{
		Arguments:   os.Args,
		ConfigFlags: defaultConfigFlags,
		IOStreams:   galaxycfg.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
	})
}

func NewDefaultGalaxyCommandWithArgs(o GalaxyOptions) *cobra.Command {
	cmd := NewGalaxyCommand(o)
	return cmd
}

func NewGalaxyCommand(o GalaxyOptions) *cobra.Command {

	cmds := &cobra.Command{
		Use:          "galaxy",
		Short:        "CodeGalaxy 一站式云原生研发管理平台",
		SilenceUsage: true,
		Long: templates.LongDesc(`
CodeGalaxy 一站式云原生研发管理平台.

想要了解更多CodeGalaxy平台的资料,请查看:https://docs.code-galaxy.net/`),
		Run: runHelp,
		// 执行命令之前运行，初始化profiling文件
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// `galaxy docker` is a standalone local-host toolbox. It must work
			// without Galaxy login/configuration and never initializes an API client.
			if !strings.HasPrefix(cmd.CommandPath(), "galaxy docker") {
				err := o.ConfigFlags.Load()
				if err != nil {
					return err
				}
			}
			return initProfiling()
		},
		// 执行命令后执行，写入分析文件到磁盘
		PersistentPostRunE: func(*cobra.Command, []string) error {
			if err := flushProfiling(); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmds.PersistentFlags()
	addProfilingFlags(flags)

	GalaxyConfigFlags := o.ConfigFlags
	if GalaxyConfigFlags == nil {
		GalaxyConfigFlags = defaultConfigFlags
	}

	GalaxyConfigFlags.AddFlags(flags)

	cmds.AddCommand(version.NewCmdVersion(o.IOStreams))
	cmds.AddCommand(get.NewCmdList("galaxy", o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(login.NewCmdLogin(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(logout.NewCmdLogout(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(initproject.NewCmdInit(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(upgrade.NewCmdUpgrade(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(build.NewCmdBuild(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(deploy.NewCmdDeploy(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(dockercmd.NewCmdAgentInstall(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(dockercmd.NewCmdDocker(o.IOStreams))
	cmds.AddCommand(info.NewCmdInfo(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(route.NewCmdRoute(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(switchCmd.NewCmdSwitch(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(rollback.NewCmdRollback(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(exec.NewCmdExec(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(deleteCmd.NewCmdDelete(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(watch.NewCmdWatch(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(autocompletion.NewCmdAutoCompletion(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(hostctl2.NewCmdHostCtl(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(cp.NewCmdBuild(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(projectdiff.NewCmdDiff(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(projectdiff.NewCmdSync(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(reload.NewCmdReload(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(scale.NewCmdScale(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(create.NewCmdCreate(o.ConfigFlags, o.IOStreams))
	cmds.AddCommand(edit.NewCmdEdit(o.ConfigFlags, o.IOStreams))
	return cmds
}

func runHelp(cmd *cobra.Command, args []string) {
	_ = cmd.Help()
}
