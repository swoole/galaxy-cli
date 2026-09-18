package projectdiff

import (
	"bufio"
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	"galaxy/pkg/utils/templates"
	"galaxy/service/container"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/spf13/cobra"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const syncIgnoreFile = ".galaxyignore"

var syncExample = templates.Examples(`
  # 项目只有一个实例时，直接同步 Git 变更文件
  galaxy sync

  # 项目有多个实例时，必须指定实例名称
  galaxy sync development

  # 按照 git diff <commit> 的文件范围同步到唯一实例
  galaxy sync a1b2c3d

  # 同步指定 commit 之后直至当前工作目录的全部变更
  galaxy sync development --commit a1b2c3d

  # 预览将要同步的文件，不写入容器
  galaxy sync development --dry-run

  # 同时删除 Git 中已删除的实例文件
  galaxy sync development --delete

  # 项目根目录的 .galaxyignore 可排除敏感配置和本地文件

  # 指定容器和容器内项目根目录
  galaxy sync development --container abc123 --remote-root /var/www/html`)

type SyncOptions struct {
	instanceName string
	container    string
	remoteRoot   string
	baseCommit   string
	dryRun       bool
	allowDelete  bool
	cfgFlags     *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

type syncItem struct {
	GitStatus string `table:"Git"`
	Path      string `table:"文件"`
	Result    string `table:"操作"`
	localPath string
	remote    string
	skip      bool
	delete    bool
}

type syncIgnoreRules struct {
	matcher gitignore.Matcher
	loaded  bool
}

func NewCmdSync(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := &SyncOptions{cfgFlags: flags, IOStreams: streams}
	cmd := &cobra.Command{
		Use:     "sync [COMMIT|INSTANCE]",
		Short:   "将 Git 工作区变更文件同步到部署实例容器",
		Long:    "默认将当前项目 git status 中修改和新增的文件写入指定运行容器；位置参数为 commit 哈希或指定 --commit 后，同步该提交之后直至当前工作目录的全部变更。项目根目录的 .galaxyignore 使用类似 .gitignore 的规则排除禁止同步的文件。需要同时指定实例和 commit 时，请使用 galaxy sync INSTANCE --commit COMMIT。使用 --delete 时还会删除 Git 中已删除的实例文件；实例中同名路径为目录时会停止同步，需手动处理。",
		Example: syncExample,
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.run())
		},
	}
	cmd.Flags().StringVarP(&o.container, "container", "c", "", "指定运行容器名称或 ID")
	cmd.Flags().StringVar(&o.remoteRoot, "remote-root", "", "容器内项目根目录；默认读取容器工作目录")
	cmd.Flags().StringVar(&o.baseCommit, "commit", "", "同步指定 commit 之后直至当前工作目录的全部变更")
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "只显示同步计划，不修改容器")
	cmd.Flags().BoolVar(&o.allowDelete, "delete", false, "同时删除 Git 中已删除的实例文件")
	return cmd
}

func (o *SyncOptions) complete(args []string) error {
	if len(args) > 0 {
		o.instanceName = strings.TrimSpace(strings.TrimPrefix(args[0], "#"))
	}
	o.baseCommit = strings.TrimSpace(o.baseCommit)
	if o.baseCommit == "" && positionalCommitHash.MatchString(o.instanceName) {
		o.baseCommit = o.instanceName
		o.instanceName = ""
	}
	if o.remoteRoot != "" {
		o.remoteRoot = path.Clean(o.remoteRoot)
		if !path.IsAbs(o.remoteRoot) {
			return fmt.Errorf("--remote-root 必须是容器内的绝对路径")
		}
	}
	return nil
}

