package route

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/instance"
	"galaxy/service/project"
	"galaxy/service/sslcert"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/gogf/gf/os/gfile"
	"github.com/spf13/cobra"
	"regexp"
)

type Options struct {
	domain, location, instance, service, remark string
	port                                        uint16
	https, http2, forceHttps                    bool
	bandwidth                                   uint
	certName, certPem, privateKey               string
	project                                     *galaxycfg.Project
	cfgFlags                                    *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{IOStreams: ioStreams, cfgFlags: cfgFlags}
}

func NewCmdRoute(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{Use: "route DOMAIN", Short: "为项目创建 Web 网关路由", Example: "galaxy route api.example.com --location=/ --instance=web --port=80", Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(o.Complete(cmd, args))
		cmdutil.CheckErr(o.Validate())
		cmdutil.CheckErr(o.Run())
	}}
	cmd.Flags().BoolVar(&o.https, "https", false, "启用 HTTPS")
	cmd.Flags().BoolVar(&o.http2, "http2", false, "兼容参数；HTTP/2 由 Web 网关统一提供")
	cmd.Flags().BoolVar(&o.forceHttps, "force-https", false, "将 HTTP 重定向到 HTTPS")
	cmd.Flags().UintVar(&o.bandwidth, "bandwidth", 0, "兼容参数；新版网关按请求限流")
	cmd.Flags().StringVar(&o.certName, "cert-name", "", "已有证书名称或域名")
	cmd.Flags().StringVar(&o.certPem, "cert-pem", "", "要导入的证书路径")
	cmd.Flags().StringVar(&o.privateKey, "private-key", "", "证书私钥路径")
	cmd.Flags().StringVar(&o.location, "location", "/", "路由路径前缀")
	cmd.Flags().StringVar(&o.instance, "instance", "", "Runtime 名称或 ID")
	cmd.Flags().StringVar(&o.service, "service", "", "Swarm Service 名称")
	cmd.Flags().Uint16Var(&o.port, "port", 0, "容器目标端口")
	cmd.Flags().StringVar(&o.remark, "remark", "", "备注")
	return cmd
}

func (o *Options) Complete(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.domain = args[0]
	}
	o.project = o.cfgFlags.ProjectConfig.DefaultProject()
	if o.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}

func (o *Options) Validate() error {
	if !regexp.MustCompile(`^([0-9A-Za-z][0-9A-Za-z-]{0,62}\.)+[A-Za-z]{2,63}$`).MatchString(o.domain) {
		return fmt.Errorf("域名 %q 不符合规则", o.domain)
	}
	if o.location == "" || o.location[0] != '/' {
		return fmt.Errorf("路由路径必须以 / 开头")
	}
	if o.forceHttps {
		o.https = true
	}
	if o.https && ((o.certPem == "") != (o.privateKey == "")) {
		return fmt.Errorf("导入证书时必须同时传入 --cert-pem 和 --private-key")
	}
	return nil
}

func (o *Options) Run() error {
	p := o.project
	runtimeService := instance.NewService(o.cfgFlags)
	runtime, err := runtimeService.SelectedRuntime("请选择要接入网关的实例", p.OrgId, p.GroupId, p.ProjectId, o.instance)
	if err != nil {
		return err
	}
	if o.service != "" && o.service != runtime.ServiceName && o.service != runtime.RuntimeRef {
		return fmt.Errorf("Runtime %s 的 Service 是 %s，不是 %s", runtime.Name, runtime.ServiceName, o.service)
	}
	if runtime.Cluster == nil || runtime.Cluster.ID == 0 {
		return fmt.Errorf("Runtime %s 缺少 Swarm 集群信息", runtime.Name)
	}
	port, err := o.selectPort(runtime)
	if err != nil {
		return err
	}
	certificateID, err := o.certificateID()
	if err != nil {
		return err
	}
	route, err := project.NewService(o.cfgFlags).CreateRoute(p.OrgId, p.GroupId, p.ProjectId, runtime.Cluster.ID, o.domain, o.location, runtime.ServiceName, port, certificateID, o.https, o.forceHttps)
	if err != nil {
		return err
	}
	scheme := "http"
	if o.https {
		scheme = "https"
	}
	_, _ = fmt.Fprintf(o.Out, "路由 #%d 创建成功: %s -> %s:%d\n", route.ID, color.GreenString(scheme+"://"+o.domain+o.location), runtime.ServiceName, port)
	return nil
}

func (o *Options) selectPort(runtime *instance.Runtime) (uint32, error) {
	if o.port > 0 {
		return uint32(o.port), nil
	}
	if runtime.Spec == nil || len(runtime.Spec.Ports) == 0 {
		return 0, fmt.Errorf("Runtime 没有声明容器端口，请通过 --port 指定")
	}
	if len(runtime.Spec.Ports) == 1 {
		return runtime.Spec.Ports[0].Target, nil
	}
	titles := make([]string, len(runtime.Spec.Ports))
	for i, port := range runtime.Spec.Ports {
		titles[i] = fmt.Sprintf("%s:%d", port.Protocol, port.Target)
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: "请选择网关目标端口", Options: titles, Default: titles[0]}, &selected); err != nil {
		return 0, err
	}
	return runtime.Spec.Ports[selected].Target, nil
}

func (o *Options) certificateID() (uint32, error) {
	if !o.https {
		return 0, nil
	}
	service := sslcert.NewService(o.cfgFlags)
	name := o.certName
	if name == "" {
		name = o.domain
	}
	if o.certPem != "" {
		certPEM := gfile.GetContents(gfile.RealPath(o.certPem))
		keyPEM := gfile.GetContents(gfile.RealPath(o.privateKey))
		if certPEM == "" || keyPEM == "" {
			return 0, fmt.Errorf("证书或私钥文件为空")
		}
		certificate, err := service.Import(o.project.OrgId, name, certPEM, keyPEM)
		if err != nil {
			return 0, err
		}
		return certificate.ID, nil
	}
	certificates, err := service.Find(o.project.OrgId, name)
	if err != nil {
		return 0, err
	}
	if len(certificates) == 0 {
		return 0, fmt.Errorf("未找到适用于 %s 的证书；请传入 --cert-pem 和 --private-key 导入", o.domain)
	}
	if len(certificates) == 1 {
		return certificates[0].ID, nil
	}
	titles := make([]string, len(certificates))
	for i, certificate := range certificates {
		titles[i] = fmt.Sprintf("#%d %s (%s)", certificate.ID, certificate.Title, certificate.CommonName)
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: "请选择 TLS 证书", Options: titles, Default: titles[0]}, &selected); err != nil {
		return 0, err
	}
	return certificates[selected].ID, nil
}
