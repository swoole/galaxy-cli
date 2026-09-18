package deploy

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/protoc"
	"galaxy/service/deploy"
	"galaxy/service/image"
	"galaxy/service/instance"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"strings"
)

//galaxy deploy 列表选择
//galaxy deploy --current 当前commitID或者tag
//galaxy deploy --current --prefix encrypt- 支持传入流水线的prefix
//galaxy deploy --latest 部署最新
//galaxy deploy --commit 7e0795d

type Options struct {
	imageId        uint32
	artifact       *image.Artifact
	latest         bool   // 部署最新的镜像
	commit         string // 使用commit部署
	tag            string // 使用tag部署
	prefix         string // 镜像tag前缀
	message        string // 部署说明
	newInstance    bool   // 是否创建新的实例
	yes            bool
	defaultProject *galaxycfg.Project
	cfgFlags       *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

// TODO Deploy的时候，默认应该只显示已经有过部署记录的实例，需要增加参数展示全部，没有部署记录的时候，第一次部署应该展示全部

func NewCmdDeploy(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "deploy",
		Short:   "发布项目",
		Long:    "发布项目",
		Example: `galaxy deploy`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().BoolVar(&o.latest, "latest", false, "发布最新镜像")
	cmd.Flags().StringVar(&o.commit, "commit", "", "使用commit id指定镜像")
	cmd.Flags().StringVar(&o.tag, "tag", "", "使用tag指定镜像")
	cmd.Flags().StringVar(&o.prefix, "prefix", "", "镜像tag前缀")
	cmd.Flags().StringVarP(&o.message, "message", "m", "cli deploy", "部署说明")
	cmd.Flags().BoolVar(&o.newInstance, "new-instance", false, "是否创建新的实例")
	cmd.Flags().BoolVarP(&o.yes, "yes", "y", false, "所有需要确认的地方都直接选Y")
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	that.defaultProject = that.cfgFlags.ProjectConfig.DefaultProject()
	if that.defaultProject == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	req := &protoc.ImageListReq{
		OrgId:     that.defaultProject.OrgId,
		GroupId:   that.defaultProject.GroupId,
		ProjectId: that.defaultProject.ProjectId,
	}
	if len(that.commit) > 0 {
		req.Commitid = that.commit
	}
	if len(that.tag) > 0 {
		req.Tag = that.tag
	}
	if len(that.prefix) > 0 {
		req.VersionPrefix = that.prefix
	}
	artifacts, err := image.NewService(that.cfgFlags).Artifacts(req)
	if err != nil {
		return err
	}
	filtered := make([]*image.Artifact, 0, len(artifacts.Data))
	for _, artifact := range artifacts.Data {
		if that.commit != "" && (artifact.Build == nil || !strings.HasPrefix(artifact.Build.CommitID, that.commit)) {
			continue
		}
		tag := imageTag(artifact.Reference)
		if that.tag != "" && tag != that.tag {
			continue
		}
		if that.prefix != "" && !strings.HasPrefix(tag, that.prefix) {
			continue
		}
		filtered = append(filtered, artifact)
	}
	if len(filtered) == 0 {
		return fmt.Errorf("没有可发布的镜像")
	}
	artifact, err := that.askArtifacts("请选择要发布的镜像:", filtered, that.latest)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "将要发布镜像: %s\n", color.GreenString(artifact.Reference))
	that.imageId = artifact.ID
	that.artifact = artifact
	return nil
}

func (that *Options) Validate() error {
	if that.imageId == 0 {
		return fmt.Errorf("没有可发布的镜像")
	}
	return nil
}

