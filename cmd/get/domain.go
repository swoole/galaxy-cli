package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/service/project"
	"github.com/dustin/go-humanize"
	"github.com/gogf/gf/os/gtime"
	"github.com/spf13/cobra"
)

type printDomainTable struct {
	Domain       string `table:"域名"`
	ClusterTitle string `table:"集群"`
	Bandwidth    string `table:"带宽"`
	RouteCount   int    `table:"路由数目"`
	Creator      string `table:"创建人"`
	CreatorAt    string `table:"创建时间"`
}

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
	if len(routes) < 1 {
		_, _ = fmt.Fprintln(that.Out, "当前项目暂无路由规则")
		return nil
	}
	byHost := map[string]*printDomainTable{}
	var printTables []*printDomainTable
	for _, r := range routes {
		t := byHost[r.Hostname]
		if t == nil {
			t = &printDomainTable{Domain: r.Hostname, Bandwidth: "-", CreatorAt: humanize.Time(gtime.NewFromTimeStamp(r.CreatedAt).Time)}
			if r.Cluster != nil {
				t.ClusterTitle = r.Cluster.Title
			}
			byHost[r.Hostname] = t
			printTables = append(printTables, t)
		}
		t.RouteCount++
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}
