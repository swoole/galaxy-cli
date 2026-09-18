package get

import (
	"encoding/json"
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/service/sslcert"
	"github.com/gogf/gf/os/gtime"
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
		Short:   "SSL 证书",
		Long:    "显示组织可用的 SSL 证书",
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
	return nil
}

type printCertificateTable struct {
	Title       string `table:"证书名"`
	Domain      string `table:"域名"`
	Issuer      string `table:"签发机构"`
	Sign        string `table:"加密算法"`
	ExpiredAt   string `table:"到期时间"`
	IssueAt     string `table:"签发时间"`
	Fingerprint string `table:"指纹"`
}

func (that *optionsCertificate) Run() (err error) {
	l, err := sslcert.NewService(that.cfgFlags).Certificates(that.orgId)
	if err != nil {
		return err
	}
	if l.Total == 0 {
		_, _ = fmt.Fprintf(that.Out, "你还没有创建证书,赶快创建吧")
		return cmdutil.ErrExit
	}
	var printTables []*printCertificateTable
	for _, m := range l.Data {
		if that.domain != "" && m.CommonName != that.domain && m.Title != that.domain {
			continue
		}
		t := &printCertificateTable{
			Title: m.Title, Domain: m.CommonName, Issuer: certificateIssuer(m.Issuer), Sign: m.SignatureAlgorithm, Fingerprint: m.Fingerprint,
			ExpiredAt: gtime.NewFromTimeStamp(m.ValidTo).Format("Y-m-d H:i:s"), IssueAt: gtime.NewFromTimeStamp(m.ValidFrom).Format("Y-m-d H:i:s"),
		}
		printTables = append(printTables, t)
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}

func certificateIssuer(value string) string {
	var issuer map[string]interface{}
	if json.Unmarshal([]byte(value), &issuer) == nil {
		for _, key := range []string{"commonName", "organizationName", "CN", "O"} {
			if text, ok := issuer[key].(string); ok && text != "" {
				return text
			}
		}
	}
	return value
}
