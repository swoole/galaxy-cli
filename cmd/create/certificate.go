package create

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/sslcert"
	"github.com/gogf/gf/os/gfile"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

type optionsCertificate struct {
	domain     string
	certPem    string
	privateKey string
	project    *galaxycfg.Project
	cfgFlags   *galaxycfg.ConfigFlags
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
		Short:   "SSL 证书",
		Long:    "导入 PEM 格式的 SSL 证书和私钥",
		Aliases: []string{"cert", "ssl"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate(cmd, args))
			cmdutil.CheckErr(o.Run(cmd, args))
		},
	}
	cmd.Flags().StringVar(&o.certPem, "cert-pem", "", "证书路径")
	cmd.Flags().StringVar(&o.privateKey, "private-key", "", "证书私钥路径")
	return cmd
}

func (that *optionsCertificate) Complete(cmd *cobra.Command, args []string) error {

	if len(args) > 0 {
		that.domain = args[0]
	}
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}

	return nil
}

func (that *optionsCertificate) Validate(cmd *cobra.Command, args []string) error {

	if len(that.domain) <= 0 {
		return fmt.Errorf("请传入你要创建的域名")
	}

	if len(that.certPem) == 0 || !gfile.Exists(gfile.RealPath(that.certPem)) {
		return fmt.Errorf("你的证书路径未传入,请查看 galaxy create cert --help")
	}
	if len(that.privateKey) == 0 || !gfile.Exists(gfile.RealPath(that.privateKey)) {
		return fmt.Errorf("你的密钥路径未传入,请查看 galaxy create cert --help")
	}
	return nil
}
func (that *optionsCertificate) Run(cmd *cobra.Command, args []string) (err error) {

	certPem := gfile.GetContents(gfile.RealPath(that.certPem))
	privateKey := gfile.GetContents(gfile.RealPath(that.privateKey))

	if len(certPem) == 0 {
		return fmt.Errorf("你的证书内容为空")
	}
	if len(privateKey) == 0 {
		return fmt.Errorf("你的私钥内容为空")
	}
	certificate, err := sslcert.NewService(that.cfgFlags).Import(that.project.OrgId, that.domain, certPem, privateKey)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("证书 ID", 12), fmt.Sprintf("#%d", certificate.ID))
	_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("证书名称", 12), certificate.Title)
	_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("域名", 12), certificate.CommonName)
	_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("签发机构", 12), certificate.Issuer)
	_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("加密算法", 12), certificate.SignatureAlgorithm)
	_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("指纹", 12), certificate.Fingerprint)
	return nil
}
