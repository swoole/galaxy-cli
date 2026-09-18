package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/service/organization"
	"github.com/gogf/gf/errors/gerror"
	"github.com/spf13/cobra"
)

type printOrgTable struct {
	Id      string `table:"ID"`
	Title   string `table:"名称"`
	TypeS   string `table:"类型"`
	StatusS string `table:"认证状态"`
	RoleS   string `table:"您的角色"`
	//CreateAt string `table:"加入时间"`
}

type optionsOrg struct {
	domain   string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsOrg(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsOrg {
	return &optionsOrg{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdOrg(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsOrg(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "organization",
		Short:   "组织",
		Long:    "组织信息",
		Aliases: []string{"org"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsOrg) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.domain = args[0]
	}
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()
	return nil
}

func (that *optionsOrg) Validate() error {
	return nil
}
func (that *optionsOrg) Run() (err error) {

	svr := organization.NewService(that.cfgFlags)
	orgs, err := svr.Simple()
	if err != nil {
		return err
	}
	if len(orgs) < 1 {
		return gerror.New("你还没有组织,赶快创建吧!")
	}

	var printTables []*printOrgTable

	for _, org := range orgs {
		title := org.GetTitle()
		if that.cfgFlags.GalaxyConfig.GetDefaultOrg() != nil {
			if org.Id == that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id {
				title = fmt.Sprintf("%s(当前)", org.GetTitle())
			}
		}
		t := &printOrgTable{
			Id:      fmt.Sprintf("#%d", org.GetId()),
			Title:   title,
			TypeS:   org.GetTypeS(),
			StatusS: org.GetStatusS(),
			RoleS:   org.GetRoleS(),
		}
		printTables = append(printTables, t)
	}

	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}
