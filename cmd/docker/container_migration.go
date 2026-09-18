package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"galaxy/pkg/cliui"
	"galaxy/pkg/galaxycfg"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type containerMigrationOptions struct {
	galaxycfg.IOStreams

	container    string
	stack        string
	service      string
	expectedHash string
	assumeYes    bool
	waitTimeout  time.Duration
}

type containerInspect struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
	} `json:"State"`
	Config struct {
		Image       string            `json:"Image"`
		Env         []string          `json:"Env"`
		Cmd         []string          `json:"Cmd"`
		Entrypoint  []string          `json:"Entrypoint"`
		Labels      map[string]string `json:"Labels"`
		User        string            `json:"User"`
		WorkingDir  string            `json:"WorkingDir"`
		Hostname    string            `json:"Hostname"`
		Tty         bool              `json:"Tty"`
		OpenStdin   bool              `json:"OpenStdin"`
		Healthcheck *struct {
			Test        []string `json:"Test"`
			Interval    int64    `json:"Interval"`
			Timeout     int64    `json:"Timeout"`
			StartPeriod int64    `json:"StartPeriod"`
			Retries     int      `json:"Retries"`
		} `json:"Healthcheck"`
	} `json:"Config"`
	HostConfig struct {
		NetworkMode    string                     `json:"NetworkMode"`
		Privileged     bool                       `json:"Privileged"`
		AutoRemove     bool                       `json:"AutoRemove"`
		ReadonlyRootfs bool                       `json:"ReadonlyRootfs"`
		CapAdd         []string                   `json:"CapAdd"`
		CapDrop        []string                   `json:"CapDrop"`
		Devices        []map[string]any           `json:"Devices"`
		DeviceRequests []map[string]any           `json:"DeviceRequests"`
		Links          []string                   `json:"Links"`
		PidMode        string                     `json:"PidMode"`
		IpcMode        string                     `json:"IpcMode"`
		SecurityOpt    []string                   `json:"SecurityOpt"`
		Sysctls        map[string]string          `json:"Sysctls"`
		ExtraHosts     []string                   `json:"ExtraHosts"`
		DNS            []string                   `json:"Dns"`
		PortBindings   map[string][]containerPort `json:"PortBindings"`
		RestartPolicy  containerRestartPolicy     `json:"RestartPolicy"`
		LogConfig      struct {
			Type   string            `json:"Type"`
			Config map[string]string `json:"Config"`
		} `json:"LogConfig"`
	} `json:"HostConfig"`
	Mounts          []containerMount `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
}

type containerPort struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type containerRestartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

type containerMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Driver      string `json:"Driver"`
	RW          bool   `json:"RW"`
}

type standaloneContainer struct {
	ID     string
	Name   string
	Image  string
	Status string
}

type generatedContainerStack struct {
	Content  string
	Warnings []string
	Failures []string
}

func newCmdContainerMigration(ioStreams galaxycfg.IOStreams) *cobra.Command {
	containerCmd := &cobra.Command{
		Use:   "container",
		Short: "管理本机独立 Docker 容器",
		Long:  "纯本地工具：直接访问当前主机的 Docker Engine，不调用 Galaxy API。",
	}
	containerCmd.AddCommand(newCmdStandaloneContainerList(ioStreams))

	o := &containerMigrationOptions{IOStreams: ioStreams}
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "检查并将独立容器迁移为 Swarm Service",
		Long:  "检查并迁移 docker run 等方式启动的独立容器。检查通过并确认后直接迁移，成功后原容器保持停止状态，以便回滚。",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return o.runConvert(false)
		},
	}
	flags := migrateCmd.PersistentFlags()
	flags.StringVar(&o.container, "container", "", "源容器名称或 ID")
	flags.StringVar(&o.stack, "stack", "", "目标 Swarm Stack 名称，默认由容器名称生成")
	flags.StringVar(&o.service, "service", "", "目标 Swarm Service 名称，默认由容器名称生成")
	_ = migrateCmd.MarkPersistentFlagRequired("container")
	executeFlags := migrateCmd.Flags()
	executeFlags.StringVar(&o.expectedHash, "source-hash", "", "可选：要求最终配置匹配指定的 64 位指纹")
	executeFlags.BoolVarP(&o.assumeYes, "yes", "y", false, "检查通过后跳过 yes 确认")
	executeFlags.DurationVar(&o.waitTimeout, "wait", time.Minute, "等待 Service 达到运行状态的超时；设为 0 跳过等待")

	containerCmd.AddCommand(migrateCmd)
	return containerCmd
}

func newCmdStandaloneContainerList(ioStreams galaxycfg.IOStreams) *cobra.Command {
	format := "table"
	command := &cobra.Command{
		Use:   "list",
		Short: "列出当前节点上不属于 Compose 或 Swarm 的独立容器",
		RunE: func(command *cobra.Command, _ []string) error {
			output, err := exec.CommandContext(
				command.Context(), "docker", "container", "ls", "--all", "--format", "{{json .}}",
			).CombinedOutput()
			if err != nil {
				return fmt.Errorf("读取本机容器失败：%s", commandFailure(output, err))
			}
			rows, err := parseStandaloneContainers(output)
			if err != nil {
				return err
			}
			if format == "json" {
				encoder := json.NewEncoder(ioStreams.Out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(rows)
			}
			printStandaloneContainers(ioStreams.Out, rows)
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

func parseStandaloneContainers(output []byte) ([]standaloneContainer, error) {
	rows, err := decodeJSONObjectList(output)
	if err != nil {
		return nil, fmt.Errorf("解析 docker container ls 输出失败：%w", err)
	}
	result := make([]standaloneContainer, 0, len(rows))
	for _, row := range rows {
		labels := parseDockerLabelString(mapString(row, "Labels", "labels"))
		if labels["com.docker.compose.project"] != "" || labels["com.docker.swarm.service.name"] != "" {
			continue
		}
		result = append(result, standaloneContainer{
			ID:     sanitizeCell(mapString(row, "ID", "Id", "id")),
			Name:   sanitizeCell(mapString(row, "Names", "Name", "name")),
			Image:  sanitizeCell(mapString(row, "Image", "image")),
			Status: sanitizeCell(mapString(row, "Status", "status")),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func parseDockerLabelString(value string) map[string]string {
	labels := map[string]string{}
	for _, item := range strings.Split(value, ",") {
		key, val, found := strings.Cut(strings.TrimSpace(item), "=")
		if found {
			labels[key] = val
		}
	}
	return labels
}

func printStandaloneContainers(output io.Writer, rows []standaloneContainer) {
	ui := cliui.New(output)
	ui.Title("独立容器", fmt.Sprintf("当前节点 · 共 %d 个容器", len(rows)))
	if len(rows) == 0 {
		ui.Empty("当前 Docker 节点没有独立容器")
		return
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			row.ID, row.Name, row.Image, ui.State(row.Status),
		})
	}
	ui.Table([]cliui.Column{
		{Header: "CONTAINER ID", WidthMax: 20},
		{Header: "NAME", WidthMax: 32},
		{Header: "IMAGE", WidthMax: 60},
		{Header: "STATUS", WidthMax: 32},
	}, tableRows)
}

func (o *containerMigrationOptions) runCheck(printNext bool) (*containerInspect, generatedContainerStack, error) {
	checks := make([]composeMigrationCheck, 0, 16)
	add := func(status, name, detail string) {
		checks = append(checks, composeMigrationCheck{Status: status, Name: name, Detail: sanitizeCell(detail)})
	}
	if strings.TrimSpace(o.container) == "" {
		add("失败", "源容器", "必须指定 --container")
		printContainerMigrationChecks(o.Out, o.container, o.stack, o.service, checks, "", generatedContainerStack{})
		return nil, generatedContainerStack{}, errors.New("独立容器当前不可迁移")
	}

	inspect, err := inspectContainer(o.container)
	if err != nil {
		add("失败", "源容器", err.Error())
		printContainerMigrationChecks(o.Out, o.container, o.stack, o.service, checks, "", generatedContainerStack{})
		return nil, generatedContainerStack{}, errors.New("独立容器当前不可迁移")
	}
	o.resolveNames(inspect)
	if err := validateSwarmName(o.stack, "Stack"); err != nil {
		add("失败", "目标 Stack", err.Error())
	}
	if err := validateSwarmName(o.service, "Service"); err != nil {
		add("失败", "目标 Service", err.Error())
	}
	add("通过", "源容器", fmt.Sprintf("%s (%s)，状态 %s", strings.TrimPrefix(inspect.Name, "/"), shortID(inspect.ID), inspect.State.Status))
	if inspect.Config.Labels["com.docker.compose.project"] != "" {
		add("失败", "容器归属", "该容器由 Docker Compose 管理，请使用 galaxy docker compose migrate")
	} else if inspect.Config.Labels["com.docker.swarm.service.name"] != "" {
		add("失败", "容器归属", "该容器已经是 Swarm Service Task")
	} else {
		add("通过", "容器归属", "独立容器")
	}
	if !inspect.State.Running {
		add("警告", "运行状态", "源容器当前未运行；迁移后 Service 仍会启动")
	}

	swarmContext := inspectLocalSwarmContext()
	for _, check := range swarmContext.Checks {
		add(check.Status, check.Name, check.Detail)
	}
	if swarmContext.Ready {
		add("通过", "节点约束", "Service 固定到原节点 "+swarmContext.Hostname+"，以保留本地端口和存储语义")
	}

	generated := generateContainerStack(inspect, o.service, swarmContext.Hostname)
	for _, failure := range generated.Failures {
		add("失败", "Swarm 兼容性", failure)
	}
	for _, warning := range generated.Warnings {
		add("警告", "迁移风险", warning)
	}
	if generated.Content != "" {
		plan := swarmMigrationPlan{Stack: o.stack, Content: generated.Content}
		for _, check := range inspectSwarmPlan(plan, swarmContext.Ready) {
			add(check.Status, check.Name, check.Detail)
		}
	}

	sourceHash := hashText(generated.Content)
	printContainerMigrationChecks(o.Out, strings.TrimPrefix(inspect.Name, "/"), o.stack, o.service, checks, sourceHash, generated)
	for _, check := range checks {
		if check.Status == "失败" {
			return inspect, generated, errors.New("独立容器当前不可迁移")
		}
	}
	if printNext {
		next := fmt.Sprintf(
			"galaxy docker container migrate --container %s --stack %s --service %s",
			shellQuote(o.container), shellQuote(o.stack), shellQuote(o.service),
		)
		cliui.New(o.Out).Hint("下一步", next)
	}
	return inspect, generated, nil
}

func (o *containerMigrationOptions) runConvert(oneClick bool) error {
	inspect, generated, err := o.runCheck(false)
	if err != nil {
		return err
	}
	actualHash := hashText(generated.Content)
	if !oneClick && o.expectedHash != "" {
		if len(o.expectedHash) != 64 {
			return errors.New("--source-hash 必须是 64 位配置指纹")
		}
		if !strings.EqualFold(actualHash, o.expectedHash) {
			return errors.New("容器配置与 --source-hash 不匹配，操作已取消")
		}
	}
	if err := confirmSwarmMigration(
		o.In,
		o.Out,
		fmt.Sprintf(
			"将停止独立容器 %s，并创建 Swarm Service %s_%s；原容器会保留用于回滚。",
			strings.TrimPrefix(inspect.Name, "/"), o.stack, o.service,
		),
		o.assumeYes,
	); err != nil {
		if errors.Is(err, errSwarmMigrationCancelled) {
			return nil
		}
		return err
	}
	wasRunning := inspect.State.Running
	source := swarmMigrationSource{}
	if wasRunning {
		source.Stop = func() error {
			output, stopErr := exec.Command("docker", "container", "stop", inspect.ID).CombinedOutput()
			if stopErr != nil {
				return fmt.Errorf("停止源容器失败：%s", commandFailure(output, stopErr))
			}
			return nil
		}
		source.Restore = func() error {
			output, restoreErr := exec.Command("docker", "container", "start", inspect.ID).CombinedOutput()
			if restoreErr != nil {
				return fmt.Errorf("启动原容器失败：%s", commandFailure(output, restoreErr))
			}
			return nil
		}
	}
	plan := swarmMigrationPlan{Stack: o.stack, Content: generated.Content}
	ui := cliui.New(o.Out)
	output, deployErr := executeSwarmMigration(plan, source, o.waitTimeout, ui.Info)
	if deployErr != nil {
		return deployErr
	}
	serviceFullName := o.stack + "_" + o.service
	ui.Success("迁移已提交：" + sanitizeCell(string(output)))
	if o.waitTimeout > 0 {
		ui.Success("Swarm Service 已运行：" + serviceFullName)
	} else {
		ui.Warning("Swarm Service 已创建（未等待运行状态）：" + serviceFullName)
		ui.Info(fmt.Sprintf(
			"原容器 %s 已停止并保留；跳过就绪等待时不会询问删除",
			strings.TrimPrefix(inspect.Name, "/"),
		))
		return nil
	}
	containerName := strings.TrimPrefix(inspect.Name, "/")
	cleanup, cleanupErr := confirmStandaloneContainerCleanup(
		o.In, o.Out, containerName, inspect.ID,
	)
	if cleanupErr != nil {
		ui.Warning("迁移已成功，但无法读取旧容器清理确认：" + cleanupErr.Error())
		ui.Hint("手动检查", "docker inspect "+shellQuote(inspect.ID))
		return nil
	}
	if !cleanup {
		ui.Info("原容器 " + containerName + " 已停止并保留")
		return nil
	}
	ui.Info("正在删除旧容器 " + containerName + " (" + shortID(inspect.ID) + ")")
	if err := runDockerContainerAction("rm", []string{inspect.ID}); err != nil {
		ui.Warning("Swarm Service 已运行，但旧容器删除失败：" + err.Error())
		ui.Hint("手动检查", "docker inspect "+shellQuote(inspect.ID))
		return nil
	}
	ui.Success("旧容器已删除；关联 Volume、绑定挂载路径和数据均已保留")
	return nil
}

func confirmStandaloneContainerCleanup(
	input io.Reader,
	output io.Writer,
	containerName string,
	containerID string,
) (bool, error) {
	ui := cliui.New(output)
	ui.Title("迁移后清理", "Swarm Service 已就绪，旧容器不再提供服务")
	ui.Summary([]cliui.Pair{
		{Label: "旧容器", Value: containerName},
		{Label: "容器 ID", Value: shortID(containerID)},
		{Label: "当前状态", Value: "已停止"},
	})
	ui.Warning("本次只删除上述旧容器；关联 Volume、绑定挂载路径和数据不会删除。")
	if input == nil {
		return false, errors.New("标准输入不可用")
	}
	fmt.Fprint(output, "是否删除旧容器？请输入 yes 确认，其他输入保留: ")
	answer, err := readConfirmationLine(input)
	if err != nil && strings.TrimSpace(answer) == "" {
		return false, err
	}
	if !strings.EqualFold(strings.TrimSpace(answer), "yes") {
		ui.Info("未确认清理，旧容器已保留")
		return false, nil
	}
	ui.Success("已确认删除旧容器")
	return true, nil
}

func inspectContainer(name string) (*containerInspect, error) {
	output, err := exec.Command("docker", "inspect", "--type", "container", name).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("读取容器失败：%s", commandFailure(output, err))
	}
	var rows []containerInspect
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, fmt.Errorf("解析容器信息失败：%w", err)
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("期望一个容器，实际得到 %d 个", len(rows))
	}
	return &rows[0], nil
}

func (o *containerMigrationOptions) resolveNames(inspect *containerInspect) {
	base := swarmSafeName(strings.TrimPrefix(inspect.Name, "/"))
	if o.stack == "" {
		o.stack = base
	}
	if o.service == "" {
		o.service = base
	}
}

func swarmSafeName(value string) string {
	value = strings.ToLower(value)
	value = regexp.MustCompile(`[^a-z0-9_.-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "_.-")
	if value == "" {
		return "migrated-container"
	}
	if len(value) > 63 {
		value = value[:63]
	}
	return value
}

