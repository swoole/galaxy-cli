package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"galaxy/pkg/cliui"
	"galaxy/pkg/galaxycfg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type localComposeMigrationOptions struct {
	galaxycfg.IOStreams

	project         string
	projectDir      string
	composeFiles    []string
	stack           string
	expectedHash    string
	assumeYes       bool
	waitTimeout     time.Duration
	projectDirSet   bool
	composeFilesSet bool
	resolvedDir     string
	resolvedFiles   []string
	resolvedContent string
}

type composeProject struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	ConfigFiles string `json:"config_files"`
}

type composeMigrationSummary struct {
	Services int
	Networks int
	Volumes  int
	Configs  int
	Secrets  int
	Images   []string
	Warnings []string
	Failures []string
}

type composeBindPlacement struct {
	Content  string
	Services []string
	Failures []string
}

type composeStackNormalization struct {
	Content         string
	Services        []string
	RemovedRootName bool
}

func newCmdComposeMigration(ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := &localComposeMigrationOptions{IOStreams: ioStreams}
	composeCmd := &cobra.Command{
		Use:   "compose",
		Short: "在本机执行 Docker Compose 工具",
		Long:  "纯本地工具：直接调用当前主机的 docker compose，不访问 Galaxy API，也不属于 Galaxy 产品功能。",
	}
	composeCmd.AddCommand(newCmdComposeList(ioStreams))
	for _, action := range []string{
		"build", "up", "down", "start", "stop", "restart",
		"pull", "ps", "logs", "config", "exec", "run", "rm",
	} {
		composeCmd.AddCommand(newLocalComposeCommand(ioStreams, action))
	}
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "检查并将 Manager 本机的 Compose 项目迁移为 Swarm Stack",
		Long:  "纯本地工具：先检查 Compose 项目，检查通过并确认后迁移为 Swarm Stack；不调用 Galaxy API。",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return o.runConvertCommand(command)
		},
	}
	flags := migrateCmd.PersistentFlags()
	flags.StringVar(&o.project, "project", "", "Compose 项目名称")
	flags.StringVar(&o.projectDir, "project-directory", ".", "Compose 项目工作目录")
	flags.StringSliceVarP(&o.composeFiles, "file", "f", nil, "Compose 配置文件，可重复指定")
	flags.StringVar(&o.stack, "stack", "", "目标 Swarm Stack 名称，默认使用 Compose 项目名称")
	_ = migrateCmd.MarkPersistentFlagRequired("project")
	executeFlags := migrateCmd.Flags()
	executeFlags.StringVar(&o.expectedHash, "source-hash", "", "可选：要求最终配置匹配指定的 64 位指纹")
	executeFlags.BoolVarP(&o.assumeYes, "yes", "y", false, "检查通过后跳过 yes 确认")
	executeFlags.DurationVar(&o.waitTimeout, "wait", time.Minute, "等待 Stack 全部 Service 就绪的超时；设为 0 跳过等待")

	composeCmd.AddCommand(migrateCmd)
	return composeCmd
}

func (o *localComposeMigrationOptions) runConvertCommand(command *cobra.Command) error {
	o.captureExplicitComposeFlags(command)
	if err := o.runCheck(false); err != nil {
		return err
	}
	return o.runConvert()
}

func (o *localComposeMigrationOptions) captureExplicitComposeFlags(command *cobra.Command) {
	o.projectDirSet = command.Flags().Changed("project-directory")
	o.composeFilesSet = command.Flags().Changed("file")
}

func newCmdComposeList(ioStreams galaxycfg.IOStreams) *cobra.Command {
	format := "table"
	command := &cobra.Command{
		Use:   "list",
		Short: "列出当前 Docker 节点上的全部 Compose 项目",
		RunE: func(command *cobra.Command, _ []string) error {
			docker := exec.CommandContext(command.Context(), "docker", "compose", "ls", "--all", "--format", "json")
			output, diagnostic, err := commandOutput(docker)
			if err != nil {
				return fmt.Errorf("读取本机 Compose 项目失败：%s", commandFailure(diagnostic, err))
			}
			projects, err := parseComposeProjects(output)
			if err != nil {
				return err
			}
			if format == "json" {
				encoder := json.NewEncoder(ioStreams.Out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(projects)
			}
			printComposeProjects(ioStreams.Out, projects)
			return nil
		},
	}
	command.Flags().StringVarP(&format, "format", "o", format, "输出格式：table 或 json")
	command.PreRunE = func(*cobra.Command, []string) error {
		if format != "table" && format != "json" {
			return errors.New("--format 只支持 table 或 json")
		}
		return nil
	}
	return command
}

func newLocalComposeCommand(ioStreams galaxycfg.IOStreams, action string) *cobra.Command {
	return &cobra.Command{
		Use:                action + " [docker compose 参数...]",
		Short:              "在本机执行 docker compose " + action,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			docker := exec.CommandContext(command.Context(), "docker", append([]string{"compose", action}, args...)...)
			docker.Stdin = ioStreams.In
			docker.Stdout = ioStreams.Out
			docker.Stderr = ioStreams.ErrOut
			if err := docker.Run(); err != nil {
				return fmt.Errorf("docker compose %s 执行失败：%w", action, err)
			}
			return nil
		},
	}
}