func (o *SyncOptions) run() error {
	project := o.cfgFlags.ProjectConfig.DefaultProject()
	if project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	changed, resolvedBase, err := gitChangedFiles(o.cfgFlags.GalaxyProjectRoot(), o.baseCommit)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		if resolvedBase != "" {
			_, _ = fmt.Fprintf(o.Out, "从 commit %s 至当前工作目录没有需要同步的变更文件\n", shortHash(resolvedBase))
		} else {
			_, _ = fmt.Fprintln(o.Out, "Git 工作区没有需要同步的变更文件")
		}
		return nil
	}
	ignoreRules, err := loadSyncIgnoreRules(o.cfgFlags.GalaxyProjectRoot())
	if err != nil {
		return err
	}

	shared := &Options{
		instanceName: o.instanceName,
		container:    o.container,
		remoteRoot:   o.remoteRoot,
		commandName:  "sync",
		cfgFlags:     o.cfgFlags,
		IOStreams:    o.IOStreams,
	}
	target, err := shared.selectTarget(project)
	if err != nil {
		return err
	}
	service := container.NewService(o.cfgFlags)
	remoteRoot, err := shared.resolveRemoteRoot(service, project, target)
	if err != nil {
		return err
	}

	items, pending, err := buildSyncItems(
		o.cfgFlags.GalaxyProjectRoot(),
		remoteRoot,
		changed,
		ignoreRules,
		o.allowDelete,
	)
	if err != nil {
		return err
	}
	if err := checkSyncTargetDirectories(items, func(remotePath string) (bool, error) {
		return remoteDirectoryExists(service, project, target, remotePath)
	}); err != nil {
		return err
	}

	instanceTitle := target.runtime.Name
	if instanceTitle == "" {
		instanceTitle = target.runtime.ServiceName
	}
	_, _ = fmt.Fprintln(o.Out, "◆ Git 工作区 → 部署实例文件同步")
	_, _ = fmt.Fprintf(o.Out, "  项目 #%d · 实例 %s · 容器 %s\n", project.ProjectId, instanceTitle, target.container.Name)
	if resolvedBase != "" {
		_, _ = fmt.Fprintf(o.Out, "  Git 基准 %s\n", shortHash(resolvedBase))
	}
	if ignoreRules.loaded {
		_, _ = fmt.Fprintf(o.Out, "  忽略规则 %s\n", filepath.Join(o.cfgFlags.GalaxyProjectRoot(), syncIgnoreFile))
	}
	_, _ = fmt.Fprintf(o.Out, "  本地 %s\n  实例 %s\n\n", o.cfgFlags.GalaxyProjectRoot(), remoteRoot)
	_, _ = fmt.Fprint(o.Out, output.PlainTable(items))
	_, _ = fmt.Fprintln(o.Out, "\n▲ 文件变更作用于容器可写层；Service 更新、容器重建或重新调度后将丢失。")

	if o.dryRun {
		_, _ = fmt.Fprintf(o.Out, "\nDry run 完成：%d 个变更待处理，%d 个跳过，未修改容器\n", pending, len(items)-pending)
		return nil
	}
	if pending == 0 {
		_, _ = fmt.Fprintf(o.Out, "\n没有可同步文件，%d 个变更已跳过\n", len(items))
		return nil
	}

	_, _ = fmt.Fprintf(o.Out, "\n开始处理 %d 个文件变更...\n", pending)
	synced := 0
	for _, item := range items {
		if item.skip {
			continue
		}
		action := "更新"
		if item.delete {
			action = "删除"
		}
		_, _ = fmt.Fprintf(o.Out, "[%d/%d] %s %s -> %s\n", synced+1, pending, action, item.Path, item.remote)
		if item.delete {
			if err := removeRemoteFile(service, project, target, item.remote); err != nil {
				return fmt.Errorf("删除实例文件 %s 失败: %w", item.remote, err)
			}
			synced++
			continue
		}
		if err := ensureRemoteDirectory(service, project, target, path.Dir(item.remote)); err != nil {
			return fmt.Errorf("创建 %s 失败: %w", path.Dir(item.remote), err)
		}
		content, err := os.ReadFile(item.localPath)
		if err != nil {
			return fmt.Errorf("读取本地文件 %s 失败: %w", item.Path, err)
		}
		if err := service.WriteFile(
			project.OrgId,
			project.GroupId,
			project.ProjectId,
			target.clusterID,
			target.container.ID,
			item.remote,
			content,
		); err != nil {
			return fmt.Errorf("写入实例文件 %s 失败: %w", item.remote, err)
		}
		synced++
	}
	_, _ = fmt.Fprintf(o.Out, "\n✓ 同步完成：成功 %d 个，跳过 %d 个\n", synced, len(items)-synced)
	return nil
}

