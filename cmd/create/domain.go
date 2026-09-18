package create

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/instance"
	"galaxy/service/project"
	"github.com/AlecAivazis/survey/v2"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/text/gregex"
	"github.com/spf13/cobra"
)

type optionsDomain struct {
	domain      string
	clusterName string
	runtime     *instance.Runtime
	project     *galaxycfg.Project
	cfgFlags    *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsDomain(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsDomain {
	return &optionsDomain{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdDomain(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsDomain(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:   "domain [NAME] [FLAGS]",
		Short: "域名",
		Long:  "域名",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringVar(&o.clusterName, "cluster", "", "集群名称")

	return cmd
}

func (that *optionsDomain) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.domain = args[0]
	}
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	instanceSvr := instance.NewService(that.cfgFlags)
	runtimes, err := instanceSvr.Runtimes(that.project.OrgId, that.project.GroupId, that.project.ProjectId)
	if err != nil {
		return err
	}
	if len(runtimes) == 0 {
		return fmt.Errorf("该项目还未部署实例,请先部署实例。")
	}
	if that.clusterName != "" {
		for _, runtime := range runtimes {
			if runtime.Cluster != nil && (runtime.Cluster.Title == that.clusterName || fmt.Sprint(runtime.Cluster.ID) == that.clusterName) {
				that.runtime = runtime
				break
			}
		}
		if that.runtime == nil {
			return fmt.Errorf("未找到集群 %q 中的项目 Runtime", that.clusterName)
		}
	} else {
		that.runtime, err = instanceSvr.SelectedRuntime("请选择要接入域名的实例", that.project.OrgId, that.project.GroupId, that.project.ProjectId, "")
		if err != nil {
			return err
		}
	}

	return nil
}

func (that *optionsDomain) Validate() error {
	if len(that.domain) <= 0 {
		return fmt.Errorf("请传入你要创建的域名")
	}
	match, err := gregex.MatchString(`^([0-9a-zA-Z][0-9a-zA-Z\-]{0,62}\.)+([a-zA-Z]{0,62})$`, that.domain)
	if err != nil {
		return err
	}
	if len(match) == 0 {
		return gerror.Newf("域名: %s 不符合规则", that.domain)
	}
	if that.runtime == nil || that.runtime.Cluster == nil {
		return fmt.Errorf("创建域名必须指定集群")
	}

	//if that.https && that.certSsl == nil {
	//	return fmt.Errorf("开启https必须传入要使用的证书")
	//}

	return nil
}
func (that *optionsDomain) Run() (err error) {
	svr := project.NewService(that.cfgFlags)
	port := uint32(0)
	if that.runtime.Spec != nil && len(that.runtime.Spec.Ports) == 1 {
		port = that.runtime.Spec.Ports[0].Target
	}
	if that.runtime.Spec != nil && len(that.runtime.Spec.Ports) > 1 {
		titles := make([]string, len(that.runtime.Spec.Ports))
		for i, p := range that.runtime.Spec.Ports {
			titles[i] = fmt.Sprintf("%s:%d", p.Protocol, p.Target)
		}
		selected := 0
		if err := survey.AskOne(&survey.Select{Message: "请选择网关目标端口", Options: titles, Default: titles[0]}, &selected); err != nil {
			return err
		}
		port = that.runtime.Spec.Ports[selected].Target
	}
	if port == 0 {
		return fmt.Errorf("Runtime 没有可用的容器端口，请使用 galaxy create route 并通过 --port 指定目标端口")
	}
	route, err := svr.CreateRoute(that.project.OrgId, that.project.GroupId, that.project.ProjectId, that.runtime.Cluster.ID, that.domain, "/", that.runtime.ServiceName, port, 0, false, false)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "域名 [%s] 的默认路由创建成功,ID:%d\n", that.domain, route.ID)
	return nil
}

//
//func (that *optionsDomain) getAttrs() *protoc.DomainListAttr {
//	var enable uint32 = 0
//	var http2 uint32 = 0
//	var forceHttps uint32 = 0
//	var certId uint32 = 0
//	hs := &protoc.Https{
//		Enable:     &enable,
//		Http2:      &http2,
//		ForceHttps: &forceHttps,
//		CertId:     &certId,
//	}
//	if that.https {
//		enable = 1
//		if that.http2 {
//			http2 = 1
//		}
//		if that.forceHttps {
//			forceHttps = 1
//		}
//		certId = that.certSsl.GetId()
//	}
//
//	return &protoc.DomainListAttr{
//		Https: hs,
//	}
//}
//
//func (that *optionsDomain) getRouters() []*protoc.Router {
//
//	return []*protoc.Router{&protoc.Router{
//		Prefix: "/",
//		PodId:  615,
//		Port:   80, ServiceId: 447,
//	}}
//}
