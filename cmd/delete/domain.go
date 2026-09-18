package delete

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/project"
	"github.com/spf13/cobra"
)

type optionsDomain struct {
	domain   string
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
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
		Use:   "domain",
		Short: "域名",
		Long:  "域名",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsDomain) Complete(cmd *cobra.Command, args []string) error {

	if len(args) > 0 {
		that.domain = args[0]
	}
	if len(that.domain) <= 0 {
		return fmt.Errorf("请传入你要删除的域名")
	}
	that.project = that.cfgFlags.ProjectConfig.DefaultProject()

	return nil
}

func (that *optionsDomain) Validate() error {
	if that.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}

func (that *optionsDomain) Run() (err error) {
	svr := project.NewService(that.cfgFlags)
	routes, err := svr.Routes(that.project.OrgId, that.project.GroupId, that.project.ProjectId)
	if err != nil {
		return err
	}
	var matched []*project.Route
	for _, route := range routes {
		if route.Hostname == that.domain {
			matched = append(matched, route)
		}
	}
	if len(matched) == 0 {
		return fmt.Errorf("未找到你要删除的域名 %s", that.domain)
	}
	for _, route := range matched {
		if route.Cluster == nil || route.Cluster.ID == 0 {
			return fmt.Errorf("路由 #%d 缺少 Swarm 集群信息", route.ID)
		}
		if err := svr.DeleteRoute(that.project.OrgId, that.project.GroupId, that.project.ProjectId, route.Cluster.ID, route.ID, route.Source); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(that.Out, "已删除域名 %s 的 %d 条项目路由\n", that.domain, len(matched))
	return nil
}
