package projectdiff

import (
	"bytes"
	"errors"
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/errcode"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/output"
	terminal "galaxy/pkg/utils/term"
	"galaxy/service/container"
	"galaxy/service/instance"
	"github.com/AlecAivazis/survey/v2"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Options struct {
	instanceName string
	container    string
	remoteRoot   string
	baseCommit   string
	resolvedBase string
	commandName  string
	contextLines int
	cfgFlags     *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

type target struct {
	runtime   *instance.Runtime
	container *container.Container
	clusterID uint32
}

type changedFile struct {
	Path      string
	GitStatus string
}

type comparison struct {
	GitStatus string `table:"Git"`
	Path      string `table:"文件"`
	Result    string `table:"实例对比"`
	diff      string
}

var positionalCommitHash = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

func NewCmdDiff(flags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := &Options{cfgFlags: flags, IOStreams: streams, contextLines: 3, commandName: "diff"}
	cmd := &cobra.Command{
		Use:   "diff [COMMIT|INSTANCE]",
		Short: "对比 Git 工作区变更与部署实例中的文件",
		Long:  "默认读取当前项目 git status 中的变更文件；位置参数为 commit 哈希或指定 --commit 后，读取该提交之后直至当前工作目录的全部变更文件，并通过 Galaxy API 对比运行实例容器内的对应文件。需要同时指定实例和 commit 时，请使用 galaxy diff INSTANCE --commit COMMIT。",
		Example: `  galaxy diff
  galaxy diff development
  galaxy diff 85c6f5bda110dd880da493761db0f5dcfc75683d
  galaxy diff development --commit 85c6f5bda110dd880da493761db0f5dcfc75683d`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.run())
		},
	}
	cmd.Flags().StringVarP(&o.container, "container", "c", "", "指定运行容器名称或 ID")
	cmd.Flags().StringVar(&o.remoteRoot, "remote-root", "", "容器内项目根目录；默认读取容器工作目录")
	cmd.Flags().StringVar(&o.baseCommit, "commit", "", "对比指定 commit 之后直至当前工作目录的全部变更")
	cmd.Flags().IntVarP(&o.contextLines, "context", "U", 3, "统一 diff 的上下文行数")
	return cmd
}

