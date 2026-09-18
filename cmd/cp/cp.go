package cp

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/utils/templates"
	"galaxy/service/container"
	"galaxy/service/instance"
	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var cpExample = templates.Examples(`
  # 将本地文件或目录复制到项目的 Swarm 容器
  galaxy cp /tmp/foo runtime-or-container:/tmp/bar

  # 从项目的 Swarm 容器复制文件或目录到本地
  galaxy cp runtime-or-container:/tmp/foo /tmp/bar`)

type Options struct {
	Container string
	args      []string
	cfgFlags  *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}
type target struct {
	container *container.Container
	clusterID uint32
}

func newOptions(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *Options {
	return &Options{IOStreams: streams, cfgFlags: flags}
}
func NewCmdBuild(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(flags, streams)
	cmd := &cobra.Command{Use: "cp <file-spec-src> <file-spec-dest>", Short: "在本地与项目 Swarm 容器之间复制文件或目录", Example: cpExample, Args: cobra.ExactArgs(2), Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(o.Complete(cmd, args))
		cmdutil.CheckErr(o.Run())
	}}
	cmd.Flags().StringVarP(&o.Container, "container", "c", "", "容器名称或 ID")
	return cmd
}
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.args = args
	return nil
}
func (o *Options) Run() error {
	p := o.cfgFlags.ProjectConfig.DefaultProject()
	if p == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	src, err := extractFileSpec(o.args[0])
	if err != nil {
		return err
	}
	dst, err := extractFileSpec(o.args[1])
	if err != nil {
		return err
	}
	if (src.PodName == "") == (dst.PodName == "") {
		return fmt.Errorf("源路径和目标路径必须恰好有一个是容器路径")
	}
	remote := src
	if remote.PodName == "" {
		remote = dst
	}
	preferred := o.Container
	if preferred == "" {
		preferred = remote.PodName
	}
	target, err := o.selectTarget(p, preferred)
	if err != nil {
		return err
	}
	service := container.NewService(o.cfgFlags)
	if src.PodName != "" {
		return o.download(service, p, target, src.File.String(), dst.File.String())
	}
	return o.upload(service, p, target, src.File.String(), dst.File.String())
}
func (o *Options) selectTarget(p *galaxycfg.Project, preferred string) (*target, error) {
	explicitContainer := o.Container != ""
	runtimes, err := instance.NewService(o.cfgFlags).Runtimes(p.OrgId, p.GroupId, p.ProjectId)
	if err != nil {
		return nil, err
	}
	clusters := map[uint32]bool{}
	targetService := ""
	for _, runtime := range runtimes {
		if runtime.Cluster != nil {
			clusters[runtime.Cluster.ID] = true
			if !explicitContainer && (preferred == runtime.Name || preferred == runtime.ServiceName || preferred == runtime.RuntimeRef || (preferred == p.Title && len(runtimes) == 1)) {
				targetService = runtime.ServiceName
			}
		}
	}
	service := container.NewService(o.cfgFlags)
	var matches []*target
	for clusterID := range clusters {
		items, err := service.Containers(p.OrgId, p.GroupId, p.ProjectId, clusterID)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.State != "running" {
				continue
			}
			current := &target{container: item, clusterID: clusterID}
			if preferred != "" && (item.Name == preferred || item.ID == preferred || (len(preferred) >= 4 && strings.HasPrefix(item.ID, preferred)) || (!explicitContainer && targetService != "" && strings.HasPrefix(item.Name, targetService+"."))) {
				return current, nil
			}
			matches = append(matches, current)
		}
	}
	if preferred != "" {
		return nil, fmt.Errorf("未找到运行中的项目容器 %q", preferred)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("当前项目没有运行中的 Swarm 容器")
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	titles := make([]string, len(matches))
	for i, item := range matches {
		titles[i] = item.container.Name
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: "请选择容器", Options: titles, Default: titles[0]}, &selected); err != nil {
		return nil, err
	}
	return matches[selected], nil
}
func (o *Options) upload(service *container.Service, p *galaxycfg.Project, target *target, local, remote string) error {
	info, err := os.Stat(local)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return o.uploadFile(service, p, target, local, remote)
	}
	root := filepath.Clean(local)
	return filepath.Walk(root, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		destination := remote
		if rel != "." {
			destination = path.Join(remote, filepath.ToSlash(rel))
		}
		if info.IsDir() {
			return o.mkdirRemote(service, p, target, destination)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return o.uploadFile(service, p, target, current, destination)
	})
}
func (o *Options) uploadFile(service *container.Service, p *galaxycfg.Project, target *target, local, remote string) error {
	content, err := os.ReadFile(local)
	if err != nil {
		return err
	}
	if err := o.mkdirRemote(service, p, target, path.Dir(remote)); err != nil {
		return err
	}
	if err := service.WriteFile(p.OrgId, p.GroupId, p.ProjectId, target.clusterID, target.container.ID, remote, content); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "%s -> %s:%s\n", local, target.container.Name, remote)
	return nil
}
func (o *Options) mkdirRemote(service *container.Service, p *galaxycfg.Project, target *target, directory string) error {
	result, err := service.Exec(p.OrgId, p.GroupId, p.ProjectId, target.clusterID, target.container.ID, []string{"mkdir", "-p", directory}, "")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("创建容器目录失败: %s", result.Stderr)
	}
	return nil
}
func (o *Options) download(service *container.Service, p *galaxycfg.Project, target *target, remote, local string) error {
	result, err := service.Exec(p.OrgId, p.GroupId, p.ProjectId, target.clusterID, target.container.ID, []string{"test", "-d", remote}, "")
	if err != nil {
		return err
	}
	if result.ExitCode == 0 {
		return o.downloadDir(service, p, target, remote, local)
	}
	content, err := service.ReadFile(p.OrgId, p.GroupId, p.ProjectId, target.clusterID, target.container.ID, remote)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(local); statErr == nil && info.IsDir() {
		local = filepath.Join(local, path.Base(remote))
	}
	if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(local, content, 0644); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "%s:%s -> %s\n", target.container.Name, remote, local)
	return nil
}
func (o *Options) downloadDir(service *container.Service, p *galaxycfg.Project, target *target, remote, local string) error {
	if err := os.MkdirAll(local, 0755); err != nil {
		return err
	}
	listing, err := service.ListFiles(p.OrgId, p.GroupId, p.ProjectId, target.clusterID, target.container.ID, remote)
	if err != nil {
		return err
	}
	for _, item := range listing.Files {
		if item.Name == "." || item.Name == ".." || strings.Contains(item.Name, "/") {
			continue
		}
		remoteChild := path.Join(remote, item.Name)
		localChild := filepath.Join(local, item.Name)
		switch item.Type {
		case "dir":
			if err := o.downloadDir(service, p, target, remoteChild, localChild); err != nil {
				return err
			}
		case "file":
			if err := o.download(service, p, target, remoteChild, localChild); err != nil {
				return err
			}
		}
	}
	return nil
}
