package docker

import (
	"galaxy/pkg/galaxycfg"

	"github.com/spf13/cobra"
)

// NewCmdDocker creates commands that manage the Docker installation on the local host.
func NewCmdDocker(ioStreams galaxycfg.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docker",
		Short: "管理本机 Docker Engine",
		Long:  "管理本机 Docker Engine。涉及系统配置的命令需要使用 root 权限运行。",
	}
	cmd.AddCommand(newCmdSecureAPI(ioStreams))
	cmd.AddCommand(newCmdComposeMigration(ioStreams))
	cmd.AddCommand(newCmdContainerMigration(ioStreams))
	return cmd
}