func (o *localComposeMigrationOptions) runCheck(printNext bool) error {
	checks := make([]composeMigrationCheck, 0, 12)
	add := func(status, name, detail string) {
		checks = append(checks, composeMigrationCheck{Status: status, Name: name, Detail: sanitizeCell(detail)})
	}
	discoveryDetail, discoveryErr := o.discoverComposeInput()
	if discoveryErr != nil {
		add("失败", "Compose 项目发现", discoveryErr.Error())
		printMigrationChecks(o.Out, o.project, o.stack, "", composeMigrationSummary{}, checks, "")
		return errors.New("Compose 项目当前不可迁移")
	}
	if discoveryDetail != "" {
		add("通过", "Compose 项目发现", discoveryDetail)
	}
	stackName := strings.TrimSpace(o.stack)
	if stackName == "" {
		stackName = strings.TrimSpace(o.project)
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`).MatchString(o.project) {
		add("失败", "Compose 项目", "--project 必须以小写字母或数字开头，只能包含小写字母、数字、下划线和连字符")
		printMigrationChecks(o.Out, o.project, stackName, "", composeMigrationSummary{}, checks, "")
		return errors.New("Compose 项目当前不可迁移")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`).MatchString(stackName) {
		add("失败", "目标 Stack", "名称只能包含字母、数字、下划线、点号和连字符，最长 63 字符")
		printMigrationChecks(o.Out, o.project, stackName, "", composeMigrationSummary{}, checks, "")
		return errors.New("Compose 项目当前不可迁移")
	}

	directory, err := filepath.Abs(o.projectDir)
	if err == nil {
		directory, err = filepath.EvalSymlinks(directory)
	}
	if err != nil {
		add("失败", "项目目录", err.Error())
		printMigrationChecks(o.Out, o.project, stackName, "", composeMigrationSummary{}, checks, "")
		return errors.New("Compose 项目当前不可迁移")
	}
	o.resolvedDir = directory
	o.resolvedFiles = nil
	for _, name := range o.composeFiles {
		if !filepath.IsAbs(name) {
			name = filepath.Join(directory, name)
		}
		resolved, resolveErr := filepath.Abs(name)
		if resolveErr != nil {
			add("失败", "Compose 文件", resolveErr.Error())
			continue
		}
		o.resolvedFiles = append(o.resolvedFiles, resolved)
	}
	fileDetail := "Docker Compose 自动发现"
	if len(o.resolvedFiles) > 0 {
		fileDetail = strings.Join(o.resolvedFiles, ", ")
	}
	add("通过", "项目目录", directory)
	add("通过", "Compose 文件", fileDetail)

	versionOutput, versionErr := exec.Command("docker", "compose", "version", "--short").CombinedOutput()
	if versionErr != nil {
		add("失败", "Docker Compose", commandFailure(versionOutput, versionErr))
	} else {
		add("通过", "Docker Compose", strings.TrimSpace(string(versionOutput)))
	}

	content, configErr := o.composeOutput("config")
	if configErr != nil {
		add("失败", "Compose 配置解析", configErr.Error())
	} else if len(content) == 0 || len(content) > 4<<20 {
		add("失败", "Compose 配置解析", "最终配置必须在 1 字节到 4 MiB 之间")
	} else {
		o.resolvedContent = string(content)
		add("通过", "Compose 配置解析", fmt.Sprintf("最终配置 %s", humanBytes(len(content))))
	}

	var summary composeMigrationSummary
	if o.resolvedContent != "" {
		summary = analyzeComposeMigrationConfig(o.resolvedContent)
		for _, failure := range summary.Failures {
			add("失败", "Swarm 兼容性", failure)
		}
		for _, warning := range summary.Warnings {
			add("警告", "迁移风险", warning)
		}
	}

	swarmContext := inspectLocalSwarmContext()
	for _, check := range swarmContext.Checks {
		add(check.Status, check.Name, check.Detail)
	}

	if swarmContext.Ready && o.resolvedContent != "" {
		placement := applyComposeBindPlacement(o.resolvedContent, swarmContext.Hostname)
		for _, failure := range placement.Failures {
			add("失败", "本地存储放置约束", failure)
		}
		if len(placement.Failures) == 0 {
			o.resolvedContent = placement.Content
			if len(placement.Services) > 0 {
				add("通过", "本地存储放置约束", fmt.Sprintf(
					"Service %s 已固定到节点 %s",
					strings.Join(placement.Services, ", "), swarmContext.Hostname,
				))
			} else {
				add("通过", "本地存储放置约束", "未发现 bind mount 或 local Docker Volume")
			}
		}
	}

	if o.resolvedContent != "" {
		ports, portErr := normalizeComposeStackConfig(o.resolvedContent)
		if portErr != nil {
			add("失败", "端口配置规范化", portErr.Error())
		} else {
			o.resolvedContent = ports.Content
			if len(ports.Services) > 0 {
				add("通过", "端口配置规范化", fmt.Sprintf(
					"已将 Service %s 的 published/target 端口转换为 Swarm 整数格式",
					strings.Join(ports.Services, ", "),
				))
			}
			if ports.RemovedRootName {
				add("通过", "Stack Schema 规范化", "已移除 Compose v2 顶层 name；Swarm Stack 不支持该字段")
			}
		}
	}

	if configErr == nil && o.resolvedContent != "" {
		plan := swarmMigrationPlan{Stack: stackName, Content: o.resolvedContent, WorkingDir: o.resolvedDir}
		for _, check := range inspectSwarmPlan(plan, swarmContext.Ready) {
			add(check.Status, check.Name, check.Detail)
		}
	}

	containerOutput, containerErr := o.composeOutput("ps", "--all", "--format", "json")
	if containerErr != nil {
		add("警告", "现有 Compose 容器", containerErr.Error())
	} else {
		total, running, stopped, parseErr := parseComposeContainerStates(containerOutput)
		if parseErr != nil {
			add("警告", "现有 Compose 容器", parseErr.Error())
		} else if total == 0 {
			add("警告", "现有 Compose 容器", "未发现容器；将仅根据配置创建 Swarm Stack")
		} else {
			add("通过", "现有 Compose 容器", fmt.Sprintf("总计 %d，运行 %d，非运行 %d", total, running, stopped))
		}
	}

	sourceHash := ""
	if o.resolvedContent != "" {
		sourceHash = hashText(o.resolvedContent)
	}
	printMigrationChecks(o.Out, o.project, stackName, directory, summary, checks, sourceHash)
	for _, check := range checks {
		if check.Status == "失败" {
			return errors.New("Compose 项目当前不可迁移")
		}
	}

	next := []string{
		"galaxy docker compose migrate",
		"--project " + shellQuote(o.project),
		"--project-directory " + shellQuote(o.resolvedDir),
		"--stack " + shellQuote(stackName),
	}
	for _, file := range o.resolvedFiles {
		next = append(next, "-f "+shellQuote(file))
	}
	if printNext {
		cliui.New(o.Out).Hint("下一步", strings.Join(next, " "))
	}
	return nil
}