func (that *Options) Run() error {
	svr := deploy.NewService(that.cfgFlags)
	project := that.defaultProject
	runtimes, err := instance.NewService(that.cfgFlags).Runtimes(project.OrgId, project.GroupId, project.ProjectId)
	if err != nil {
		return err
	}
	if !that.newInstance && len(runtimes) > 0 {
		runtime, err := instance.NewService(that.cfgFlags).SelectedRuntime("请选择要更新镜像的实例:", project.OrgId, project.GroupId, project.ProjectId, "")
		if err != nil {
			return err
		}
		if !that.yes {
			confirmed := false
			if err := survey.AskOne(&survey.Confirm{Message: fmt.Sprintf("确定将 %s 更新到 %s?", runtime.Name, that.artifact.Reference), Default: true}, &confirmed); err != nil {
				return err
			}
			if !confirmed {
				return nil
			}
		}
		if err := svr.UpdateArtifact(project.OrgId, project.GroupId, project.ProjectId, runtime.ID, that.imageId, that.message); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(that.Out, "实例 %s 的镜像更新任务已提交\n", color.GreenString(runtime.Name))
		return nil
	}

	baseOptions, err := svr.ReleaseOptions(project.OrgId, project.GroupId, project.ProjectId, 0)
	if err != nil {
		return err
	}
	envID, clusterID, err := svr.SelectTarget(baseOptions)
	if err != nil {
		return err
	}
	clusterOptions, err := svr.ReleaseOptions(project.OrgId, project.GroupId, project.ProjectId, clusterID)
	if err != nil {
		return err
	}
	serviceName := availableServiceName(runtimes)
	if !that.yes {
		serviceName, err = svr.AskInstanceName(serviceName)
		if err != nil {
			return err
		}
	}
	networks := []string{}
	if len(clusterOptions.Networks) > 0 {
		selected := 0
		if len(clusterOptions.Networks) > 1 && !that.yes {
			titles := make([]string, len(clusterOptions.Networks))
			for i, network := range clusterOptions.Networks {
				titles[i] = fmt.Sprintf("%s (%s)", network.Name, network.Driver)
			}
			if err := survey.AskOne(&survey.Select{Message: "请选择容器网络", Options: titles, Default: titles[0]}, &selected); err != nil {
				return err
			}
		}
		networks = append(networks, clusterOptions.Networks[selected].ID)
	}
	defaultPort := 0
	for _, artifact := range clusterOptions.Artifacts {
		if artifact.ID == that.imageId {
			defaultPort = artifact.DefaultPort
			break
		}
	}
	ports := []map[string]interface{}{}
	if defaultPort > 0 {
		ports = append(ports, map[string]interface{}{"target": defaultPort, "published": clusterOptions.SuggestedPort, "protocol": "tcp", "mode": "ingress"})
	}
	version := imageTag(that.artifact.Reference)
	spec := map[string]interface{}{
		"service_name": serviceName, "replicas": 1, "command": []string{}, "args": []string{}, "networks": networks, "ports": ports,
		"mounts": []interface{}{}, "env": []interface{}{}, "configs": []interface{}{}, "secrets": []interface{}{},
		"resources": map[string]interface{}{"limits": map[string]interface{}{"cpus": 1, "memory_mb": 512}, "reservations": map[string]interface{}{"cpus": 0.1, "memory_mb": 128}},
		"update":    map[string]interface{}{"parallelism": 1, "delay_seconds": 0, "order": "stop-first", "failure_action": "rollback"},
	}
	if err := svr.CreateRelease(project.OrgId, project.GroupId, project.ProjectId, envID, clusterID, that.imageId, version, that.message, spec); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "发布请求已提交,你可以通过 `%s` 命令查看部署状态\n", color.GreenString("galaxy list deploy"))
	return nil
}

func imageTag(reference string) string {
	last := reference
	if slash := strings.LastIndex(last, "/"); slash >= 0 {
		last = last[slash+1:]
	}
	if colon := strings.LastIndex(last, ":"); colon >= 0 && colon+1 < len(last) {
		return last[colon+1:]
	}
	return "latest"
}

func availableServiceName(runtimes []*instance.Runtime) string {
	used := map[string]bool{}
	for _, runtime := range runtimes {
		used[runtime.Name] = true
	}
	if !used["web"] {
		return "web"
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("web-%d", i)
		if !used[candidate] {
			return candidate
		}
	}
	return "web-new"
}
