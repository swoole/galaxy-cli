package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/service/cluster"
	"github.com/dustin/go-humanize"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/gtime"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type printClusterTable struct {
	ID            string `table:"#"`
	Title         string `table:"名称"`
	Status        string `table:"状态"`
	Connection    string `table:"连接方式"`
	DockerVersion string `table:"Docker版本"`
	Envs          string `table:"环境"`
	Creator       string `table:"创建人"`
	CreateTimeAt  string `table:"创建时间"`
}

type optionsCluster struct {
	args     []string
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsCluster(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsCluster {
	return &optionsCluster{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdCluster(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsCluster(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "cluster",
		Short:   "集群",
		Long:    "集群",
		Aliases: []string{"clu"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsCluster) Complete(cmd *cobra.Command, args []string) error {
	that.args = args

	return nil
}

func (that *optionsCluster) Validate() error {

	return nil
}
func (that *optionsCluster) Run() (err error) {
	orgID := that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id
	var groupID, projectID uint32
	if current := that.cfgFlags.ProjectConfig.DefaultProject(); current != nil {
		orgID, groupID, projectID = current.OrgId, current.GroupId, current.ProjectId
	}
	clusters, err := cluster.NewService(that.cfgFlags).List(orgID, groupID, projectID)
	if err != nil {
		return err
	}
	if len(clusters) < 1 {
		return gerror.New("未找到可用的集群")
	}
	var printTables []*printClusterTable
	for _, c := range clusters {
		var envName []string
		for _, e := range c.Envs {
			envName = append(envName, e.Title)
		}
		Creator := "系统管理员"
		if c.CreatorInfo != nil && c.CreatorInfo.Nickname != "" {
			Creator = c.CreatorInfo.Nickname
		}
		t := &printClusterTable{
			ID:            fmt.Sprintf("%d", c.ID),
			Title:         c.Title,
			Status:        clusterStatus(c.Status),
			Connection:    clusterConnection(c.Endpoint),
			DockerVersion: valueOrDash(c.Version),
			Envs:          valueOrDash(gstr.Join(envName, "/")),
			Creator:       Creator,
			CreateTimeAt:  humanize.Time(gtime.NewFromTimeStamp(c.CreatedAt).Time),
		}
		printTables = append(printTables, t)
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}

func clusterStatus(status int) string {
	switch status {
	case 0:
		return "创建中"
	case 1:
		return "初始化中"
	case 2:
		return "待连接"
	case 3:
		return "就绪"
	case 4:
		return "离线"
	case 9:
		return "异常"
	default:
		return "未知"
	}
}

func clusterConnection(endpoint string) string {
	endpoint = gstr.ToLower(gstr.Trim(endpoint))
	switch {
	case gstr.HasPrefix(endpoint, "agent-ssh://"):
		return "Agent SSH"
	case gstr.HasPrefix(endpoint, "agent-local://"), gstr.HasPrefix(endpoint, "agent://"):
		return "Agent 本地"
	case gstr.HasPrefix(endpoint, "https://"):
		return "HTTPS/TLS"
	case gstr.HasPrefix(endpoint, "tcp://"), gstr.HasPrefix(endpoint, "http://"):
		return "TCP"
	case gstr.HasPrefix(endpoint, "unix://"):
		return "Unix Socket"
	case endpoint == "":
		return "-"
	default:
		return "其他"
	}
}

func valueOrDash(value string) string {
	if gstr.Trim(value) == "" {
		return "-"
	}
	return value
}