func (o *Options) complete(args []string) error {
	if len(args) > 0 {
		o.instanceName = strings.TrimSpace(strings.TrimPrefix(args[0], "#"))
	}
	if o.contextLines < 0 {
		return fmt.Errorf("--context 不能小于 0")
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

func (o *Options) run() error {
	project := o.cfgFlags.ProjectConfig.DefaultProject()
	if project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	files, resolvedBase, err := gitChangedFiles(o.cfgFlags.GalaxyProjectRoot(), o.baseCommit)
	if err != nil {
		return err
	}
	o.resolvedBase = resolvedBase
	if len(files) == 0 {
		if o.resolvedBase != "" {
			_, _ = fmt.Fprintf(o.Out, "从 commit %s 至当前工作目录没有待对比的变更文件\n", shortHash(o.resolvedBase))
		} else {
			_, _ = fmt.Fprintln(o.Out, "Git 工作区没有待对比的变更文件")
		}
		return nil
	}

	target, err := o.selectTarget(project)
	if err != nil {
		return err
	}
	containerService := container.NewService(o.cfgFlags)
	remoteRoot, err := o.resolveRemoteRoot(containerService, project, target)
	if err != nil {
		return err
	}

	results := make([]*comparison, 0, len(files))
	for _, file := range files {
		result, err := o.compareFile(containerService, project, target, remoteRoot, file)
		if err != nil {
			return fmt.Errorf("对比 %s 失败: %w", file.Path, err)
		}
		results = append(results, result)
	}

	instanceTitle := target.runtime.Name
	if instanceTitle == "" {
		instanceTitle = target.runtime.ServiceName
	}
	_, _ = fmt.Fprintf(o.Out, "◆ Git 工作区 → 部署实例文件差异\n")
	_, _ = fmt.Fprintf(o.Out, "  项目 #%d · 实例 %s · 容器 %s\n", project.ProjectId, instanceTitle, target.container.Name)
	if o.resolvedBase != "" {
		_, _ = fmt.Fprintf(o.Out, "  Git 基准 %s\n", shortHash(o.resolvedBase))
	}
	_, _ = fmt.Fprintf(o.Out, "  本地 %s\n  实例 %s\n\n", o.cfgFlags.GalaxyProjectRoot(), remoteRoot)
	_, _ = fmt.Fprint(o.Out, output.PlainTable(results))

	different := 0
	for _, result := range results {
		if result.Result == "一致" {
			continue
		}
		different++
		if result.diff != "" {
			renderedDiff := colorizeDiff(result.diff, terminal.AllowsColorOutput(o.Out))
			_, _ = fmt.Fprintf(o.Out, "\n%s", renderedDiff)
			if !strings.HasSuffix(result.diff, "\n") {
				_, _ = fmt.Fprintln(o.Out)
			}
		}
	}
	_, _ = fmt.Fprintf(o.Out, "\n汇总: %d 个 Git 变更文件，%d 个与运行实例不同，%d 个一致\n", len(results), different, len(results)-different)
	return nil
}

func (o *Options) selectTarget(project *galaxycfg.Project) (*target, error) {
	runtimes, err := instance.NewService(o.cfgFlags).Runtimes(
		project.OrgId,
		project.GroupId,
		project.ProjectId,
	)
	if err != nil {
		return nil, err
	}
	runtime, err := selectRuntimeForCommand(runtimes, o.instanceName, o.commandName)
	if err != nil {
		return nil, err
	}
	if runtime.Cluster == nil || runtime.Cluster.ID == 0 {
		return nil, fmt.Errorf("实例 %q 未绑定有效集群", runtime.Name)
	}
	if runtime.RuntimeRef == "" {
		return nil, fmt.Errorf("实例 %q 尚未关联运行资源", runtime.Name)
	}
	items, err := container.NewService(o.cfgFlags).ServiceContainers(
		project.OrgId,
		project.GroupId,
		project.ProjectId,
		runtime.Cluster.ID,
		runtime.RuntimeRef,
	)
	if err != nil {
		return nil, err
	}
	var matches []*container.Container
	for _, item := range items {
		if item.State != "running" || !containerBelongsToRuntime(item, runtime) {
			continue
		}
		if o.container != "" && containerMatches(item, o.container) {
			return &target{runtime: runtime, container: item, clusterID: runtime.Cluster.ID}, nil
		}
		matches = append(matches, item)
	}
	if o.container != "" {
		return nil, fmt.Errorf("实例 %q 中未找到运行容器 %q", runtime.Name, o.container)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("实例 %q 当前没有运行中的容器", runtime.Name)
	}
	if len(matches) == 1 {
		return &target{runtime: runtime, container: matches[0], clusterID: runtime.Cluster.ID}, nil
	}
	options := make([]string, len(matches))
	for index, item := range matches {
		options[index] = fmt.Sprintf("%s [%s]", item.Name, shortID(item.ID))
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{
		Message: "实例有多个运行副本，请选择要对比的容器",
		Options: options,
		Default: options[0],
	}, &selected); err != nil {
		return nil, err
	}
	return &target{runtime: runtime, container: matches[selected], clusterID: runtime.Cluster.ID}, nil
}

func selectRuntimeForCommand(runtimes []*instance.Runtime, preferred, commandName string) (*instance.Runtime, error) {
	if len(runtimes) == 0 {
		return nil, fmt.Errorf("当前项目没有可用的部署实例")
	}
	if preferred != "" {
		for _, runtime := range runtimes {
			if runtime.Name == preferred ||
				runtime.ServiceName == preferred ||
				runtime.RuntimeRef == preferred ||
				fmt.Sprint(runtime.ID) == preferred {
				return runtime, nil
			}
		}
		return nil, fmt.Errorf("未找到部署实例 %q", preferred)
	}
	if len(runtimes) == 1 {
		return runtimes[0], nil
	}
	names := make([]string, 0, len(runtimes))
	for _, runtime := range runtimes {
		name := runtime.Name
		if name == "" {
			name = runtime.ServiceName
		}
		if name == "" {
			name = fmt.Sprintf("#%d", runtime.ID)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if commandName == "" {
		commandName = "diff"
	}
	return nil, fmt.Errorf(
		"当前项目有 %d 个部署实例，必须指定实例名称：galaxy %s <INSTANCE>\n可用实例：%s",
		len(runtimes),
		commandName,
		strings.Join(names, "、"),
	)
}

func (o *Options) resolveRemoteRoot(service *container.Service, project *galaxycfg.Project, target *target) (string, error) {
	if o.remoteRoot != "" {
		return o.remoteRoot, nil
	}
	result, err := service.Exec(
		project.OrgId,
		project.GroupId,
		project.ProjectId,
		target.clusterID,
		target.container.ID,
		[]string{"pwd"},
		"",
	)
	if err != nil {
		return "", err
	}
	remoteRoot := path.Clean(strings.TrimSpace(result.Stdout))
	if result.ExitCode != 0 || !path.IsAbs(remoteRoot) {
		return "", fmt.Errorf("无法自动识别容器工作目录，请使用 --remote-root 指定: %s", strings.TrimSpace(result.Stderr))
	}
	return remoteRoot, nil
}

func (o *Options) compareFile(
	service *container.Service,
	project *galaxycfg.Project,
	target *target,
	remoteRoot string,
	file changedFile,
) (*comparison, error) {
	localPath := filepath.Join(o.cfgFlags.GalaxyProjectRoot(), filepath.FromSlash(file.Path))
	localContent, localExists, err := readLocalFile(localPath)
	if err != nil {
		return nil, err
	}
	remotePath := path.Join(remoteRoot, file.Path)
	remoteExists, err := remoteFileExists(service, project, target, remotePath)
	if err != nil {
		return nil, err
	}
	var remoteContent []byte
	if remoteExists {
		remoteContent, err = service.ReadFile(
			project.OrgId,
			project.GroupId,
			project.ProjectId,
			target.clusterID,
			target.container.ID,
			remotePath,
		)
		if err != nil {
			return nil, err
		}
	}

	result := &comparison{GitStatus: file.GitStatus, Path: file.Path}
	switch {
	case !localExists && !remoteExists:
		result.Result = "两端均缺失"
	case !localExists:
		result.Result = "本地已删除"
	case !remoteExists:
		result.Result = "实例中缺失"
	case bytes.Equal(localContent, remoteContent):
		result.Result = "一致"
	case isBinary(localContent) || isBinary(remoteContent):
		result.Result = "二进制不同"
	default:
		result.Result = "内容不同"
	}
	if result.Result != "一致" && result.Result != "二进制不同" && result.Result != "两端均缺失" {
		diff, err := unifiedDiff(remoteContent, localContent, remoteExists, localExists, file.Path, o.contextLines)
		if err != nil {
			return nil, err
		}
		result.diff = diff
	}
	return result, nil
}

func remoteFileExists(service *container.Service, project *galaxycfg.Project, target *target, remotePath string) (bool, error) {
	result, err := service.Exec(
		project.OrgId,
		project.GroupId,
		project.ProjectId,
		target.clusterID,
		target.container.ID,
		[]string{"test", "-f", remotePath},
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
		return false, fmt.Errorf("检查实例文件失败（退出码 %d）: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
}

func gitChangedFiles(root, baseRevision string) ([]changedFile, string, error) {
	repository, err := git.PlainOpen(root)
	if err != nil {
		return nil, "", fmt.Errorf("当前目录不是 Git 仓库: %w", err)
	}
	if baseRevision == "" {
		files, err := gitWorktreeChangedFiles(root)
		return files, "", err
	}

	baseHash, err := repository.ResolveRevision(plumbing.Revision(baseRevision))
	if err != nil {
		return nil, "", fmt.Errorf("无法解析 commit %q: %w", baseRevision, err)
	}
	baseCommit, err := repository.CommitObject(*baseHash)
	if err != nil {
		return nil, "", fmt.Errorf("读取 commit %q 失败: %w", baseRevision, err)
	}
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, "", fmt.Errorf("读取 commit %q 文件树失败: %w", baseRevision, err)
	}
	filePaths, err := gitDiffNameOnly(root, baseRevision)
	if err != nil {
		return nil, "", err
	}

	files := make([]changedFile, 0, len(filePaths))
	for _, filePath := range filePaths {
		files = append(files, changedFile{
			Path:      filePath,
			GitStatus: treeToWorktreeStatus(root, baseTree, filePath),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, baseHash.String(), nil
}

func gitDiffNameOnly(root, baseRevision string) ([]string, error) {
	command := exec.Command("git", "-C", root, "diff", "--name-only", "-z", baseRevision, "--")
	output, err := command.Output()
	if err != nil {
		message := ""
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			message = strings.TrimSpace(string(exitError.Stderr))
		}
		if message != "" {
			return nil, fmt.Errorf("执行 git diff --name-only %q 失败: %s", baseRevision, message)
		}
		return nil, fmt.Errorf("执行 git diff --name-only %q 失败: %w", baseRevision, err)
	}
	parts := bytes.Split(output, []byte{0})
	files := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		filePath := filepath.ToSlash(string(part))
		if _, exists := seen[filePath]; exists {
			continue
		}
		seen[filePath] = struct{}{}
		files = append(files, filePath)
	}
	sort.Strings(files)
	return files, nil
}

func gitWorktreeChangedFiles(root string) ([]changedFile, error) {
	// Native Git applies the repository's line-ending and filter settings. go-git's
	// worktree status can report a normalized, unchanged file as modified.
	command := exec.Command("git", "-C", root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	output, err := command.Output()
	if err != nil {
		message := ""
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			message = strings.TrimSpace(string(exitError.Stderr))
		}
		if message != "" {
			return nil, fmt.Errorf("读取 Git 工作区状态失败: %s", message)
		}
		return nil, fmt.Errorf("读取 Git 工作区状态失败: %w", err)
	}
	parts := bytes.Split(output, []byte{0})
	files := make([]changedFile, 0, len(parts)-1)
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		if len(part) < 4 || part[2] != ' ' {
			return nil, fmt.Errorf("Git 工作区状态格式无效: %q", part)
		}
		files = append(files, changedFile{
			Path:      filepath.ToSlash(string(part[3:])),
			GitStatus: string(part[:2]),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func treeToWorktreeStatus(root string, tree *object.Tree, filePath string) string {
	_, treeErr := tree.File(filePath)
	_, localErr := os.Stat(filepath.Join(root, filepath.FromSlash(filePath)))
	switch {
	case treeErr == object.ErrFileNotFound && localErr == nil:
		return "A "
	case treeErr == nil && os.IsNotExist(localErr):
		return "D "
	default:
		return "M "
	}
}

func readLocalFile(filePath string) ([]byte, bool, error) {
	content, err := os.ReadFile(filePath)
	if err == nil {
		return content, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

func unifiedDiff(remote, local []byte, remoteExists, localExists bool, filePath string, context int) (string, error) {
	from := "实例/" + filePath
	to := "工作区/" + filePath
	if !remoteExists {
		from = "/dev/null"
	}
	if !localExists {
		to = "/dev/null"
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(remote)),
		B:        difflib.SplitLines(string(local)),
		FromFile: from,
		ToFile:   to,
		Context:  context,
	})
	if err != nil || diff == "" {
		return diff, err
	}
	return fmt.Sprintf("diff --git %s %s\n%s", from, to, diff), nil
}

func colorizeDiff(diff string, color bool) string {
	if !color {
		return diff
	}
	lines := strings.SplitAfter(diff, "\n")
	for index, line := range lines {
		value := strings.TrimSuffix(line, "\n")
		suffix := ""
		if strings.HasSuffix(line, "\n") {
			suffix = "\n"
		}
		escape := ""
		switch {
		case strings.HasPrefix(value, "diff --git "):
			escape = "\x1b[1m"
		case strings.HasPrefix(value, "+++"), strings.HasPrefix(value, "+"):
			escape = "\x1b[32m"
		case strings.HasPrefix(value, "---"), strings.HasPrefix(value, "-"):
			escape = "\x1b[31m"
		case strings.HasPrefix(value, "@@"):
			escape = "\x1b[36m"
		}
		if escape != "" {
			lines[index] = escape + value + "\x1b[0m" + suffix
		}
	}
	return strings.Join(lines, "")
}

func containerBelongsToRuntime(item *container.Container, runtime *instance.Runtime) bool {
	if item == nil || runtime == nil {
		return false
	}
	if item.ServiceID != "" && item.ServiceID == runtime.RuntimeRef {
		return true
	}
	if item.ServiceName != "" && (item.ServiceName == runtime.ServiceName || item.ServiceName == runtime.RuntimeRef) {
		return true
	}
	return runtime.ServiceName != "" && strings.HasPrefix(item.Name, runtime.ServiceName+".")
}

func containerMatches(item *container.Container, preferred string) bool {
	if item.Name == preferred || item.ID == preferred {
		return true
	}
	return len(preferred) >= 4 && strings.HasPrefix(item.ID, preferred)
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func isBinary(content []byte) bool {
	const sniffLength = 8000
	if len(content) > sniffLength {
		content = content[:sniffLength]
	}
	return bytes.IndexByte(content, 0) >= 0
}
