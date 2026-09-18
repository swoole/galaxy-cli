package delete

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/sslcert"
	"github.com/spf13/cobra"
)

type optionsCertificate struct {
	domain   string
	orgId    uint32
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsCertificate(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsCertificate {
	return &optionsCertificate{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdCertificate(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsCertificate(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "certificate",
		Short:   "证书",
		Long:    "证书",
		Aliases: []string{"cert", "ssl"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsCertificate) Complete(cmd *cobra.Command, args []string) error {

	if len(args) > 0 {
		that.domain = args[0]
	}
	org := that.cfgFlags.GalaxyConfig.GetDefaultOrg()
	if org != nil {
		that.orgId = org.Id
	}
	project := that.cfgFlags.ProjectConfig.DefaultProject()
	if project != nil {
		that.orgId = project.OrgId
	}

	return nil
}

func (that *optionsCertificate) Validate() error {

	if that.orgId == 0 {
		return fmt.Errorf("未找到对应的组织,请确定你已经登录,如未登录,可以执行 galaxy login 登录")
	}
	if len(that.domain) <= 0 {
		return fmt.Errorf("请传入你要删除的证书")
	}
	return nil
}
func (that *optionsCertificate) Run() (err error) {
	svr := sslcert.NewService(that.cfgFlags)
	certificates, err := svr.Find(that.orgId, that.domain)
	if err != nil {
		return err
	}
	if len(certificates) == 0 {
		return fmt.Errorf("你要删除的证书 %s 不存在,请确认后在操作", that.domain)
	}
	certificate := certificates[0]
	for _, item := range certificates {
		if item.Title == that.domain || item.CommonName == that.domain || fmt.Sprint(item.ID) == that.domain {
			certificate = item
			break
		}
	}
	_, _ = fmt.Fprintf(that.Out, "你正在删除证书 %s \n", certificate.Title)
	err = svr.DeleteCertificate(that.orgId, certificate.ID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "删除证书 %s 成功\n", certificate.Title)
	return nil
}
