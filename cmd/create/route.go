package create

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/instance"
	"galaxy/service/project"
	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

type optionsRoute struct {
	domain   string
	location string
	instance string
	service  string
	port     uint16
	remark   string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsRoute(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsRoute {
	return &optionsRoute{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdRoute(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsRoute(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "route",
		Short:   "路由",
		Long:    "路由",
		Example: "galaxy create route www.oa.com --location=/ --instance=default --service=ifadxr-svc --port=80 --remark=test",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringVar(&o.location, "location", "", "路由路径")
	cmd.Flags().StringVar(&o.instance, "instance", "", "实例名")
	cmd.Flags().StringVar(&o.service, "service", "", "服务名")
	cmd.Flags().Uint16Var(&o.port, "port", 0, "端口")
	cmd.Flags().StringVar(&o.remark, "remark", "", "备注")
	return cmd
}

func (that *optionsRoute) Complete(cmd *cobra.Command, args []string) error {

	if len(args) > 0 {
		that.domain = args[0]
	}
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}

	return nil
}

func (that *optionsRoute) Validate() error {
	if len(that.domain) <= 0 {
		return fmt.Errorf("请传入你要创建的域名")
	}

	if len(that.location) <= 0 {
		return fmt.Errorf("请输入你要创建的路由路径")
	}

	return nil
}
func (that *optionsRoute) Run() (err error) {
	instanceSvr := instance.NewService(that.cfgFlags)
	runtime, err := instanceSvr.SelectedRuntime("请选择你要操作的实例", that.project.OrgId, that.project.GroupId, that.project.ProjectId, that.instance)
	if err != nil {
		return err
	}
	if that.service != "" && that.service != runtime.ServiceName && that.service != runtime.RuntimeRef {
		return fmt.Errorf("Runtime %s 的 Service 是 %s，不是 %s", runtime.Name, runtime.ServiceName, that.service)
	}
	if runtime.Cluster == nil || runtime.Cluster.ID == 0 {
		return fmt.Errorf("Runtime %s 缺少 Swarm 集群信息", runtime.Name)
	}
	targetPort := uint32(that.port)
	if targetPort == 0 && runtime.Spec != nil {
		ports := runtime.Spec.Ports
		if len(ports) == 1 {
			targetPort = ports[0].Target
		}
		if len(ports) > 1 {
			titles := make([]string, len(ports))
			for i, port := range ports {
				titles[i] = fmt.Sprintf("%s:%d", port.Protocol, port.Target)
			}
			selected := 0
			if err := survey.AskOne(&survey.Select{Message: "请选择网关目标端口", Options: titles, Default: titles[0]}, &selected); err != nil {
				return err
			}
			targetPort = ports[selected].Target
		}
	}
	if targetPort == 0 {
		return fmt.Errorf("Runtime 没有可用的容器端口，请通过 --port 指定网关目标端口")
	}
	_, err = project.NewService(that.cfgFlags).CreateRoute(
		that.project.OrgId, that.project.GroupId, that.project.ProjectId, runtime.Cluster.ID,
		that.domain, that.location, runtime.ServiceName, targetPort, 0, false, false,
	)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "为域名 %s 创建路由 %s 成功,你可以访问 %s\n", that.domain, that.location, fmt.Sprintf("http://%s%s", that.domain, that.location))
	return nil
}