func (o *localComposeMigrationOptions) runConvert() error {
	actualHashText := hashText(o.resolvedContent)
	if o.expectedHash != "" {
		if len(o.expectedHash) != 64 {
			return errors.New("--source-hash 必须是 64 位配置指纹")
		}
		if !strings.EqualFold(actualHashText, o.expectedHash) {
			return errors.New("Compose 配置与 --source-hash 不匹配，操作已取消")
		}
	}
	if o.stack == "" {
		o.stack = o.project
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`).MatchString(o.stack) {
		return errors.New("--stack 只能包含字母、数字、下划线、点号和连字符，最长 63 字符")
	}
	plan := swarmMigrationPlan{Stack: o.stack, Content: o.resolvedContent, WorkingDir: o.resolvedDir}
	if err := plan.validate(); err != nil {
		return err
	}
	existingServices, err := plan.existingServices()
	if err != nil {
		return err
	}
	if len(existingServices) > 0 {
		return fmt.Errorf("目标 Stack %s 已存在 Service：%s；请重新执行 check 并选择其他 --stack", o.stack, strings.Join(existingServices, ", "))
	}
	if err := confirmSwarmMigration(
		o.In,
		o.Out,
		fmt.Sprintf("将下线 Compose 项目 %s，并创建 Swarm Stack %s；失败时自动恢复源项目。", o.project, o.stack),
		o.assumeYes,
	); err != nil {
		if errors.Is(err, errSwarmMigrationCancelled) {
			return nil
		}
		return err
	}
	existing, err := o.composeOutput("ps", "-q")
	if err != nil {
		return err
	}
	running, err := o.composeOutput("ps", "--status", "running", "-q")
	if err != nil {
		return err
	}
	existingIDs := strings.Fields(string(existing))
	runningIDs := strings.Fields(string(running))
	hadResources := len(existingIDs) > 0
	source := swarmMigrationSource{}
	if len(runningIDs) > 0 {
		source.Stop = func() error {
			return runDockerContainerAction("stop", runningIDs)
		}
		source.Restore = func() error {
			return runDockerContainerAction("start", runningIDs)
		}
	}
	ui := cliui.New(o.Out)
	result, convertErr := executeSwarmMigration(plan, source, o.waitTimeout, ui.Info)
	if convertErr != nil {
		return convertErr
	}
	ui.Success("转换成功：" + strings.TrimSpace(string(result)))
	if hadResources {
		ui.Info("原 Compose 容器已停止并保留，可用于人工回滚")
	}
	if o.waitTimeout == 0 {
		ui.Warning("已跳过 Stack 就绪等待，本次不会询问删除旧 Compose 容器或配置文件")
		return nil
	}
	deletableConfigFiles, sharedConfigFiles, referenceErr := o.partitionComposeConfigFiles()
	if referenceErr != nil {
		deletableConfigFiles = nil
		sharedConfigFiles = append([]string(nil), o.resolvedFiles...)
		ui.Warning("无法确认配置文件是否被其他 Compose 项目引用，本次禁止自动删除配置：" + referenceErr.Error())
	}
	cleanup, cleanupErr := confirmComposeCleanup(
		o.In, o.Out, o.project, existingIDs, deletableConfigFiles, sharedConfigFiles,
	)
	if cleanupErr != nil {
		ui.Warning("迁移已成功，但无法读取清理确认：" + cleanupErr.Error())
		ui.Hint("手动检查", composeContainerCleanupHint(existingIDs))
		return nil
	}
	if !cleanup {
		ui.Info("已保留旧 Compose 容器和配置文件")
		ui.Hint("稍后检查", composeContainerCleanupHint(existingIDs))
		return nil
	}
	if hadResources {
		ui.Info("正在删除旧 Compose 容器")
		if err := removeComposeContainers(o.project, existingIDs); err != nil {
			ui.Warning("Stack 迁移已成功，但旧 Compose 容器删除失败：" + err.Error())
			ui.Hint("手动检查", composeContainerCleanupHint(existingIDs))
			return nil
		}
		ui.Success("旧 Compose 容器已删除")
	}
	if len(deletableConfigFiles) > 0 {
		ui.Info("正在删除未被其他 Compose 项目引用的旧配置文件")
		if err := removeComposeConfigFiles(deletableConfigFiles); err != nil {
			ui.Warning("Stack 迁移已成功，但部分 Compose 配置文件删除失败：" + err.Error())
			return nil
		}
		ui.Success("旧 Compose 配置文件已删除；项目目录、命名卷和数据均已保留")
	}
	if len(sharedConfigFiles) > 0 {
		ui.Warning("被其他 Compose 项目引用的配置文件已保留")
	}
	return nil
}

func confirmComposeCleanup(
	input io.Reader,
	output io.Writer,
	project string,
	containerIDs []string,
	configFiles []string,
	sharedConfigFiles []string,
) (bool, error) {
	ui := cliui.New(output)
	ui.Title("迁移后清理", "Swarm Stack 已就绪，旧资源不再提供服务")
	summary := []cliui.Pair{
		{Label: "Compose 项目", Value: project},
		{Label: "旧容器", Value: fmt.Sprintf("%d 个（当前为停止状态）", len(containerIDs))},
		{Label: "配置文件", Value: fmt.Sprintf("%d 个", len(configFiles))},
	}
	ui.Summary(summary)
	for _, file := range configFiles {
		ui.Hint("待删除配置", file)
	}
	for _, file := range sharedConfigFiles {
		ui.Hint("共享配置（保留）", file)
	}
	ui.Warning("本次只删除旧 Compose 容器和以上配置文件；项目目录、命名卷及其数据不会删除。")
	if input == nil {
		return false, errors.New("标准输入不可用")
	}
	fmt.Fprint(output, "是否删除旧 Compose 容器和配置文件？请输入 yes 确认，其他输入保留: ")
	answer, err := readConfirmationLine(input)
	if err != nil && strings.TrimSpace(answer) == "" {
		return false, err
	}
	if !strings.EqualFold(strings.TrimSpace(answer), "yes") {
		ui.Info("未确认清理，旧资源已保留")
		return false, nil
	}
	ui.Success("已确认清理旧 Compose 容器和配置文件")
	return true, nil
}

func runDockerContainerAction(action string, containerIDs []string) error {
	if len(containerIDs) == 0 {
		return nil
	}
	args := append([]string{"container", action}, containerIDs...)
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行 docker %s 失败：%s", strings.Join(args, " "), commandFailure(output, err))
	}
	return nil
}

func removeComposeContainers(project string, containerIDs []string) error {
	validated := make([]string, 0, len(containerIDs))
	for _, containerID := range containerIDs {
		output, err := exec.Command(
			"docker", "inspect", "--type", "container",
			"--format", `{{index .Config.Labels "com.docker.compose.project"}}`,
			containerID,
		).CombinedOutput()
		if err != nil {
			if strings.Contains(strings.ToLower(string(output)), "no such") {
				continue
			}
			return fmt.Errorf("删除前校验容器 %s 失败：%s", containerID, commandFailure(output, err))
		}
		actualProject := strings.TrimSpace(string(output))
		if actualProject != project {
			return fmt.Errorf(
				"安全检查拒绝删除容器 %s：Compose 项目标签为 %q，预期为 %q",
				containerID, actualProject, project,
			)
		}
		validated = append(validated, containerID)
	}
	return runDockerContainerAction("rm", validated)
}

func removeComposeConfigFiles(files []string) error {
	var failures []string
	for _, file := range files {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			failures = append(failures, fmt.Sprintf("%s：%v", file, err))
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "；"))
	}
	return nil
}

func (o *localComposeMigrationOptions) partitionComposeConfigFiles() (
	deletable []string,
	shared []string,
	err error,
) {
	output, diagnostic, commandErr := commandOutput(
		exec.Command("docker", "compose", "ls", "--all", "--format", "json"),
	)
	if commandErr != nil {
		return nil, nil, fmt.Errorf("读取 Compose 项目清单失败：%s", commandFailure(diagnostic, commandErr))
	}
	projects, parseErr := parseComposeProjects(output)
	if parseErr != nil {
		return nil, nil, parseErr
	}
	referenced := map[string]struct{}{}
	for _, project := range projects {
		if project.Name == o.project {
			continue
		}
		files := splitComposeConfigFiles(project.ConfigFiles)
		if len(files) == 0 || hasRelativePath(files) {
			_, discoveredFiles, discoverErr := inspectComposeProjectLabels(project.Name)
			if discoverErr != nil {
				return nil, nil, fmt.Errorf("无法确认项目 %s 的配置引用：%w", project.Name, discoverErr)
			}
			files = discoveredFiles
		}
		for _, file := range files {
			if !filepath.IsAbs(file) {
				return nil, nil, fmt.Errorf("项目 %s 返回相对配置路径 %s", project.Name, file)
			}
			referenced[filepath.Clean(file)] = struct{}{}
		}
	}
	for _, file := range o.resolvedFiles {
		if _, exists := referenced[filepath.Clean(file)]; exists {
			shared = append(shared, file)
		} else {
			deletable = append(deletable, file)
		}
	}
	return deletable, shared, nil
}

func composeContainerCleanupHint(containerIDs []string) string {
	if len(containerIDs) == 0 {
		return "没有待清理的旧 Compose 容器"
	}
	quoted := make([]string, 0, len(containerIDs))
	for _, containerID := range containerIDs {
		quoted = append(quoted, shellQuote(containerID))
	}
	return "核对项目标签后执行 docker container rm " + strings.Join(quoted, " ")
}

func (o *localComposeMigrationOptions) composeOutput(action string, extra ...string) ([]byte, error) {
	args := []string{"compose", "--project-name", o.project, "--project-directory", o.resolvedDir}
	for _, file := range o.resolvedFiles {
		args = append(args, "-f", file)
	}
	args = append(args, action)
	args = append(args, extra...)
	command := exec.Command("docker", args...)
	command.Dir = o.resolvedDir
	output, diagnostic, err := commandOutput(command)
	if err != nil {
		return output, fmt.Errorf("执行 docker %s 失败：%s", strings.Join(args, " "), commandFailure(diagnostic, err))
	}
	return output, nil
}

func (o *localComposeMigrationOptions) discoverComposeInput() (string, error) {
	if o.composeFilesSet || o.projectDirSet {
		return "", nil
	}
	command := exec.Command("docker", "compose", "ls", "--all", "--format", "json")
	output, diagnostic, err := commandOutput(command)
	if err != nil {
		return "", fmt.Errorf("读取本机 Compose 项目失败：%s", commandFailure(diagnostic, err))
	}
	projects, err := parseComposeProjects(output)
	if err != nil {
		return "", err
	}
	selected, err := selectComposeProject(o.project, projects)
	if err != nil {
		return "", err
	}
	o.project = selected.Name
	files := splitComposeConfigFiles(selected.ConfigFiles)
	workingDir := ""
	if len(files) == 0 || hasRelativePath(files) {
		labelDir, labelFiles, labelErr := inspectComposeProjectLabels(o.project)
		if labelErr == nil {
			workingDir = labelDir
			if len(labelFiles) > 0 {
				files = labelFiles
			}
		}
	}
	if len(files) == 0 {
		return "", fmt.Errorf(
			"项目 %s 没有可发现的 Compose 配置文件；请显式指定 --project-directory 和 -f",
			o.project,
		)
	}
	if workingDir == "" {
		first := files[0]
		if filepath.IsAbs(first) {
			workingDir = filepath.Dir(first)
		}
	}
	if workingDir == "" {
		absoluteDir, resolveErr := filepath.Abs(o.projectDir)
		if resolveErr != nil {
			return "", fmt.Errorf("解析 Compose 工作目录失败：%w", resolveErr)
		}
		workingDir = absoluteDir
	}
	resolvedFiles := make([]string, 0, len(files))
	for _, file := range files {
		if !filepath.IsAbs(file) {
			file = filepath.Join(workingDir, file)
		}
		resolvedFiles = append(resolvedFiles, filepath.Clean(file))
	}
	o.projectDir = workingDir
	o.composeFiles = resolvedFiles
	return fmt.Sprintf(
		"自动选择项目 %s，目录 %s，配置 %s",
		o.project, workingDir, strings.Join(resolvedFiles, ", "),
	), nil
}

func selectComposeProject(requested string, projects []composeProject) (composeProject, error) {
	if requested == "" {
		return composeProject{}, errors.New("必须指定 --project")
	}
	for _, project := range projects {
		if project.Name == requested {
			return project, nil
		}
	}
	return composeProject{}, fmt.Errorf(
		"未发现 Compose 项目 %s；当前项目：%s",
		requested, composeProjectNames(projects),
	)
}

func composeProjectNames(projects []composeProject) string {
	if len(projects) == 0 {
		return "无"
	}
	names := make([]string, 0, len(projects))
	for _, project := range projects {
		names = append(names, project.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func splitComposeConfigFiles(value string) []string {
	files := make([]string, 0)
	for _, file := range strings.Split(value, ",") {
		if file = strings.TrimSpace(file); file != "" {
			files = append(files, file)
		}
	}
	return files
}

func hasRelativePath(paths []string) bool {
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return true
		}
	}
	return false
}

func inspectComposeProjectLabels(project string) (workingDir string, files []string, err error) {
	output, err := exec.Command(
		"docker", "container", "ls", "--all",
		"--filter", "label=com.docker.compose.project="+project,
		"--format", "{{.ID}}",
	).CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("查询 Compose 项目容器失败：%s", commandFailure(output, err))
	}
	ids := strings.Fields(string(output))
	if len(ids) == 0 {
		return "", nil, errors.New("Compose 项目没有可用于读取配置标签的容器")
	}
	labelOutput, err := exec.Command(
		"docker", "inspect", "--format", "{{json .Config.Labels}}", ids[0],
	).CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("读取 Compose 项目标签失败：%s", commandFailure(labelOutput, err))
	}
	var labels map[string]string
	if err := json.Unmarshal(labelOutput, &labels); err != nil {
		return "", nil, fmt.Errorf("解析 Compose 项目标签失败：%w", err)
	}
	return strings.TrimSpace(labels["com.docker.compose.project.working_dir"]),
		splitComposeConfigFiles(labels["com.docker.compose.project.config_files"]), nil
}

func parseComposeProjects(output []byte) ([]composeProject, error) {
	rows, err := decodeJSONObjectList(output)
	if err != nil {
		return nil, fmt.Errorf("解析 docker compose ls 输出失败：%w", err)
	}
	projects := make([]composeProject, 0, len(rows))
	for _, row := range rows {
		name := mapString(row, "Name", "name")
		if name == "" {
			continue
		}
		projects = append(projects, composeProject{
			Name:        sanitizeCell(name),
			Status:      sanitizeCell(mapString(row, "Status", "status")),
			ConfigFiles: sanitizeCell(mapString(row, "ConfigFiles", "configFiles", "config_files")),
		})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

func printComposeProjects(output io.Writer, projects []composeProject) {
	ui := cliui.New(output)
	ui.Title("Compose 项目", fmt.Sprintf("当前节点 · 共 %d 个项目", len(projects)))
	if len(projects) == 0 {
		ui.Empty("当前 Docker 节点没有 Compose 项目")
		return
	}
	rows := make([][]string, 0, len(projects))
	for _, project := range projects {
		rows = append(rows, []string{project.Name, ui.State(project.Status), project.ConfigFiles})
	}
	ui.Table([]cliui.Column{
		{Header: "PROJECT", WidthMax: 32},
		{Header: "STATUS", WidthMax: 24},
		{Header: "CONFIG FILES", WidthMax: 80},
	}, rows)
}

func parseComposeContainerStates(output []byte) (total, running, stopped int, err error) {
	rows, err := decodeJSONObjectList(output)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("解析 docker compose ps 输出失败：%w", err)
	}
	for _, row := range rows {
		state := strings.ToLower(mapString(row, "State", "state"))
		status := strings.ToLower(mapString(row, "Status", "status"))
		if state == "" && status == "" {
			continue
		}
		total++
		if state == "running" || strings.HasPrefix(status, "up ") || status == "up" {
			running++
		} else {
			stopped++
		}
	}
	return total, running, stopped, nil
}

func decodeJSONObjectList(output []byte) ([]map[string]any, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &rows); err == nil {
		return rows, nil
	}
	var single map[string]any
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil {
		return []map[string]any{single}, nil
	}
	rows = nil
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func mapString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		value, exists := row[key]
		if !exists || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case []any:
			items := make([]string, 0, len(typed))
			for _, item := range typed {
				items = append(items, fmt.Sprint(item))
			}
			return strings.Join(items, ",")
		default:
			return fmt.Sprint(value)
		}
	}
	return ""
}

func analyzeComposeMigrationConfig(content string) composeMigrationSummary {
	var document map[string]any
	summary := composeMigrationSummary{}
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		summary.Failures = []string{"无法解析 Compose 最终配置：" + err.Error()}
		return summary
	}
	services := stringMap(document["services"])
	networks := stringMap(document["networks"])
	volumes := stringMap(document["volumes"])
	configs := stringMap(document["configs"])
	secrets := stringMap(document["secrets"])
	summary.Services = len(services)
	summary.Networks = len(networks)
	summary.Volumes = len(volumes)
	summary.Configs = len(configs)
	summary.Secrets = len(secrets)
	if summary.Services == 0 {
		summary.Failures = append(summary.Failures, "Compose 配置没有 Service")
	}

	imageSet := map[string]struct{}{}
	warningSet := map[string]struct{}{}
	failureSet := map[string]struct{}{}
	for serviceName, raw := range services {
		service := stringMap(raw)
		image := strings.TrimSpace(fmt.Sprint(service["image"]))
		if image == "<nil>" {
			image = ""
		}
		if image == "" {
			failureSet[fmt.Sprintf("Service %s 没有 image；Swarm 不会执行 Compose build", serviceName)] = struct{}{}
		} else {
			imageSet[image] = struct{}{}
		}
		if _, exists := service["build"]; exists {
			warningSet[fmt.Sprintf("Service %s 的 build 会被 Swarm 忽略；迁移前必须构建并推送 image", serviceName)] = struct{}{}
		}
		for field, explanation := range map[string]string{
			"container_name": "container_name 在 Swarm 中不生效",
			"depends_on":     "depends_on 不保证 Swarm Service 启动顺序",
			"restart":        "restart 应改为 deploy.restart_policy",
			"links":          "links 是旧式容器连接方式，Swarm 中应使用 Overlay 网络和服务名",
			"profiles":       "profiles 可能使部分 Service 未进入最终迁移配置",
		} {
			if _, exists := service[field]; exists {
				warningSet[fmt.Sprintf("Service %s：%s", serviceName, explanation)] = struct{}{}
			}
		}
		for _, mount := range anySlice(service["volumes"]) {
			switch value := mount.(type) {
			case string:
				source := strings.SplitN(value, ":", 2)[0]
				if strings.HasPrefix(source, "/") || strings.HasPrefix(source, ".") {
					warningSet[fmt.Sprintf("Service %s 使用绑定挂载 %s；迁移时将固定到当前节点", serviceName, source)] = struct{}{}
				}
			case map[string]any:
				if strings.EqualFold(fmt.Sprint(value["type"]), "bind") {
					source := fmt.Sprint(value["source"])
					warningSet[fmt.Sprintf("Service %s 使用绑定挂载 %s；迁移时将固定到当前节点", serviceName, source)] = struct{}{}
				}
			}
		}
	}
	if len(volumes) > 0 {
		warningSet["命名卷默认使用节点本地存储；相关 Service 迁移时将固定到当前节点"] = struct{}{}
	}
	for value := range imageSet {
		summary.Images = append(summary.Images, value)
	}
	for value := range warningSet {
		summary.Warnings = append(summary.Warnings, value)
	}
	for value := range failureSet {
		summary.Failures = append(summary.Failures, value)
	}
	sort.Strings(summary.Images)
	sort.Strings(summary.Warnings)
	sort.Strings(summary.Failures)
	return summary
}

func applyComposeBindPlacement(content, hostname string) composeBindPlacement {
	result := composeBindPlacement{Content: content}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		result.Failures = []string{"无法解析 Compose 最终配置：" + err.Error()}
		return result
	}
	services := stringMap(document["services"])
	volumes := stringMap(document["volumes"])
	for serviceName, raw := range services {
		service := stringMap(raw)
		if !composeServiceUsesLocalStorage(service, volumes) {
			continue
		}
		result.Services = append(result.Services, serviceName)
		deploy := stringMap(service["deploy"])
		service["deploy"] = deploy
		placement := stringMap(deploy["placement"])
		deploy["placement"] = placement
		constraints := stringSlice(placement["constraints"])
		required := "node.hostname == " + hostname
		hasRequired := false
		for _, constraint := range constraints {
			key, operator, value, matched := parsePlacementConstraint(constraint)
			if !matched || key != "node.hostname" {
				continue
			}
			switch operator {
			case "==":
				if value == hostname {
					hasRequired = true
				} else {
					result.Failures = append(result.Failures, fmt.Sprintf(
						"Service %s 已约束到其他节点 %s，与绑定挂载所在节点 %s 冲突",
						serviceName, value, hostname,
					))
				}
			case "!=":
				if value == hostname {
					result.Failures = append(result.Failures, fmt.Sprintf(
						"Service %s 明确排除了绑定挂载所在节点 %s",
						serviceName, hostname,
					))
				}
			}
		}
		if !hasRequired {
			constraints = append(constraints, required)
		}
		placement["constraints"] = constraints
	}
	sort.Strings(result.Services)
	result.Failures = uniqueSorted(result.Failures)
	if len(result.Services) == 0 || len(result.Failures) > 0 {
		return result
	}
	output, err := yaml.Marshal(document)
	if err != nil {
		result.Failures = append(result.Failures, "写入绑定挂载放置约束失败："+err.Error())
		return result
	}
	result.Content = string(output)
	return result
}

func normalizeComposeStackConfig(content string) (composeStackNormalization, error) {
	result := composeStackNormalization{Content: content}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return result, fmt.Errorf("解析 Compose 端口配置失败：%w", err)
	}
	if _, exists := document["name"]; exists {
		delete(document, "name")
		result.RemovedRootName = true
	}
	changedServices := map[string]struct{}{}
	for serviceName, raw := range stringMap(document["services"]) {
		service := stringMap(raw)
		for _, rawPort := range anySlice(service["ports"]) {
			port, ok := rawPort.(map[string]any)
			if !ok {
				continue
			}
			for _, field := range []string{"target", "published"} {
				text, isString := port[field].(string)
				if !isString || !regexp.MustCompile(`^[0-9]+$`).MatchString(text) {
					continue
				}
				value, err := strconv.Atoi(text)
				if err != nil {
					return result, fmt.Errorf("Service %s 的 %s 端口无效：%s", serviceName, field, text)
				}
				port[field] = value
				changedServices[serviceName] = struct{}{}
			}
		}
	}
	if len(changedServices) == 0 && !result.RemovedRootName {
		return result, nil
	}
	for service := range changedServices {
		result.Services = append(result.Services, service)
	}
	sort.Strings(result.Services)
	output, err := yaml.Marshal(document)
	if err != nil {
		return result, fmt.Errorf("写入 Swarm 端口配置失败：%w", err)
	}
	result.Content = string(output)
	return result, nil
}

func composeServiceUsesLocalStorage(service, volumes map[string]any) bool {
	for _, mount := range anySlice(service["volumes"]) {
		switch value := mount.(type) {
		case string:
			parts := strings.Split(value, ":")
			if len(parts) == 1 {
				return true
			}
			source := parts[0]
			if strings.HasPrefix(source, "/") || strings.HasPrefix(source, ".") ||
				composeVolumeUsesLocalDriver(source, volumes) {
				return true
			}
		case map[string]any:
			mountType := strings.ToLower(strings.TrimSpace(fmt.Sprint(value["type"])))
			if mountType == "bind" {
				return true
			}
			if mountType == "volume" {
				source := strings.TrimSpace(fmt.Sprint(value["source"]))
				if source == "" || source == "<nil>" || composeVolumeUsesLocalDriver(source, volumes) {
					return true
				}
			}
		}
	}
	return false
}

func composeVolumeUsesLocalDriver(name string, volumes map[string]any) bool {
	volume, exists := volumes[name]
	if !exists {
		return true
	}
	driver := strings.TrimSpace(fmt.Sprint(stringMap(volume)["driver"]))
	return driver == "" || driver == "<nil>" || strings.EqualFold(driver, "local")
}

func parsePlacementConstraint(value string) (key, operator, constraintValue string, matched bool) {
	parts := regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)\s*(==|!=)\s*(.*?)\s*$`).FindStringSubmatch(value)
	if len(parts) != 4 {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[3], true
}

func stringMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return map[string]any{}
}

