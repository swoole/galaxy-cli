package rollback

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/service/deploy"
	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"strings"
)

type Options struct {
	releaseID uint32
	yes       bool
	cfgFlags  *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *Options {
	return &Options{cfgFlags: flags, IOStreams: streams}
}
func NewCmdRollback(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(flags, streams)
	cmd := &cobra.Command{Use: "rollback [RELEASE_ID]", Short: "回滚项目 Runtime 到指定历史发布", Run: func(cmd *cobra.Command, args []string) { cmdutil.CheckErr(o.Complete(args)); cmdutil.CheckErr(o.Run()) }}
	cmd.Flags().BoolVarP(&o.yes, "yes", "y", false, "跳过确认")
	return cmd
}
func (o *Options) Complete(args []string) error {
	if len(args) > 0 {
		if _, err := fmt.Sscan(strings.TrimPrefix(args[0], "#"), &o.releaseID); err != nil {
			return fmt.Errorf("发布 ID 格式不正确")
		}
	}
	return nil
}
func (o *Options) Run() error {
	p := o.cfgFlags.ProjectConfig.DefaultProject()
	if p == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	service := deploy.NewService(o.cfgFlags)
	if o.releaseID == 0 {
		list, err := service.Releases(p.OrgId, p.GroupId, p.ProjectId)
		if err != nil {
			return err
		}
		if len(list.Data) == 0 {
			return fmt.Errorf("没有可回滚的发布记录")
		}
		choices := list.Data
		titles := make([]string, len(choices))
		for i, release := range choices {
			titles[i] = fmt.Sprintf("#%d %s %s", release.ID, release.Version, release.Remark)
		}
		selected := 0
		if err := survey.AskOne(&survey.Select{Message: "请选择要回滚到的发布", Options: titles, Default: titles[0]}, &selected); err != nil {
			return err
		}
		o.releaseID = choices[selected].ID
	}
	if !o.yes {
		confirmed := false
		if err := survey.AskOne(&survey.Confirm{Message: fmt.Sprintf("确定回滚到发布 #%d?", o.releaseID), Default: false}, &confirmed); err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}
	if err := service.RollbackRelease(p.OrgId, p.GroupId, p.ProjectId, o.releaseID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "回滚到发布 #%d 的任务已提交\n", o.releaseID)
	return nil
}
