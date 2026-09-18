package exec

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/container"
	"galaxy/service/instance"
	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"strings"
)

type StreamOptions struct {
	ContainerName string
	Stdin, TTY    bool
	galaxycfg.IOStreams
}
type Options struct {
	StreamOptions
	ResourceName string
	Command      []string
	cfgFlags     *galaxycfg.ConfigFlags
}
type target struct {
	container    *container.Container
	clusterID    uint32
	clusterTitle string
}

func newOptions(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *Options {
	return &Options{StreamOptions: StreamOptions{IOStreams: streams}, cfgFlags: flags}
}
func NewCmdExec(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(flags, streams)
	cmd := &cobra.Command{Use: "exec [RUNTIME_OR_CONTAINER] [flags] -- COMMAND [args...]", Short: "在项目的 Swarm 容器中执行命令", Example: "galaxy exec -- pwd\ngalaxy exec web -- php -v\ngalaxy exec -it web", Aliases: []string{"ssh"}, Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(o.Complete(cmd, args, cmd.ArgsLenAtDash()))
		cmdutil.CheckErr(o.Validate())
		cmdutil.CheckErr(o.Run())
	}}
	cmd.Flags().StringVarP(&o.ContainerName, "container", "c", "", "优先选择的 Runtime、容器名称或容器 ID")
	cmd.Flags().BoolVarP(&o.Stdin, "stdin", "i", false, "连接标准输入")
	cmd.Flags().BoolVarP(&o.TTY, "tty", "t", false, "分配交互式终端")
	return cmd
}
func (o *Options) Complete(cmd *cobra.Command, args []string, dash int) error {
	if dash > 0 {
		o.ResourceName = args[0]
	} else if dash < 0 && len(args) > 0 {
		o.ResourceName = args[0]
	}
	if dash >= 0 {
		o.Command = args[dash:]
	}
	if len(o.Command) == 0 {
		o.Command = []string{"sh"}
		o.Stdin = true
		o.TTY = true
	}
	return nil
}
func (o *Options) Validate() error {
	if o.Out == nil || o.ErrOut == nil {
		return fmt.Errorf("必须同时提供标准输出和错误输出")
	}
	return nil
}
func (o *Options) Run() error {
	p := o.cfgFlags.ProjectConfig.DefaultProject()
	if p == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	target, err := o.selectTarget(p)
	if err != nil {
		return err
	}
	service := container.NewService(o.cfgFlags)
	if o.TTY || o.Stdin {
		return service.Interactive(p.OrgId, p.GroupId, p.ProjectId, target.clusterID, target.container.ID, o.Command, o.IOStreams)
	}
	result, err := service.Exec(p.OrgId, p.GroupId, p.ProjectId, target.clusterID, target.container.ID, o.Command, "")
	if err != nil {
		return err
	}
	if result.Stdout != "" {
		_, _ = fmt.Fprint(o.Out, result.Stdout)
	}
	if result.Stderr != "" {
		_, _ = fmt.Fprint(o.ErrOut, result.Stderr)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("容器命令退出码: %d", result.ExitCode)
	}
	return nil
}
func (o *Options) selectTarget(p *galaxycfg.Project) (*target, error) {
	preferred := o.ContainerName
	explicitContainer := preferred != ""
	if preferred == "" {
		preferred = o.ResourceName
	}
	runtimes, err := instance.NewService(o.cfgFlags).Runtimes(p.OrgId, p.GroupId, p.ProjectId)
	if err != nil {
		return nil, err
	}
	clusters := map[uint32]string{}
	targetService := ""
	for _, runtime := range runtimes {
		if runtime.Cluster != nil && runtime.Cluster.ID > 0 {
			clusters[runtime.Cluster.ID] = runtime.Cluster.Title
			if !explicitContainer && preferred != "" && (preferred == runtime.Name || preferred == runtime.ServiceName || preferred == runtime.RuntimeRef || (preferred == p.Title && len(runtimes) == 1)) {
				targetService = runtime.ServiceName
			}
		}
	}
	service := container.NewService(o.cfgFlags)
	var targets []*target
	for clusterID, title := range clusters {
		items, err := service.Containers(p.OrgId, p.GroupId, p.ProjectId, clusterID)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.State != "running" {
				continue
			}
			if preferred != "" && (item.Name == preferred || item.ID == preferred || (len(preferred) >= 4 && len(item.ID) >= len(preferred) && item.ID[:len(preferred)] == preferred) || (!explicitContainer && targetService != "" && strings.HasPrefix(item.Name, targetService+"."))) {
				return &target{container: item, clusterID: clusterID, clusterTitle: title}, nil
			}
			targets = append(targets, &target{container: item, clusterID: clusterID, clusterTitle: title})
		}
	}
	if preferred != "" {
		return nil, fmt.Errorf("未找到运行中的项目容器 %q", preferred)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("当前项目没有运行中的 Swarm 容器")
	}
	if len(targets) == 1 {
		return targets[0], nil
	}
	titles := make([]string, len(targets))
	for i, item := range targets {
		titles[i] = fmt.Sprintf("%s [%s]", item.container.Name, item.clusterTitle)
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: "请选择容器", Options: titles, Default: titles[0]}, &selected); err != nil {
		return nil, err
	}
	return targets[selected], nil
}
