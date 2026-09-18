package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/service/project"
	"github.com/spf13/cobra"
)

type optionsRoute struct {
	domain   string
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
		Use:   "route",
		Short: "路由",
		Long:  "路由",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate(cmd, args))
			cmdutil.CheckErr(o.Run(cmd, args))
		},
	}
	return cmd
}

func (that *optionsRoute) Complete(cmd *cobra.Command, args []string) error {

	if len(args) > 0 {
		that.domain = args[0]
	}
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()

	return nil
}

func (that *optionsRoute) Validate(cmd *cobra.Command, args []string) error {

	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}
func (that *optionsRoute) Run(cmd *cobra.Command, args []string) (err error) {
	svr := project.NewService(that.cfgFlags)
	routes, err := svr.Routes(that.project.OrgId, that.project.GroupId, that.project.ProjectId)
	if err != nil {
		return err
	}
	var printTables []*printRouteTable
	for _, r := range routes {
		if that.domain != "" && r.Hostname != that.domain {
			continue
		}
		printTables = append(printTables, &printRouteTable{Domain: r.Hostname, Location: r.PathPrefix, Instance: fmt.Sprintf("#%d", r.ID), Service: r.TargetService, Port: fmt.Sprintf("%d", r.TargetPort), Remake: r.Status})
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}

type printRouteTable struct {
	Location string `table:"Location"`
	Domain   string `table:"域名"`
	Instance string `table:"实例"`
	Service  string `table:"网络服务"`
	Port     string `table:"服务端口"`
	Remake   string `table:"备注"`
}