func anySlice(value any) []any {
	if value == nil {
		return nil
	}
	if result, ok := value.([]any); ok {
		return result
	}
	return nil
}

func printMigrationChecks(
	output io.Writer,
	project string,
	stack string,
	directory string,
	summary composeMigrationSummary,
	checks []composeMigrationCheck,
	sourceHash string,
) {
	pairs := []cliui.Pair{
		{Label: "项目", Value: valueOrDash(project)},
		{Label: "目录", Value: valueOrDash(directory)},
		{Label: "目标 Stack", Value: valueOrDash(stack)},
		{Label: "资源", Value: fmt.Sprintf(
			"Services %d  ·  Networks %d  ·  Volumes %d  ·  Configs %d  ·  Secrets %d",
			summary.Services, summary.Networks, summary.Volumes, summary.Configs, summary.Secrets,
		)},
	}
	if len(summary.Images) > 0 {
		pairs = append(pairs, cliui.Pair{Label: "镜像", Value: strings.Join(summary.Images, "\n")})
	}
	printSwarmMigrationReport(output, swarmMigrationReport{
		Title:    "Compose → Swarm 迁移检查",
		Subtitle: "只读检查 · 不会停止容器或创建 Service",
		Summary:  pairs,
		Checks:   checks,
		Hash:     sourceHash,
	})
}

func humanBytes(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(size)/(1024*1024))
}
