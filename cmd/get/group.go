package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/service/group"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/gtime"
	"github.com/spf13/cobra"
)

type printGroupTable struct {
	Id           string `table:"ID"`
	Title        string `table:"名称"`
	ProjectCount int    `table:"项目数"`
	MemberCount  int    `table:"成员数"`
	CreatedAt    string `table:"创建时间"`
}

type optionsGroup struct {
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsGroup(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *optionsGroup {
	return &optionsGroup{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func newCmdGroup(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsGroup(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "group",
		Short:   "项目组",
		Long:    "项目组",
		Aliases: []string{"groups"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *optionsGroup) Complete(cmd *cobra.Command, args []string) error {

	return nil
}

func (that *optionsGroup) Validate() error {

	return nil
}
func (that *optionsGroup) Run() (err error) {
	svr := group.NewService(that.cfgFlags)
	groups, err := svr.GetList()
	if err != nil {
		return err
	}
	if len(groups.GetData()) < 1 {
		return gerror.New("你还没有项目组，请先在 Web 控制台创建项目组")
	}
	var printTables []*printGroupTable
	for _, group := range groups.GetData() {
		title := group.GetTitle()
		if that.cfgFlags.GalaxyConfig.GetDefaultOrg() != nil {
			if group.Id == that.cfgFlags.GalaxyConfig.GetDefaultOrg().DefaultGroupId {
				title = fmt.Sprintf("%s(当前)", group.GetTitle())
			}
		}
		t := &printGroupTable{
			Id:           fmt.Sprintf("#%d", group.GetId()),
			Title:        title,
			ProjectCount: int(group.GetProjectCount()),
			MemberCount:  int(group.GetMemberCount()),
			CreatedAt:    gtime.NewFromTimeStamp(group.GetCreatedAt()).Format("Y-m-d H:i:s"),
		}
		printTables = append(printTables, t)
	}
	_, _ = fmt.Fprint(that.Out, output.PlainTable(printTables))
	return nil
}
