package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	containerservice "galaxy/service/container"
	"galaxy/service/instance"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"strings"
	"time"
)

type printContainerTable struct {
	ID        string `table:"容器 ID"`
	Name      string `table:"容器名称"`
	Instance  string `table:"实例"`
	Service   string `table:"Service"`
	Cluster   string `table:"集群"`
	State     string `table:"状态"`
	Health    string `table:"健康"`
	Image     string `table:"镜像"`
	CreatedAt string `table:"创建时间"`
}

type optionsContainer struct {
	preferred string
	project   *galaxycfg.Project
	cfgFlags  *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newCmdContainer(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := &optionsContainer{cfgFlags: cfgFlags, IOStreams: streams}
	return &cobra.Command{
		Use:     "container [INSTANCE_OR_SERVICE]",
		Aliases: []string{"containers", "cont"},
		Short:   "项目实例的 Swarm 容器",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.run())
		},
	}
}

func (o *optionsContainer) complete(args []string) error {
	o.project = o.cfgFlags.ProjectConfig.DefaultProject()
	if o.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	if len(args) == 1 {
		o.preferred = strings.TrimPrefix(args[0], "#")
	}
	return nil
}

func (o *optionsContainer) run() error {
	p := o.project
	runtimes, err := instance.NewService(o.cfgFlags).Runtimes(p.OrgId, p.GroupId, p.ProjectId)
	if err != nil {
		return err
	}
	selected, err := selectContainerRuntime(runtimes, o.preferred)
	if err != nil {
		return err
	}
	clusterRuntimes := map[uint32][]*instance.Runtime{}
	for _, runtime := range runtimes {
		if selected != nil && runtime.ID != selected.ID {
			continue
		}
		if runtime.Cluster != nil && runtime.Cluster.ID > 0 {
			clusterRuntimes[runtime.Cluster.ID] = append(clusterRuntimes[runtime.Cluster.ID], runtime)
		}
	}
	service := containerservice.NewService(o.cfgFlags)
	rows := make([]*printContainerTable, 0)
	for clusterID, scopedRuntimes := range clusterRuntimes {
		items, err := service.Containers(p.OrgId, p.GroupId, p.ProjectId, clusterID)
		if err != nil {
			return err
		}
		for _, item := range items {
			runtime := containerRuntime(item, scopedRuntimes)
			if runtime == nil {
				continue
			}
			clusterTitle := fmt.Sprintf("#%d", clusterID)
			if runtime.Cluster != nil && runtime.Cluster.Title != "" {
				clusterTitle = runtime.Cluster.Title
			}
			serviceName := item.ServiceName
			if serviceName == "" {
				serviceName = runtime.ServiceName
			}
			rows = append(rows, &printContainerTable{
				ID: shortContainerID(item.ID), Name: item.Name, Instance: runtime.Name,
				Service: serviceName, Cluster: clusterTitle, State: item.State,
				Health: containerHealth(item.Health), Image: imageWithoutDigest(item.Image),
				CreatedAt: humanize.Time(time.Unix(item.CreatedAt, 0)),
			})
		}
	}
	if len(rows) == 0 {
		if o.preferred != "" {
			return fmt.Errorf("实例 %q 当前没有 Swarm 容器", o.preferred)
		}
		return fmt.Errorf("当前项目没有 Swarm 容器")
	}
	_, _ = fmt.Fprint(o.Out, output.PlainTable(rows))
	return nil
}

func selectContainerRuntime(runtimes []*instance.Runtime, preferred string) (*instance.Runtime, error) {
	if preferred == "" {
		return nil, nil
	}
	for _, runtime := range runtimes {
		if runtime.Name == preferred || runtime.ServiceName == preferred || runtime.RuntimeRef == preferred || fmt.Sprint(runtime.ID) == preferred {
			return runtime, nil
		}
	}
	return nil, fmt.Errorf("未找到实例或 Service %q", preferred)
}

func containerRuntime(item *containerservice.Container, runtimes []*instance.Runtime) *instance.Runtime {
	for _, runtime := range runtimes {
		if item.ServiceID != "" && item.ServiceID == runtime.RuntimeRef {
			return runtime
		}
		if item.ServiceName != "" && (item.ServiceName == runtime.ServiceName || item.ServiceName == runtime.RuntimeRef) {
			return runtime
		}
		if runtime.ServiceName != "" && strings.HasPrefix(item.Name, runtime.ServiceName+".") {
			return runtime
		}
	}
	return nil
}

func shortContainerID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func containerHealth(health string) string {
	if health == "" || health == "none" {
		return "-"
	}
	return health
}

func imageWithoutDigest(image string) string {
	if separator := strings.IndexByte(image, '@'); separator >= 0 {
		return image[:separator]
	}
	return image
}
