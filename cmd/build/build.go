package build

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/git"
	"galaxy/protoc"
	"galaxy/service/build"
	"galaxy/service/pipelines"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gogf/gf/container/garray"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/text/gstr"
	"github.com/gogf/gf/util/gconv"
	"github.com/spf13/cobra"
	"io"
)

type Options struct {
	gitObj    *git.Git
	branch    string // 分支
	commitId  string // commit
	tagName   string
	message   string
	reBuildId uint32
	pipeline  string
	fast      bool // 是否快速构建
	cfgFlags  *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
		reBuildId: 0,
		fast:      false,
	}
}

func NewCmdBuild(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "build",
		Short:   "构建项目",
		Long:    "通过流水线构建项目",
		Example: `galaxy build`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate(cmd, args))
			cmdutil.CheckErr(o.Run())
		},
		ValidArgsFunction: ValidArgsFunc,
	}
	cmd.Flags().StringVarP(&o.message, "message", "m", "", "构建说明")
	cmd.Flags().StringVarP(&o.pipeline, "pipeline", "p", "", "流水线名称")
	cmd.Flags().StringVarP(&o.commitId, "commit", "c", "", "指定要编译的版本commitId")
	cmd.Flags().StringVarP(&o.tagName, "tag", "t", "", "指定要编译的版本TagName")
	_ = cmd.RegisterFlagCompletionFunc("tag", tagCompletion)
	_ = cmd.RegisterFlagCompletionFunc("commit", commitCompletion)
	_ = cmd.RegisterFlagCompletionFunc("pipeline", pipelineCompletion)
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	that.gitObj = git.NewGit(that.cfgFlags.GalaxyProjectRoot())
	// 如果未传入参数,则表示可能是使用flags传入的参数或者获取当前环境的默认参数
	if len(args) == 0 {
		// commitid和tagname不能同时传入
		if len(that.commitId) > 0 && len(that.tagName) > 0 {
			return fmt.Errorf("构建命令不能同时传入tag和commitId")
		}
		// 如果出入了tag name 则优先使用
		if len(that.tagName) > 0 {
			return that.procTag(that.tagName)
		}
		// 使用commit id编译
		return that.procCommitID()
	}

	// 解析参数结构"main:8b28ec05-1304"
	// main表示分支，8b28ec05表示commitid，1304表示是已存在的构建，存在则表示是执行从新构建。
	// 以上格式不存在，则表示传入的是tagname，需要通过git查找tagname是否存在
	nameStr := args[0]

	// 如果是快速构建，则执行快速构建流程
	if nameStr == "fast" {
		that.fast = true
		return nil
	}
	// 先判断是否是rebuild的版本
	nameStrExplode := gstr.Explode("#", nameStr)
	if len(nameStrExplode) == 2 {
		that.reBuildId = gconv.Uint32(nameStrExplode[1])
		return nil
	}
	// 正常commit版本，需要带上分支
	nameStrExplode = gstr.Explode(":", nameStr)
	if len(nameStrExplode) == 2 {
		that.branch = nameStrExplode[0]
		that.commitId = nameStrExplode[1]
		return nil
	}
	// 其他情况统一认为是tag版本
	return that.procTag(nameStr)
}

func (that *Options) Validate(cmd *cobra.Command, args []string) error {
	// 快速构建，不执行参数检查
	if that.fast {
		return nil
	}
	// 重新构建，不需要其他参数
	if that.reBuildId > 0 {
		return nil
	}
	// 使用tag构建，需要获取tag信息
	if len(that.tagName) > 0 {
		_, err := that.gitObj.CheckTag(that.tagName)
		if err != nil {
			return gerror.Newf("你要构建的版本[%s]不存在", that.tagName)
		}
		// 特殊操作，如果是tag构建，则不需要传分支，分支名直接是tag名，commitId为空
		that.branch = that.tagName
		that.commitId = ""
		return nil
	}
	// 如果commitId还存在
	if len(that.commitId) > 0 {
		info, err := that.gitObj.CommitInfo(that.commitId)
		if err != nil {
			return gerror.Newf("你要构建的版本CommitId[%s]不存在", that.commitId)
		}
		if len(that.message) == 0 {
			that.message = info.Message
		}
	}

	return nil
}

