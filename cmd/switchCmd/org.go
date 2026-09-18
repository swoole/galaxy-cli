package switchCmd

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/protoc"
	"galaxy/service/organization"
	"github.com/spf13/cobra"
)

type optionsOrg struct {
	orgName  string
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
		Long:    "组织",
		Aliases: []string{"org"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringVar(&o.orgName, "org", "", "传入组织名")
	return cmd
}

func (that *optionsOrg) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.orgName = args[0]
	}
	return nil
}

func (that *optionsOrg) Validate() error {
	//if len(that.orgName) <= 0 {
	//	return fmt.Errorf("你传入的参数不正确,正确的命令：%s ", color.GreenString("galaxy switch OrgName"))
	//}
	return nil
}

func (that *optionsOrg) Run() error {
	var org *protoc.Organization
	org, err := organization.NewService(that.cfgFlags).SelectedOrg("选择你要切换的组织", that.orgName)
	if org.GetId() <= 0 {
		return nil
	}
	that.cfgFlags.GalaxyConfig.SetOrgId(org.GetId())
	err = that.cfgFlags.GalaxyConfig.Save(that.cfgFlags.GetGalaxyConfigFile())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "切换组织成功,当前组织: [%s]\n", org.GetTitle())
	return nil
}