func loadSyncIgnoreRules(localRoot string) (*syncIgnoreRules, error) {
	filePath := filepath.Join(localRoot, syncIgnoreFile)
	file, err := os.Open(filePath)
	if os.IsNotExist(err) {
		return &syncIgnoreRules{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取同步忽略规则 %s 失败: %w", filePath, err)
	}
	defer file.Close()

	patterns := make([]gitignore.Pattern, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, nil))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取同步忽略规则 %s 失败: %w", filePath, err)
	}
	return &syncIgnoreRules{
		matcher: gitignore.NewMatcher(patterns),
		loaded:  true,
	}, nil
}

func (r *syncIgnoreRules) matches(filePath string) bool {
	cleanPath := strings.TrimPrefix(filepath.ToSlash(filePath), "./")
	if cleanPath == syncIgnoreFile {
		return true
	}
	return r != nil && r.matcher != nil && r.matcher.Match(strings.Split(cleanPath, "/"), false)
}

func buildSyncItems(
	localRoot string,
	remoteRoot string,
	changed []changedFile,
	ignoreRules *syncIgnoreRules,
	allowDelete bool,
) ([]*syncItem, int, error) {
	items := make([]*syncItem, 0, len(changed))
	pending := 0
	for _, file := range changed {
		if file.Path == "" || path.IsAbs(file.Path) || path.Clean(file.Path) != file.Path || file.Path == ".." || strings.HasPrefix(file.Path, "../") {
			return nil, 0, fmt.Errorf("无效的 Git 文件路径 %q", file.Path)
		}
		localPath := filepath.Join(localRoot, filepath.FromSlash(file.Path))
		info, statErr := os.Stat(localPath)
		item := &syncItem{
			GitStatus: file.GitStatus,
			Path:      file.Path,
			localPath: localPath,
			remote:    path.Join(remoteRoot, file.Path),
		}
		switch {
		case ignoreRules.matches(file.Path):
			item.Result = "跳过：.galaxyignore"
			item.skip = true
		case os.IsNotExist(statErr):
			if allowDelete && strings.Contains(file.GitStatus, "D") {
				item.Result = "待删除"
				item.delete = true
				pending++
			} else {
				item.Result = "跳过：本地已删除"
				item.skip = true
			}
		case statErr != nil:
			return nil, 0, fmt.Errorf("读取本地文件 %s 失败: %w", file.Path, statErr)
		case !info.Mode().IsRegular():
			item.Result = "跳过：不是普通文件"
			item.skip = true
		default:
			item.Result = "待同步"
			pending++
		}
		items = append(items, item)
	}
	return items, pending, nil
}

func checkSyncTargetDirectories(items []*syncItem, isDirectory func(string) (bool, error)) error {
	for _, item := range items {
		if item.skip {
			continue
		}
		isDir, err := isDirectory(item.remote)
		if err != nil {
			return fmt.Errorf("检查实例路径 %s 失败: %w", item.remote, err)
		}
		if isDir {
			if item.delete {
				return fmt.Errorf("拒绝删除 %s：实例路径 %s 是目录。本次未修改任何文件，请手动处理路径冲突后重试", item.Path, item.remote)
			}
			return fmt.Errorf("拒绝同步 %s：实例路径 %s 是目录，写入文件可能清空目录内容。本次未写入任何文件，请手动处理路径冲突后重试", item.Path, item.remote)
		}
	}
	return nil
}

func remoteDirectoryExists(service *container.Service, project *galaxycfg.Project, target *target, remotePath string) (bool, error) {
	result, err := service.Exec(
		project.OrgId,
		project.GroupId,
		project.ProjectId,
		target.clusterID,
		target.container.ID,
		[]string{"test", "-d", remotePath},
		"",
	)
	if err != nil {
		return false, err
	}
	switch result.ExitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("容器命令退出码 %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
}

func ensureRemoteDirectory(
	service *container.Service,
	project *galaxycfg.Project,
	target *target,
	directory string,
) error {
	result, err := service.Exec(
		project.OrgId,
		project.GroupId,
		project.ProjectId,
		target.clusterID,
		target.container.ID,
		[]string{"mkdir", "-p", directory},
		"",
	)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("容器命令退出码 %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func removeRemoteFile(service *container.Service, project *galaxycfg.Project, target *target, remotePath string) error {
	result, err := service.Exec(
		project.OrgId,
		project.GroupId,
		project.ProjectId,
		target.clusterID,
		target.container.ID,
		[]string{"rm", "-f", "--", remotePath},
		"",
	)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("容器命令退出码 %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}
