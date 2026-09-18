package docker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"galaxy/pkg/cliui"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type localSwarmInfo struct {
	LocalNodeState   string `json:"LocalNodeState"`
	ControlAvailable bool   `json:"ControlAvailable"`
	NodeID           string `json:"NodeID"`
	NodeAddr         string `json:"NodeAddr"`
	Nodes            int    `json:"Nodes"`
	Managers         int    `json:"Managers"`
	Cluster          struct {
		ID string `json:"ID"`
	} `json:"Cluster"`
}

type composeMigrationCheck struct {
	Status string `json:"status"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

type swarmMigrationPlan struct {
	Stack      string
	Content    string
	WorkingDir string
}

type swarmMigrationSource struct {
	Stop    func() error
	Restore func() error
}

type swarmMigrationProgress func(string)

type swarmMigrationReport struct {
	Title    string
	Subtitle string
	Summary  []cliui.Pair
	Checks   []composeMigrationCheck
	Hash     string
}

type localSwarmContext struct {
	Info     *localSwarmInfo
	Hostname string
	Checks   []composeMigrationCheck
	Ready    bool
}

var errSwarmMigrationCancelled = errors.New("迁移已取消")

func confirmSwarmMigration(input io.Reader, output io.Writer, message string, assumeYes bool) error {
	ui := cliui.New(output)
	if assumeYes {
		ui.Info("已通过 --yes 确认迁移")
		return nil
	}
	if input == nil {
		return errors.New("无法读取迁移确认；请在交互终端输入 yes，或使用 --yes")
	}
	ui.Title("迁移确认", "下一步将修改本机 Docker 资源")
	ui.Summary([]cliui.Pair{{Label: "即将执行", Value: message}})
	ui.Warning("该操作会产生短暂停机。只有输入完整的 yes 才会继续，其他输入均取消。")
	fmt.Fprint(output, "请输入 yes 确认迁移: ")
	answer, err := readConfirmationLine(input)
	if err != nil && strings.TrimSpace(answer) == "" {
		return fmt.Errorf("读取迁移确认失败：%w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(answer), "yes") {
		ui.Info("未输入 yes，迁移已取消；未修改 Docker 资源")
		return errSwarmMigrationCancelled
	}
	ui.Success("确认通过，开始迁移")
	return nil
}

// readConfirmationLine deliberately avoids buffered read-ahead so consecutive
// confirmations can safely share stdin, including piped input in automation.
func readConfirmationLine(input io.Reader) (string, error) {
	var line strings.Builder
	buffer := []byte{0}
	for {
		count, err := input.Read(buffer)
		if count > 0 {
			if buffer[0] == '\n' {
				return line.String(), nil
			}
			line.WriteByte(buffer[0])
		}
		if err != nil {
			return line.String(), err
		}
	}
}

func inspectLocalSwarmContext() localSwarmContext {
	context := localSwarmContext{}
	info, err := localDockerSwarmInfo()
	if err != nil {
		context.Checks = append(context.Checks, composeMigrationCheck{
			Status: "失败", Name: "Swarm Manager", Detail: err.Error(),
		})
		return context
	}
	context.Info = info
	context.Checks = append(context.Checks, composeMigrationCheck{
		Status: "通过",
		Name:   "Swarm Manager",
		Detail: fmt.Sprintf(
			"Swarm %s，节点 %d，Manager %d，本机 %s (%s)",
			info.Cluster.ID, info.Nodes, info.Managers, info.NodeID, info.NodeAddr,
		),
	})
	hostname, err := localDockerNodeHostname()
	if err != nil {
		context.Checks = append(context.Checks, composeMigrationCheck{
			Status: "失败", Name: "节点身份", Detail: err.Error(),
		})
		return context
	}
	context.Hostname = hostname
	context.Ready = true
	return context
}

func inspectSwarmPlan(plan swarmMigrationPlan, managerReady bool) []composeMigrationCheck {
	checks := make([]composeMigrationCheck, 0, 2)
	if strings.TrimSpace(plan.Content) != "" {
		if err := plan.validate(); err != nil {
			checks = append(checks, composeMigrationCheck{
				Status: "失败", Name: "Stack 配置校验", Detail: err.Error(),
			})
		} else {
			checks = append(checks, composeMigrationCheck{
				Status: "通过", Name: "Stack 配置校验", Detail: "docker stack config 校验通过",
			})
		}
	}
	if !managerReady {
		return checks
	}
	existing, err := plan.existingServices()
	switch {
	case err != nil:
		checks = append(checks, composeMigrationCheck{
			Status: "失败", Name: "目标 Stack 冲突", Detail: err.Error(),
		})
	case len(existing) > 0:
		checks = append(checks, composeMigrationCheck{
			Status: "失败", Name: "目标 Stack 冲突",
			Detail: "已存在同名 Stack Service：" + strings.Join(existing, ", "),
		})
	default:
		checks = append(checks, composeMigrationCheck{
			Status: "通过", Name: "目标 Stack 冲突", Detail: "未发现同名 Stack",
		})
	}
	return checks
}

func (p swarmMigrationPlan) validate() error {
	command := exec.Command("docker", "stack", "config", "--compose-file", "-")
	command.Dir = p.WorkingDir
	command.Stdin = strings.NewReader(p.Content)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行 docker stack config 失败：%s", commandFailure(output, err))
	}
	return nil
}

func (p swarmMigrationPlan) existingServices() ([]string, error) {
	output, err := exec.Command(
		"docker", "service", "ls",
		"--filter", "label=com.docker.stack.namespace="+p.Stack,
		"--format", "{{.Name}}",
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("查询目标 Stack 失败：%s", commandFailure(output, err))
	}
	names := strings.Fields(string(output))
	sort.Strings(names)
	return names, nil
}

func (p swarmMigrationPlan) deploy() ([]byte, error) {
	command := exec.Command("docker", "stack", "deploy", "--compose-file", "-", p.Stack)
	command.Dir = p.WorkingDir
	command.Stdin = strings.NewReader(p.Content)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("执行 docker stack deploy 失败：%s", commandFailure(output, err))
	}
	return output, nil
}

func executeSwarmMigration(
	plan swarmMigrationPlan,
	source swarmMigrationSource,
	waitTimeout time.Duration,
	progress swarmMigrationProgress,
) ([]byte, error) {
	if source.Stop != nil {
		reportMigrationProgress(progress, "正在停止并保留旧工作负载，以便失败时自动恢复")
		if err := source.Stop(); err != nil {
			return nil, err
		}
		reportMigrationProgress(progress, "旧工作负载已停止并保留")
	}
	reportMigrationProgress(progress, "正在提交 Swarm Stack "+plan.Stack)
	output, err := plan.deploy()
	if err == nil && waitTimeout > 0 {
		reportMigrationProgress(progress, fmt.Sprintf("Stack 已提交，等待 Service 就绪（最长 %s）", waitTimeout))
		err = waitForSwarmStack(plan.Stack, waitTimeout, progress)
	}
	if err == nil {
		if waitTimeout == 0 {
			reportMigrationProgress(progress, "Stack 已提交，已按 --wait=0 跳过就绪等待")
		} else {
			reportMigrationProgress(progress, "Stack 全部 Service 已就绪")
		}
		return output, nil
	}
	reportMigrationProgress(progress, "迁移未完成，正在清理目标 Stack 并恢复旧工作负载")
	return nil, rollbackSwarmMigration(plan, source, err)
}

func reportMigrationProgress(progress swarmMigrationProgress, message string) {
	if progress != nil {
		progress(message)
	}
}

func rollbackSwarmMigration(
	plan swarmMigrationPlan,
	source swarmMigrationSource,
	migrationErr error,
) error {
	cleanupErr := removeSwarmStackIfExists(plan.Stack)
	var restoreErr error
	if source.Restore != nil {
		restoreErr = source.Restore()
	}
	switch {
	case cleanupErr != nil && restoreErr != nil:
		return fmt.Errorf("迁移失败：%v；清理目标 Stack 失败：%v；恢复源工作负载失败：%v",
			migrationErr, cleanupErr, restoreErr)
	case cleanupErr != nil:
		return fmt.Errorf("迁移失败：%v；清理目标 Stack 失败：%v", migrationErr, cleanupErr)
	case restoreErr != nil:
		return fmt.Errorf("迁移失败：%v；恢复源工作负载失败：%v", migrationErr, restoreErr)
	case source.Restore != nil:
		return fmt.Errorf("迁移失败，源工作负载已自动恢复：%w", migrationErr)
	default:
		return migrationErr
	}
}

func waitForSwarmStack(stack string, timeout time.Duration, progress swarmMigrationProgress) error {
	deadline := time.Now().Add(timeout)
	lastStatus := "尚未读取到 Service 副本状态"
	lastReportedStatus := ""
	lastReportedAt := time.Time{}
	for {
		output, err := exec.Command(
			"docker", "stack", "services", stack,
			"--format", "{{.Name}}\t{{.Replicas}}",
		).CombinedOutput()
		if err != nil {
			return fmt.Errorf("读取迁移后 Stack 状态失败：%s", commandFailure(output, err))
		}
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		ready := len(lines) > 0 && strings.TrimSpace(lines[0]) != ""
		statuses := make([]string, 0, len(lines))
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				ready = false
				continue
			}
			statuses = append(statuses, fields[0]+" "+fields[1])
			current, desired, valid := strings.Cut(fields[1], "/")
			if !valid || current != desired {
				ready = false
			}
		}
		if len(statuses) > 0 {
			lastStatus = strings.Join(statuses, ", ")
			if lastStatus != lastReportedStatus || time.Since(lastReportedAt) >= 10*time.Second {
				reportMigrationProgress(progress, "Service 状态："+lastStatus)
				lastReportedStatus = lastStatus
				lastReportedAt = time.Now()
			}
		}
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			detailOutput, detailErr := exec.Command(
				"docker", "stack", "ps", "--no-trunc", stack,
				"--format", "{{.Name}} {{.CurrentState}}: {{.Error}}",
			).CombinedOutput()
			detail := sanitizeCell(string(detailOutput))
			if detailErr != nil || detail == "" {
				detail = lastStatus
			}
			return fmt.Errorf("Stack %s 在 %s 内未全部就绪：%s", stack, timeout, detail)
		}
		time.Sleep(time.Second)
	}
}

func removeSwarmStackIfExists(stack string) error {
	services, err := (swarmMigrationPlan{Stack: stack}).existingServices()
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return nil
	}
	output, err := exec.Command("docker", "stack", "rm", stack).CombinedOutput()
	if err != nil {
		return fmt.Errorf("删除目标 Stack 失败：%s", commandFailure(output, err))
	}
	waitForStackRemoval(stack, 15*time.Second)
	return nil
}

func waitForStackRemoval(stack string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		services, err := (swarmMigrationPlan{Stack: stack}).existingServices()
		if err != nil || len(services) == 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func localDockerSwarmInfo() (*localSwarmInfo, error) {
	output, err := exec.Command("docker", "info", "--format", "{{json .Swarm}}").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("读取本机 Swarm 信息失败：%s", commandFailure(output, err))
	}
	var info localSwarmInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("解析本机 Swarm 信息失败：%w", err)
	}
	if info.LocalNodeState != "active" || !info.ControlAvailable || info.Cluster.ID == "" {
		return nil, errors.New("当前主机不是活动的 Docker Swarm Manager")
	}
	return &info, nil
}

func localDockerNodeHostname() (string, error) {
	output, err := exec.Command("docker", "info", "--format", "{{.Name}}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("读取本机 Docker 节点名失败：%s", commandFailure(output, err))
	}
	hostname := strings.TrimSpace(string(output))
	if hostname == "" {
		return "", errors.New("本机 Docker 节点名为空")
	}
	return hostname, nil
}

func printSwarmMigrationReport(output io.Writer, report swarmMigrationReport) {
	ui := cliui.New(output)
	ui.Title(report.Title, report.Subtitle)
	ui.Summary(report.Summary)
	ui.Checks(toUIChecks(report.Checks))
	failures, warnings := 0, 0
	for _, check := range report.Checks {
		switch check.Status {
		case "失败":
			failures++
		case "警告":
			warnings++
		}
	}
	if failures > 0 {
		ui.Decision(false, fmt.Sprintf("不可迁移  ·  %d 项失败  ·  %d 项警告", failures, warnings))
	} else {
		ui.Decision(true, fmt.Sprintf("可以迁移  ·  %d 项警告，请迁移前确认", warnings))
	}
	if report.Hash != "" {
		ui.Hint("配置指纹", report.Hash)
	}
}

func toUIChecks(checks []composeMigrationCheck) []cliui.Check {
	result := make([]cliui.Check, 0, len(checks))
	for _, check := range checks {
		status := cliui.StatusInfo
		switch check.Status {
		case "通过":
			status = cliui.StatusPass
		case "失败":
			status = cliui.StatusFail
		case "警告":
			status = cliui.StatusWarn
		}
		result = append(result, cliui.Check{Status: status, Name: check.Name, Detail: check.Detail})
	}
	return result
}

func hashText(value string) string {
	if value == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func uniqueSorted(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	values = values[:0]
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func sanitizeCell(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}

func valueOrDash(value string) string {
	value = sanitizeCell(value)
	if value == "" {
		return "-"
	}
	return value
}

func commandFailure(output []byte, err error) string {
	message := sanitizeCell(string(output))
	if message == "" {
		message = err.Error()
	}
	lowerMessage := strings.ToLower(message)
	if strings.Contains(lowerMessage, "permission denied") &&
		(strings.Contains(lowerMessage, "docker.sock") || strings.Contains(lowerMessage, "docker daemon")) {
		message += "；请使用 sudo 运行该命令，或授予当前用户 Docker Socket 访问权限"
	}
	return message
}

func commandOutput(command *exec.Cmd) (stdout, diagnostic []byte, err error) {
	var output bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &output
	command.Stderr = &stderr
	err = command.Run()
	if err != nil {
		diagnostic = stderr.Bytes()
		if len(bytes.TrimSpace(diagnostic)) == 0 {
			diagnostic = output.Bytes()
		}
	}
	return output.Bytes(), diagnostic, err
}
