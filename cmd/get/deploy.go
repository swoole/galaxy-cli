package get

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/service/deploy"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"strings"
	"time"
)

type printDeployTable struct {
	ID        string `table:"ID"`
	Version   string `table:"版本"`
	Object    string `table:"部署对象"`
	Operation string `table:"操作"`
	Remark    string `table:"部署说明"`
	Status    string `table:"状态"`
	Creator   string `table:"部署人"`
	CreatedAt string `table:"部署时间"`
}
type optionsDeploy struct {
	project  *galaxycfg.Project
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptionsDeploy(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *optionsDeploy {
	return &optionsDeploy{cfgFlags: flags, IOStreams: streams}
}
func newCmdDeploy(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptionsDeploy(flags, streams)
	return &cobra.Command{Use: "deploy", Short: "Docker Swarm 发布记录", Run: func(cmd *cobra.Command, args []string) { cmdutil.CheckErr(o.Complete()); cmdutil.CheckErr(o.Run()) }}
}
func (o *optionsDeploy) Complete() error {
	o.project = o.cfgFlags.ProjectConfig.DefaultProject()
	if o.project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	return nil
}
func (o *optionsDeploy) Run() error {
	p := o.project
	releases, err := deploy.NewService(o.cfgFlags).Releases(p.OrgId, p.GroupId, p.ProjectId)
	if err != nil {
		return err
	}
	if len(releases.Data) == 0 {
		_, _ = fmt.Fprintln(o.Out, "当前项目暂无发布记录")
		return nil
	}
	rows := make([]*printDeployTable, 0, len(releases.Data))
	for _, release := range releases.Data {
		version := release.Version
		if version == "" && release.Artifact != nil {
			version = imageTagFromReference(release.Artifact.Reference)
		}
		if version == "" {
			version = "-"
		}
		object := "-"
		if release.Runtime != nil {
			object = release.Runtime.Name
		}
		if object == "-" && release.Cluster != nil && release.Env != nil {
			object = release.Env.Title + "/" + release.Cluster.Title
		}
		creator := "-"
		if release.CreatorInfo != nil {
			creator = release.CreatorInfo.Nickname
		}
		status := release.Status
		if status == "" {
			status = "未知"
		}
		operation := release.Operation
		if operation == "" {
			operation = "-"
		}
		rows = append(rows, &printDeployTable{ID: fmt.Sprintf("#%d", release.ID), Version: version, Object: object, Operation: operation, Remark: release.Remark, Status: status, Creator: creator, CreatedAt: humanize.Time(time.Unix(release.CreatedAt, 0))})
	}
	_, _ = fmt.Fprint(o.Out, output.PlainTable(rows))
	return nil
}
func imageTagFromReference(reference string) string {
	last := reference
	if slash := strings.LastIndex(last, "/"); slash >= 0 {
		last = last[slash+1:]
	}
	if colon := strings.LastIndex(last, ":"); colon >= 0 {
		return last[colon+1:]
	}
	return last
}
