package delete

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/project"
	"github.com/spf13/cobra"
)

type optionsRoute struct {
	domain   string
	location string
	router   *project.Route
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
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringVar(&o.location, "location", "", "路由路径")
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

	return nil
}
func (that *optionsRoute) Run() (err error) {
	svr := project.NewService(that.cfgFlags)
	routes, err := svr.Routes(that.project.OrgId, that.project.GroupId, that.project.ProjectId)
	if err != nil {
		return err
	}
	that.router, err = svr.SelectRoute("请选择你要删除的路由", routes, that.domain, that.location)
	if err != nil {
		return err
	}
	if that.router.Cluster == nil || that.router.Cluster.ID == 0 {
		return fmt.Errorf("路由缺少 Swarm 集群信息")
	}
	err = svr.DeleteRoute(that.project.OrgId, that.project.GroupId, that.project.ProjectId, that.router.Cluster.ID, that.router.ID, that.router.Source)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.Out, "删除域名 %s 的路由 %s 成功\n", that.domain, that.router.PathPrefix)
	return nil
}