func (that *Options) Run() error {
	project := that.cfgFlags.ProjectConfig.DefaultProject()
	if project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	if that.fast {
		return that.fastBuild(project)
	}
	// 重新构建
	if that.reBuildId > 0 {
		return that.reBuild(project, that.reBuildId)
	}
	return that.build(project)
}

// 重新构建
func (that *Options) reBuild(project *galaxycfg.Project, buildId uint32) error {
	svr := build.NewService(that.cfgFlags)
	info, err := svr.ReBuild(&protoc.BuildReReq{
		OrgId:     project.OrgId,
		GroupId:   project.GroupId,
		ProjectId: project.ProjectId,
		BuildId:   buildId,
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(that.Out, "重新开始构建%s %s %s %s\n等待构建中...\n",
		fmt.Sprintf("[%s:%s]", info.Branch, gstr.SubStr(info.CommitId, 0, 8)),
		fmt.Sprintf("%s(%s)", info.Pipeline.GetTitle(), info.Pipeline.GetRemark()),
		gstr.Trim(info.Remark),
		build.StatusString(info.Status),
	)
	err = that.buildLog(&protoc.BuildLogReq{
		OrgId:     info.OrgId,
		GroupId:   project.GroupId,
		ProjectId: project.ProjectId,
		BuildId:   info.Id,
	})
	if err != nil {
		return err
	}
	return nil
}

// 构建
func (that *Options) build(project *galaxycfg.Project) error {
	pipelineService := pipelines.NewService(that.cfgFlags)
	pipeLines, err := pipelineService.Simple(project.OrgId, project.GroupId, project.ProjectId)
	if err != nil {
		return err
	}
	if len(pipeLines) == 0 {
		return gerror.New("默认流水线不存在，请前往web端创建")
	}
	pipeline, err := pipelineService.AskPipelines("请选择构建的流水线:", pipeLines, that.pipeline)
	if err != nil {
		return err
	}
	if pipeline == nil {
		return gerror.New("未找到对应的流水线")
	}
	_, _ = fmt.Fprintf(that.Out, "准备构建...\n")
	svr := build.NewService(that.cfgFlags)
	info, err := svr.Build(&protoc.BuildReq{
		OrgId:      project.OrgId,
		GroupId:    project.GroupId,
		ProjectId:  project.ProjectId,
		PipelineId: pipeline.GetId(),
		Branch:     that.branch,
		CommitId:   that.commitId,
		Remark:     that.message,
	})
	if err != nil {
		return err
	}
	var versionStr string
	if len(info.CommitId) > 0 {
		versionStr = fmt.Sprintf("[%s:%s]", info.Branch, gstr.SubStr(info.CommitId, 0, 8))
	} else {
		versionStr = fmt.Sprintf("[%s]", info.Branch)
	}
	_, _ = fmt.Fprintf(that.Out, "开始构建%s %s \n%s \n%s\n等待构建中...\n",
		versionStr,
		fmt.Sprintf("%s(%s)", info.Pipeline.GetTitle(), info.Pipeline.GetRemark()),
		gstr.Trim(info.Remark),
		build.StatusString(info.Status),
	)
	err = that.buildLog(&protoc.BuildLogReq{
		OrgId:     info.OrgId,
		GroupId:   project.GroupId,
		ProjectId: project.ProjectId,
		BuildId:   info.Id,
	})
	if err != nil {
		return err
	}
	return nil
}

// 显示构建日志
func (that *Options) buildLog(req *protoc.BuildLogReq) error {
	svr := build.NewService(that.cfgFlags)
	var log = make(chan []byte, 512)
	err := svr.BuildLogWatch(req, log)
	if err != nil {
		return err
	}
	for {
		select {
		case m, ok := <-log:
			if !ok {
				return nil
			}
			_, _ = fmt.Fprint(that.Out, string(m))
		}
	}
}

// 快速构建
func (that *Options) fastBuild(project *galaxycfg.Project) error {
	_, _ = fmt.Fprintln(that.Out, "快速构建将使用当前 Git 分支、当前 Commit 和项目默认流水线")
	that.fast = false
	if err := that.procCommitID(); err != nil {
		return err
	}
	return that.build(project)
}

func ValidArgsFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	gitObj := git.NewGit(gfile.Pwd())
	currentBranch, err := gitObj.CurrentBranch()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	gitRepo, err := gitObj.Repository()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if gitRepo == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var limit = 20
	argsList := []string{"fast"}

	tagInfos, err := gitRepo.Tags()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var tagShortList = garray.NewStrArray()
	for {
		tagList, err := tagInfos.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		tagShortList.Append(tagList.Name().Short())
	}
	if tagShortList.Len() > 0 {
		argsList = append(argsList, tagShortList.PopRights(limit)...)
	}
	commitInfos, err := gitRepo.CommitObjects()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var commitList = garray.NewStrArray()
	for {
		commit, err := commitInfos.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		commitList.Append(fmt.Sprintf("%s:%s", currentBranch, gstr.SubStr(commit.ID().String(), 0, 8)))
	}
	if commitList.Len() > 0 {
		argsList = append(argsList, commitList.PopRights(limit)...)
	}
	return argsList, cobra.ShellCompDirectiveNoFileComp
}

func (that *Options) procTag(tagName string) error {

	that.tagName = tagName
	tagInfo, err := that.gitObj.CheckTag(that.tagName)
	if err != nil {
		return fmt.Errorf("获取tag信息出错, error: %v", err)
	}
	if len(that.message) == 0 {
		r, err := that.gitObj.Repository()
		if err != nil {
			return fmt.Errorf("获取Repository信息出错, error: %v", err)
		}
		var msg string
		obj1, err := r.TagObject(tagInfo.Hash())
		if err != nil && err == plumbing.ErrObjectNotFound {
			obj2, e1 := r.CommitObject(tagInfo.Hash())
			if e1 != nil {
				return fmt.Errorf("获取tag信息出错, error: %v", err)
			}
			msg = obj2.Message
		} else {
			msg = obj1.Message
		}

		if len(msg) > 0 {
			that.message = msg
		} else {
			that.message = "build tag " + that.tagName
		}
	}
	return nil
}

// 处理commitId
func (that *Options) procCommitID() (err error) {
	that.branch, err = that.gitObj.CurrentBranch()
	if err != nil {
		return err
	}
	var commitInfo *object.Commit
	if len(that.commitId) > 0 {
		commitInfo, err = that.gitObj.CommitInfo(that.commitId)
	} else {
		commitInfo, err = that.gitObj.CurrentCommit()
	}
	if err != nil {
		return err
	}
	that.commitId = commitInfo.ID().String()
	if len(that.message) == 0 {
		that.message = commitInfo.Message
	}
	return nil
}

func tagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var a []string
	c := galaxycfg.NewConfigFlags()
	err := c.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	r, err := git.NewGit(c.GalaxyProjectRoot()).Repository()
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}

	t, err := r.Tags()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var tagShortList = garray.NewStrArray()
	for {
		rf, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		tagShortList.Append(rf.Name().Short())
	}
	if tagShortList.Len() > 0 {
		a = append(a, tagShortList.PopRights(20)...)
	}
	return a, cobra.ShellCompDirectiveDefault
}
func commitCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var a []string
	c := galaxycfg.NewConfigFlags()
	err := c.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	r, err := git.NewGit(c.GalaxyProjectRoot()).Repository()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	t, err := r.CommitObjects()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var commitList = garray.NewStrArray()
	for {
		rf, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		commitList.Append(gstr.SubStr(rf.ID().String(), 0, 8))
	}
	if commitList.Len() > 0 {
		a = append(a, commitList.PopRights(20)...)
	}
	return a, cobra.ShellCompDirectiveDefault
}

func pipelineCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var titles []string
	c := galaxycfg.NewConfigFlags()
	err := c.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	project := c.ProjectConfig.DefaultProject()
	if project == nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	pipelineService := pipelines.NewService(c)
	pipeLines, err := pipelineService.Simple(project.OrgId, project.GroupId, project.ProjectId)
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	if len(pipeLines) == 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}
	for _, m := range pipeLines {
		titles = append(titles, m.Title)
	}
	return titles, cobra.ShellCompDirectiveDefault
}
