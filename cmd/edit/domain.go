package edit

import (
	"bytes"
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/cmd/util/editor"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/project"
	"galaxy/service/sslcert"
	"github.com/gogf/gf/encoding/gyaml"
	"github.com/spf13/cobra"
	encoder "github.com/zwgblue/yaml-encoder"
	"os"
)

type optionsDomain struct {
	domain   string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsDomain(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *optionsDomain {
	return &optionsDomain{IOStreams: streams, cfgFlags: cfgFlags}
}

func newCmdDomain(cfgFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsDomain(cfgFlags, streams)
	return &cobra.Command{Use: "domain DOMAIN", Short: "编辑项目 Web 网关路由", Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(o.Complete(args))
		cmdutil.CheckErr(o.Validate())
		cmdutil.CheckErr(o.Run())
	}}
}

func (o *optionsDomain) Complete(args []string) error {
	if len(args) > 0 {
		o.domain = args[0]
	}
	o.project = o.cfgFlags.ProjectConfig.DefaultProject()
	return nil
}
func (o *optionsDomain) Validate() error {
	if o.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	if o.domain == "" {
		return fmt.Errorf("请传入你要编辑的域名")
	}
	return nil
}

func (o *optionsDomain) Run() error {
	p := o.project
	service := project.NewService(o.cfgFlags)
	routes, err := service.Routes(p.OrgId, p.GroupId, p.ProjectId)
	if err != nil {
		return err
	}
	route, err := service.SelectRoute("请选择你要编辑的路由", routes, o.domain, "")
	if err != nil {
		return err
	}
	if route.Cluster == nil || route.Cluster.ID == 0 {
		return fmt.Errorf("路由缺少 Swarm 集群信息")
	}
	certTitle := ""
	if route.CertificateID > 0 {
		certificates, err := sslcert.NewService(o.cfgFlags).Find(p.OrgId, "")
		if err != nil {
			return err
		}
		for _, certificate := range certificates {
			if certificate.ID == route.CertificateID {
				certTitle = certificate.Title
				break
			}
		}
	}
	model := &DomainInfoEdit{Hostname: route.Hostname, PathPrefix: route.PathPrefix, TargetService: route.TargetService, TargetPort: route.TargetPort, EnableHTTPS: route.TLSEnabled, ForceHTTPS: route.HTTPSRedirect, CertTitle: certTitle}
	yamlObject := encoder.NewEncoder(model, encoder.WithComments(encoder.CommentsOnHead))
	content, err := yamlObject.Encode()
	if err != nil {
		return err
	}
	edit := editor.NewDefaultEditor(editor.EditorEnvs())
	contents, path, err := edit.LaunchTempFile(fmt.Sprintf("route-%d-%s", route.ID, route.Hostname), ".yaml", bytes.NewBuffer(content))
	if err != nil {
		return err
	}
	defer os.Remove(path)
	var edited *DomainInfoEdit
	if err := gyaml.DecodeTo(cmdutil.ManualStrip(contents), &edited); err != nil {
		return err
	}
	if edited == nil || edited.Hostname == "" || edited.PathPrefix == "" || edited.TargetService == "" || edited.TargetPort == 0 {
		return fmt.Errorf("hostname、pathPrefix、targetService、targetPort 均不能为空")
	}
	certificateID := uint32(0)
	if edited.EnableHTTPS {
		if edited.CertTitle == "" {
			return fmt.Errorf("开启 HTTPS 必须填写 certTitle")
		}
		certificates, err := sslcert.NewService(o.cfgFlags).Find(p.OrgId, edited.CertTitle)
		if err != nil {
			return err
		}
		if len(certificates) == 0 {
			return fmt.Errorf("证书 %s 不存在", edited.CertTitle)
		}
		certificateID = certificates[0].ID
	}
	err = service.UpdateRoute(p.OrgId, p.GroupId, p.ProjectId, route.Cluster.ID, route.ID, route.Source, map[string]interface{}{
		"hostname": edited.Hostname, "path_prefix": edited.PathPrefix, "target_service": edited.TargetService,
		"target_port": edited.TargetPort, "tls_enabled": edited.EnableHTTPS, "https_redirect": edited.ForceHTTPS,
		"certificate_id": certificateID,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "路由 #%d 更新成功\n", route.ID)
	return nil
}

type DomainInfoEdit struct {
	Hostname      string `yaml:"hostname" comment:"域名，必须属于管理员授权给项目组的域名范围"`
	PathPrefix    string `yaml:"pathPrefix" comment:"URL 路径前缀"`
	TargetService string `yaml:"targetService" comment:"本项目的 Docker Swarm Service"`
	TargetPort    uint32 `yaml:"targetPort" comment:"容器目标端口"`
	EnableHTTPS   bool   `yaml:"enableHttps" comment:"是否启用 HTTPS"`
	ForceHTTPS    bool   `yaml:"forceHttps" comment:"是否将 HTTP 重定向到 HTTPS"`
	CertTitle     string `yaml:"certTitle" comment:"HTTPS 证书名称"`
}