func validateSwarmName(value, kind string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`).MatchString(value) {
		return fmt.Errorf("%s 名称只能包含字母、数字、下划线、点号和连字符，最长 63 字符", kind)
	}
	return nil
}

func generateContainerStack(inspect *containerInspect, serviceName, hostname string) generatedContainerStack {
	result := generatedContainerStack{}
	if strings.TrimSpace(inspect.Config.Image) == "" {
		result.Failures = append(result.Failures, "容器没有可复用的镜像名称")
		return result
	}
	if inspect.HostConfig.Privileged {
		result.Failures = append(result.Failures, "privileged 容器不能等价转换为 Swarm Service")
	}
	if inspect.HostConfig.AutoRemove {
		result.Failures = append(result.Failures, "容器启用了 --rm；停止时会被自动删除，无法安全回滚")
	}
	if len(inspect.HostConfig.Devices) > 0 {
		result.Failures = append(result.Failures, "容器使用了 --device；请迁移后手工设计 Swarm 设备约束")
	}
	if len(inspect.HostConfig.DeviceRequests) > 0 {
		result.Failures = append(result.Failures, "容器使用了 GPU/DeviceRequest；请手工设计 Swarm Generic Resource 约束")
	}
	if len(inspect.HostConfig.Links) > 0 {
		result.Failures = append(result.Failures, "容器使用了旧式 --link；请先改为 Overlay 网络和服务发现")
	}
	if inspect.HostConfig.NetworkMode == "host" || inspect.HostConfig.NetworkMode == "none" ||
		strings.HasPrefix(inspect.HostConfig.NetworkMode, "container:") {
		result.Failures = append(result.Failures, "容器使用不兼容的网络模式 "+inspect.HostConfig.NetworkMode)
	}
	if inspect.HostConfig.PidMode != "" && inspect.HostConfig.PidMode != "private" {
		result.Failures = append(result.Failures, "容器使用不兼容的 PID 模式 "+inspect.HostConfig.PidMode)
	}
	if inspect.HostConfig.IpcMode != "" && inspect.HostConfig.IpcMode != "private" {
		result.Failures = append(result.Failures, "容器使用不兼容的 IPC 模式 "+inspect.HostConfig.IpcMode)
	}
	if len(inspect.HostConfig.SecurityOpt) > 0 {
		result.Warnings = append(result.Warnings, "容器的 SecurityOpt 不会自动迁移，请检查 AppArmor、Seccomp 和 no-new-privileges 设置")
	}
	if len(inspect.HostConfig.Sysctls) > 0 {
		result.Warnings = append(result.Warnings, "容器的 Sysctl 不会自动迁移，请确认 Service 是否仍需要这些内核参数")
	}
	if inspect.Config.Tty || inspect.Config.OpenStdin {
		result.Warnings = append(result.Warnings, "容器使用交互式终端或标准输入；Swarm Service 不保留交互会话")
	}
	if len(inspect.NetworkSettings.Networks) > 1 ||
		(len(inspect.NetworkSettings.Networks) == 1 && inspect.HostConfig.NetworkMode != "default" && inspect.HostConfig.NetworkMode != "bridge") {
		result.Warnings = append(result.Warnings, "原容器网络将替换为 Stack Overlay 网络；请检查依赖容器的服务发现")
	}
	if !strings.Contains(inspect.Config.Image, "@sha256:") {
		result.Warnings = append(result.Warnings, "镜像未固定 Digest；请确保该镜像可由目标节点拉取且标签未漂移")
	}

	service := map[string]any{
		"image": inspect.Config.Image,
		"deploy": map[string]any{
			"replicas": 1,
		},
	}
	deploy := service["deploy"].(map[string]any)
	if hostname != "" {
		deploy["placement"] = map[string]any{"constraints": []string{"node.hostname == " + hostname}}
	}
	if len(inspect.Config.Cmd) > 0 {
		service["command"] = inspect.Config.Cmd
	}
	if len(inspect.Config.Entrypoint) > 0 {
		service["entrypoint"] = inspect.Config.Entrypoint
	}
	if len(inspect.Config.Env) > 0 {
		service["environment"] = inspect.Config.Env
		result.Warnings = append(result.Warnings, "环境变量会写入 Stack 配置；其中的密码和 Token 应改用 Docker Secret")
	}
	if inspect.Config.User != "" {
		service["user"] = inspect.Config.User
	}
	if inspect.Config.WorkingDir != "" {
		service["working_dir"] = inspect.Config.WorkingDir
	}
	if inspect.Config.Hostname != "" && inspect.Config.Hostname != strings.TrimPrefix(inspect.Name, "/") {
		service["hostname"] = inspect.Config.Hostname
	}
	if inspect.HostConfig.ReadonlyRootfs {
		service["read_only"] = true
	}
	if len(inspect.HostConfig.CapAdd) > 0 {
		service["cap_add"] = inspect.HostConfig.CapAdd
	}
	if len(inspect.HostConfig.CapDrop) > 0 {
		service["cap_drop"] = inspect.HostConfig.CapDrop
	}
	if len(inspect.HostConfig.ExtraHosts) > 0 {
		service["extra_hosts"] = inspect.HostConfig.ExtraHosts
	}
	if len(inspect.HostConfig.DNS) > 0 {
		service["dns"] = inspect.HostConfig.DNS
	}
	addContainerHealthcheck(service, inspect)
	addContainerRestartPolicy(deploy, inspect.HostConfig.RestartPolicy)
	addContainerLabels(service, inspect.Config.Labels)
	if inspect.HostConfig.LogConfig.Type != "" && inspect.HostConfig.LogConfig.Type != "json-file" {
		logging := map[string]any{"driver": inspect.HostConfig.LogConfig.Type}
		if len(inspect.HostConfig.LogConfig.Config) > 0 {
			logging["options"] = inspect.HostConfig.LogConfig.Config
		}
		service["logging"] = logging
	}

	ports := make([]map[string]any, 0)
	portKeys := make([]string, 0, len(inspect.HostConfig.PortBindings))
	for key := range inspect.HostConfig.PortBindings {
		portKeys = append(portKeys, key)
	}
	sort.Strings(portKeys)
	for _, key := range portKeys {
		targetText, protocol, found := strings.Cut(key, "/")
		if !found {
			protocol = "tcp"
		}
		target, err := strconv.Atoi(targetText)
		if err != nil {
			result.Failures = append(result.Failures, "无法解析容器端口 "+key)
			continue
		}
		for _, binding := range inspect.HostConfig.PortBindings[key] {
			if binding.HostPort == "" {
				result.Failures = append(result.Failures, "端口 "+key+" 使用随机宿主机端口，无法保证迁移后端口不变")
				continue
			}
			published, err := strconv.Atoi(binding.HostPort)
			if err != nil {
				result.Failures = append(result.Failures, "无法解析宿主机端口 "+binding.HostPort)
				continue
			}
			ports = append(ports, map[string]any{
				"target": target, "published": published, "protocol": protocol, "mode": "host",
			})
			if binding.HostIP != "" && binding.HostIP != "0.0.0.0" && binding.HostIP != "::" {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("端口 %s 原先只绑定 %s；Swarm host 发布模式会监听节点全部地址", binding.HostPort, binding.HostIP))
			}
		}
	}
	if len(ports) > 0 {
		service["ports"] = ports
	}

	volumes := make([]map[string]any, 0, len(inspect.Mounts))
	topVolumes := map[string]any{}
	for _, mount := range inspect.Mounts {
		item := map[string]any{
			"type": mount.Type, "source": mount.Source, "target": mount.Destination, "read_only": !mount.RW,
		}
		switch mount.Type {
		case "bind":
			result.Warnings = append(result.Warnings, "绑定挂载 "+mount.Source+" 依赖原节点本地路径")
		case "volume":
			name := mount.Name
			if name == "" {
				name = mount.Source
			}
			item["source"] = name
			topVolumes[name] = map[string]any{"external": true}
			result.Warnings = append(result.Warnings, "Docker Volume "+name+" 仅在原节点存在，Service 已固定到该节点")
		case "tmpfs":
			delete(item, "source")
		default:
			result.Failures = append(result.Failures, "不支持的挂载类型 "+mount.Type)
			continue
		}
		volumes = append(volumes, item)
	}
	if len(volumes) > 0 {
		service["volumes"] = volumes
	}

	document := map[string]any{"services": map[string]any{serviceName: service}}
	if len(topVolumes) > 0 {
		document["volumes"] = topVolumes
	}
	content, err := yaml.Marshal(document)
	if err != nil {
		result.Failures = append(result.Failures, "生成 Stack 配置失败："+err.Error())
		return result
	}
	result.Content = string(content)
	result.Warnings = uniqueSorted(result.Warnings)
	result.Failures = uniqueSorted(result.Failures)
	return result
}

func addContainerLabels(service map[string]any, labels map[string]string) {
	filtered := map[string]string{}
	for key, value := range labels {
		if strings.HasPrefix(key, "com.docker.compose.") || strings.HasPrefix(key, "com.docker.swarm.") {
			continue
		}
		filtered[key] = value
	}
	if len(filtered) > 0 {
		service["labels"] = filtered
	}
}

func addContainerHealthcheck(service map[string]any, inspect *containerInspect) {
	health := inspect.Config.Healthcheck
	if health == nil || len(health.Test) == 0 {
		return
	}
	if len(health.Test) == 1 && strings.EqualFold(health.Test[0], "NONE") {
		service["healthcheck"] = map[string]any{"disable": true}
		return
	}
	value := map[string]any{"test": health.Test}
	if health.Interval > 0 {
		value["interval"] = time.Duration(health.Interval).String()
	}
	if health.Timeout > 0 {
		value["timeout"] = time.Duration(health.Timeout).String()
	}
	if health.StartPeriod > 0 {
		value["start_period"] = time.Duration(health.StartPeriod).String()
	}
	if health.Retries > 0 {
		value["retries"] = health.Retries
	}
	service["healthcheck"] = value
}

func addContainerRestartPolicy(deploy map[string]any, policy containerRestartPolicy) {
	switch policy.Name {
	case "", "no", "always", "unless-stopped":
		// A migrated container becomes a long-running replicated Service. Docker's
		// default container policy "no" must not become Swarm condition "none":
		// otherwise a clean process exit permanently leaves the Service at 0/N.
		deploy["restart_policy"] = map[string]any{"condition": "any"}
	case "on-failure":
		value := map[string]any{"condition": "on-failure"}
		if policy.MaximumRetryCount > 0 {
			value["max_attempts"] = policy.MaximumRetryCount
		}
		deploy["restart_policy"] = value
	}
}

func shortID(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func printContainerMigrationChecks(
	output io.Writer,
	container, stack, service string,
	checks []composeMigrationCheck,
	sourceHash string,
	generated generatedContainerStack,
) {
	_ = generated
	printSwarmMigrationReport(output, swarmMigrationReport{
		Title:    "独立容器 → Swarm 迁移检查",
		Subtitle: "只读检查 · 不会停止源容器",
		Summary: []cliui.Pair{
			{Label: "源容器", Value: valueOrDash(container)},
			{Label: "目标 Stack", Value: valueOrDash(stack)},
			{Label: "目标 Service", Value: valueOrDash(service)},
		},
		Checks: checks,
		Hash:   sourceHash,
	})
}
