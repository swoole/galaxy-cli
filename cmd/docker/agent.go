package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"

	"github.com/AlecAivazis/survey/v2"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	agentConfigReloadInterval = 5 * time.Second
	agentHeartbeatInterval    = 10 * time.Second
	agentHeartbeatTimeout     = 25 * time.Second
	agentReadTimeout          = 40 * time.Second
	agentWriteTimeout         = 30 * time.Second

	// envAgentImage 指定 Agent 镜像地址，等价于安装时的 --image。
	envAgentImage = "GALAXY_AGENT_IMAGE"
)

var agentConfigWriteLock sync.Mutex

type agentFile struct {
	Connections  map[string]agentConnection `yaml:"connections"`
	LegacyToken  string                     `yaml:"token,omitempty"`
	LegacyRemote map[string]any             `yaml:"remotes,omitempty"`
}

type agentConnection struct {
	Name              string       `yaml:"name"`
	Token             string       `yaml:"token"`
	Target            string       `yaml:"target"`
	AgentImage        string       `yaml:"agent_image,omitempty"`
	CredentialVersion int          `yaml:"credential_version,omitempty"`
	DeployPending     bool         `yaml:"deploy_pending,omitempty"`
	Disabled          bool         `yaml:"disabled,omitempty"`
	Remote            remoteConfig `yaml:"remote"`
}

type remoteConfig struct {
	Mode         string `yaml:"mode,omitempty"`
	DockerSocket string `yaml:"docker_socket,omitempty"`
}

type relayMessage struct {
	Type              string              `json:"type"`
	ID                string              `json:"id,omitempty"`
	Method            string              `json:"method,omitempty"`
	Path              string              `json:"path,omitempty"`
	Headers           map[string][]string `json:"headers,omitempty"`
	Body              string              `json:"body,omitempty"`
	Data              string              `json:"data,omitempty"`
	Status            int                 `json:"status,omitempty"`
	Error             string              `json:"error,omitempty"`
	Command           string              `json:"command,omitempty"`
	SwarmID           string              `json:"swarm_id,omitempty"`
	NodeID            string              `json:"node_id,omitempty"`
	CredentialVersion int                 `json:"credential_version,omitempty"`
	MachineCredential string              `json:"machine_credential,omitempty"`
	RegistryAuth      string              `json:"registry_auth,omitempty"`
}

type nodeIdentity struct {
	SwarmID    string
	NodeID     string
	NodeAddr   string
	Hostname   string
	Role       string
	SwarmNodes int
}

type agentDeployment struct {
	APIURL            string
	Image             string
	SwarmID           string
	Credential        string
	CredentialVersion int
	JoinAddresses     []string
	RegistryAuth      string
}

type agentOptions struct {
	configFlags     *galaxycfg.ConfigFlags
	streams         galaxycfg.IOStreams
	configPath      string
	dockerSocket    string
	remoteName      string
	connectionName  string
	agentToken      string
	agentTokenFile  string
	agentImage      string
	setAgentImage   string
	localOnly       bool
	reportHostAddrs bool
}

// NewCmdAgentInstall exposes the one-shot Agent installer from the distributed
// galaxy CLI. The long-running workload remains the galaxy-agent Global
// Service; no CLI process is left running after installation succeeds.
func NewCmdAgentInstall(configFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := &agentOptions{
		configFlags:     configFlags,
		streams:         streams,
		configPath:      filepath.Join(filepath.Dir(defaultAgentConfigPath()), "agent-install.yaml"),
		dockerSocket:    "/var/run/docker.sock",
		localOnly:       true,
		reportHostAddrs: true,
	}
	command := &cobra.Command{
		Use:   "agent",
		Short: "安装和维护 Galaxy Swarm Agent",
		Long:  "使用本机 Docker Socket 注册 Swarm，并将 galaxy-agent 以 Global Service 部署到所有 Linux 节点。",
	}
	install := &cobra.Command{
		Use:   "install",
		Short: "在 Swarm Manager 注册并部署 Global Agent Service",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return o.install(command.Context())
		},
	}
	install.Flags().StringVar(&o.agentToken, "bootstrap-token", "", "Galaxy API 生成的 15 分钟 Bootstrap Token")
	install.Flags().StringVar(&o.agentTokenFile, "bootstrap-token-file", "", "从权限受控文件读取 Bootstrap Token")
	install.Flags().StringVar(&o.agentImage, "image", defaultAgentImage(), "Global Agent Service 使用的镜像")
	install.Flags().StringVar(&o.dockerSocket, "docker-socket", o.dockerSocket, "Manager 本机 Docker Socket")
	command.AddCommand(install)
	set := &cobra.Command{
		Use:   "set",
		Short: "设置 Global Agent Service 参数",
		Long:  "更新现有 galaxy-agent Global Service 的管理中心地址或镜像，并触发滚动更新。",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			serverFlag := command.Flag("server")
			serverChanged := serverFlag != nil && serverFlag.Changed
			imageChanged := command.Flags().Changed("image")
			if !serverChanged && !imageChanged {
				return errors.New("至少指定一个要设置的 Agent 参数：--server 或 --image")
			}
			return o.set(command.Context(), serverChanged, imageChanged)
		},
	}
	set.Flags().StringVar(&o.dockerSocket, "docker-socket", o.dockerSocket, "Manager 本机 Docker Socket")
	set.Flags().StringVar(&o.setAgentImage, "image", "", "Global Agent Service 使用的新镜像")
	command.AddCommand(set)
	return command
}

// NewStandaloneCmdAgent builds the independent, local-Socket-only Agent
// executable. SSH-backed legacy connections are intentionally unavailable.
func NewStandaloneCmdAgent(configFlags *galaxycfg.ConfigFlags, streams galaxycfg.IOStreams) *cobra.Command {
	o := &agentOptions{
		configFlags: configFlags,
		streams:     streams,
		configPath:  defaultAgentConfigPath(),
		localOnly:   true,
	}
	cmd := &cobra.Command{
		Use:          "galaxy-agent",
		Short:        "Galaxy Swarm 节点 Agent",
		SilenceUsage: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return configFlags.Load()
		},
	}
	configFlags.AddFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().StringVar(&o.configPath, "agent-config", o.configPath, "Agent 配置文件")

	bootstrap := &cobra.Command{
		Use:   "bootstrap",
		Short: "在 Swarm Manager 上配置 Bootstrap Token",
		RunE:  func(*cobra.Command, []string) error { return o.configureLocal() },
	}
	bootstrap.Flags().StringVar(&o.remoteName, "target", "", "本机节点别名")
	bootstrap.Flags().StringVar(&o.connectionName, "name", "", "本地连接配置名称")
	bootstrap.Flags().StringVar(&o.agentToken, "bootstrap-token", "", "Galaxy API 生成的 15 分钟 Bootstrap Token")
	bootstrap.Flags().StringVar(&o.agentImage, "image", defaultAgentImage(), "Global Agent Service 使用的镜像")
	cmd.AddCommand(bootstrap)
	cmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "以前台方式运行 Agent",
		RunE:  func(command *cobra.Command, _ []string) error { return o.run(command.Context()) },
	})
	return cmd
}

func (o *agentOptions) install(parent context.Context) error {
	if err := validateAgentServerURL(o.configFlags.GetAPIServer()); err != nil {
		return err
	}
	if existing, err := loadAgentFile(o.configPath); err == nil {
		for _, connection := range existing.Connections {
			if connection.DeployPending && connection.CredentialVersion > 0 {
				fmt.Fprintln(o.streams.Out, "检测到上次注册已获得机器凭证，继续部署 Global Agent Service")
				if err := o.runInstallConnection(parent); err != nil {
					return err
				}
				if err := os.Remove(o.configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("Agent 已部署，但清理临时安装凭证失败：%w", err)
				}
				fmt.Fprintln(o.streams.Out, "Galaxy Agent 安装完成，临时安装凭证已清理")
				return nil
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取 Agent 安装状态失败：%w", err)
	}

	if err := requireAgentImage(o.agentImage); err != nil {
		return err
	}

	token, err := o.readBootstrapToken()
	if err != nil {
		return err
	}
	socket := filepath.Clean(strings.TrimSpace(o.dockerSocket))
	target := remoteConfig{Mode: "local", DockerSocket: socket}
	fmt.Fprintln(o.streams.Out, "1/4 检查本机 Docker Swarm Manager")
	if err := testLocalDockerManager(target); err != nil {
		return err
	}
	fmt.Fprintf(o.streams.Out, "Agent 镜像：%s（Swarm 各节点必须能从镜像仓库拉取）\n", o.agentImage)
	connection := agentConnection{
		Name:       hostnameOr("galaxy-agent-bootstrap"),
		Token:      token,
		Target:     "local-manager",
		AgentImage: o.agentImage,
		Remote:     target,
	}
	config := agentFile{Connections: map[string]agentConnection{"install": connection}}
	if err := validateAgentConnections(config.Connections); err != nil {
		return err
	}
	if err := saveAgentFile(o.configPath, config); err != nil {
		return fmt.Errorf("保存临时 Agent 安装状态失败：%w", err)
	}
	fmt.Fprintln(o.streams.Out, "2/4 使用 Bootstrap Token 注册 Swarm")
	fmt.Fprintln(o.streams.Out, "3/4 创建机器凭证、加密控制网络和 Docker Secret")
	if err := o.runInstallConnection(parent); err != nil {
		return fmt.Errorf("%w；可重新执行相同命令继续或重试", err)
	}
	fmt.Fprintln(o.streams.Out, "4/4 Global Agent Service 已部署到所有 Linux 节点")
	if err := os.Remove(o.configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Agent 已部署，但清理临时安装凭证失败：%w", err)
	}
	fmt.Fprintln(o.streams.Out, "Galaxy Agent 安装完成，临时安装凭证已清理")
	return nil
}

func (o *agentOptions) set(_ context.Context, serverChanged bool, imageChanged bool) error {
	apiURL := strings.TrimRight(strings.TrimSpace(o.configFlags.GetAPIServer()), "/")
	if serverChanged {
		if err := validateAgentServerURL(apiURL); err != nil {
			return err
		}
	}
	image := strings.TrimSpace(o.setAgentImage)
	if imageChanged {
		if err := validateAgentImage(image); err != nil {
			return err
		}
	}
	target := remoteConfig{Mode: "local", DockerSocket: filepath.Clean(strings.TrimSpace(o.dockerSocket))}
	fmt.Fprintln(o.streams.Out, "1/3 检查本机 Docker Swarm Manager")
	if err := testLocalDockerManager(target); err != nil {
		return err
	}
	fmt.Fprintln(o.streams.Out, "2/3 更新 galaxy-agent Service 参数")
	executor := &remoteExecutor{remote: target}
	defer executor.close()
	if err := executor.setGlobalAgent(apiURL, serverChanged, image, imageChanged, hostJoinAddresses()); err != nil {
		return err
	}
	fmt.Fprintln(o.streams.Out, "3/3 已触发 Global Agent Service 滚动更新")
	if serverChanged {
		fmt.Fprintf(o.streams.Out, "管理中心地址：%s\n", apiURL)
	}
	if imageChanged {
		fmt.Fprintf(o.streams.Out, "Agent 镜像：%s\n", image)
	}
	fmt.Fprintln(o.streams.Out, "Galaxy Agent 参数更新完成")
	return nil
}

func (o *agentOptions) runInstallConnection(parent context.Context) error {
	config, err := loadAgentFile(o.configPath)
	if err != nil {
		return err
	}
	if err := validateAgentConnections(config.Connections); err != nil {
		return err
	}
	if len(config.Connections) != 1 {
		return fmt.Errorf("Agent 安装状态必须且只能包含一个 Swarm，当前为 %d 个", len(config.Connections))
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	for key, connection := range config.Connections {
		if !connection.Remote.isLocal() {
			return errors.New("Agent 安装器只允许访问 Manager 本机 Docker Socket")
		}
		return o.connectOnce(ctx, key, connection)
	}
	return errors.New("Agent 安装状态为空")
}

func (o *agentOptions) readBootstrapToken() (string, error) {
	if strings.TrimSpace(o.agentToken) != "" && strings.TrimSpace(o.agentTokenFile) != "" {
		return "", errors.New("--bootstrap-token 与 --bootstrap-token-file 不能同时使用")
	}
	if strings.TrimSpace(o.agentTokenFile) != "" {
		info, err := os.Stat(o.agentTokenFile)
		if err != nil {
			return "", fmt.Errorf("读取 Bootstrap Token 文件失败：%w", err)
		}
		if !info.Mode().IsRegular() || info.Size() > 4096 {
			return "", errors.New("Bootstrap Token 文件必须是小于 4 KiB 的普通文件")
		}
		content, err := os.ReadFile(o.agentTokenFile)
		if err != nil {
			return "", fmt.Errorf("读取 Bootstrap Token 文件失败：%w", err)
		}
		o.agentToken = strings.TrimSpace(string(content))
	}
	if strings.TrimSpace(o.agentToken) == "" {
		answer := ""
		if err := survey.AskOne(
			&survey.Password{Message: "15 分钟 Bootstrap Token："},
			&answer,
			survey.WithValidator(survey.Required),
		); err != nil {
			return "", err
		}
		o.agentToken = answer
	}
	token := strings.TrimSpace(o.agentToken)
	if token == "" {
		return "", errors.New("Bootstrap Token 不能为空")
	}
	return token, nil
}

func defaultAgentConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "galaxy-agent.yaml")
	}
	return filepath.Join(home, ".galaxy", "agents.yaml")
}

func (o *agentOptions) configureLocal() error {
	config, err := loadAgentFile(o.configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取 Agent 配置失败：%w", err)
	}
	if config.Connections == nil {
		config.Connections = map[string]agentConnection{}
	}
	if err := requireAgentImage(o.agentImage); err != nil {
		return err
	}
	answers := struct {
		Name, Agent, Token, Target, Socket string
	}{Name: o.connectionName, Agent: hostnameOr("galaxy-agent"), Token: o.agentToken, Target: o.remoteName, Socket: "/var/run/docker.sock"}
	if answers.Target == "" {
		answers.Target = "local-manager"
	}
	if answers.Name == "" {
		answers.Name = uniqueConnectionName(config.Connections, answers.Target)
	}
	questions := []*survey.Question{
		{Name: "name", Prompt: &survey.Input{Message: "本地连接配置名称：", Default: answers.Name}, Validate: survey.Required},
		{Name: "agent", Prompt: &survey.Input{Message: "Agent 名称：", Default: answers.Agent}, Validate: survey.Required},
		{Name: "target", Prompt: &survey.Input{Message: "本机 Docker 目标别名：", Default: answers.Target}, Validate: survey.Required},
		{Name: "socket", Prompt: &survey.Input{Message: "Docker Socket：", Default: answers.Socket}, Validate: survey.Required},
	}
	if answers.Token == "" {
		questions = append([]*survey.Question{{Name: "token", Prompt: &survey.Password{Message: "集群 Agent Token："}, Validate: survey.Required}}, questions...)
	}
	if err := survey.Ask(questions, &answers); err != nil {
		return err
	}
	target := remoteConfig{Mode: "local", DockerSocket: filepath.Clean(strings.TrimSpace(answers.Socket))}
	if err := testLocalDockerManager(target); err != nil {
		return err
	}
	connection := agentConnection{Name: strings.TrimSpace(answers.Agent), Token: strings.TrimSpace(answers.Token), Target: strings.TrimSpace(answers.Target), Remote: target}
	connection.AgentImage = o.agentImage
	key, err := selectConnectionKey(config.Connections, strings.TrimSpace(answers.Name), connection)
	if err != nil {
		return err
	}
	config.Connections[key] = connection
	if err := validateAgentConnections(config.Connections); err != nil {
		return err
	}
	if o.localOnly {
		for name, connection := range config.Connections {
			if !connection.Remote.isLocal() {
				return fmt.Errorf("Agent 不支持旧 SSH 连接 %s；请在 Swarm 节点使用本机 Docker Socket", name)
			}
		}
	}
	if err := saveAgentFile(o.configPath, config); err != nil {
		return err
	}
	fmt.Fprintf(o.streams.Out, "\n连接 %s 已保存：%s\n运行 galaxy-agent run 后完成注册并部署 Global Agent Service。\n", key, o.configPath)
	return nil
}

func (o *agentOptions) run(parent context.Context) error {
	config, err := loadAgentFile(o.configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		config = agentFile{Connections: map[string]agentConnection{}}
	}
	if config.Connections == nil {
		config.Connections = map[string]agentConnection{}
	}
	environmentMode := false
	if connection, apiURL, envErr := environmentAgentConnection(); envErr != nil {
		return envErr
	} else if connection != nil {
		environmentMode = true
		config.Connections["service"] = *connection
		if apiURL != "" {
			*o.configFlags.APIServer = apiURL
		}
	}
	if len(config.Connections) == 0 {
		return errors.New("Agent 尚未配置，请先运行 galaxy-agent bootstrap")
	}
	if err := validateAgentConnections(config.Connections); err != nil {
		return err
	}
	for name, connection := range config.Connections {
		if !connection.Remote.isLocal() {
			return fmt.Errorf("Agent 不支持旧 SSH 连接 %s；请在 Swarm 节点使用本机 Docker Socket", name)
		}
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	type activeConnection struct {
		config agentConnection
		cancel context.CancelFunc
	}
	active := map[string]activeConnection{}
	reconcile := func(next agentFile) {
		changed := false
		for key, running := range active {
			connection, exists := next.Connections[key]
			if exists && connection == running.config {
				continue
			}
			running.cancel()
			delete(active, key)
			changed = true
		}
		for key, connection := range next.Connections {
			if connection.Disabled {
				continue
			}
			if _, exists := active[key]; exists {
				continue
			}
			connectionContext, cancel := context.WithCancel(ctx)
			active[key] = activeConnection{config: connection, cancel: cancel}
			go o.runConnection(connectionContext, key, connection)
			changed = true
		}
		if changed {
			fmt.Fprintf(
				o.streams.Out,
				"Galaxy Agent %s（镜像 %s）配置已加载，共 %d 条连接\n",
				buildVariable.BuildVersion,
				defaultAgentImage(),
				len(active),
			)
		}
	}
	reconcile(config)
	if environmentMode {
		<-ctx.Done()
		for _, running := range active {
			running.cancel()
		}
		return nil
	}
	ticker := time.NewTicker(agentConfigReloadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, running := range active {
				running.cancel()
			}
			return nil
		case <-ticker.C:
			next, loadError := loadAgentFile(o.configPath)
			if loadError != nil {
				fmt.Fprintf(o.streams.ErrOut, "Agent 配置重新加载失败，继续使用上一版本：%v\n", loadError)
				continue
			}
			if validationError := validateAgentConnections(next.Connections); validationError != nil {
				fmt.Fprintf(o.streams.ErrOut, "Agent 配置无效，继续使用上一版本：%v\n", validationError)
				continue
			}
			reconcile(next)
		}
	}
}

func (o *agentOptions) runConnection(ctx context.Context, key string, connection agentConnection) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := o.connectOnce(ctx, key, connection)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			return
		}
		fmt.Fprintf(o.streams.ErrOut, "连接 %s 中断：%v；%s 后重连\n", key, err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (o *agentOptions) connectOnce(ctx context.Context, key string, config agentConnection) error {
	executor := &remoteExecutor{remote: config.Remote}
	status, _, encodedInfo, requestError := executor.execute(relayMessage{Method: http.MethodGet, Path: "/info"})
	if requestError != "" || status != http.StatusOK {
		executor.close()
		if requestError != "" {
			return fmt.Errorf("Docker 目标预检失败：%s", requestError)
		}
		return fmt.Errorf("Docker 目标预检返回 HTTP %d", status)
	}
	identity, err := parseNodeIdentity(encodedInfo, os.Getenv("GALAXY_SWARM_ID"))
	if err != nil {
		executor.close()
		return err
	}
	if identity.Role == "manager" {
		nodeStatus, _, encodedNodes, nodeError := executor.execute(relayMessage{Method: http.MethodGet, Path: "/nodes"})
		if nodeError != "" || nodeStatus != http.StatusOK {
			executor.close()
			if nodeError != "" {
				return fmt.Errorf("读取 Swarm Ready 节点失败：%s", nodeError)
			}
			return fmt.Errorf("读取 Swarm Ready 节点返回 HTTP %d", nodeStatus)
		}
		identity.SwarmNodes, err = readySwarmNodeCount(encodedNodes)
		if err != nil {
			executor.close()
			return err
		}
	}
	header := http.Header{
		"Authorization":          []string{"Agent " + config.Token},
		"X-Galaxy-Swarm-ID":      []string{identity.SwarmID},
		"X-Galaxy-Node-ID":       []string{identity.NodeID},
		"X-Galaxy-Node-Role":     []string{identity.Role},
		"X-Galaxy-Node-Addr":     []string{identity.NodeAddr},
		"X-Galaxy-Node-Hostname": []string{identity.Hostname},
		"X-Galaxy-Agent-Version": []string{buildVariable.BuildVersion},
		"X-Galaxy-Swarm-Nodes":   []string{strconv.Itoa(identity.SwarmNodes)},
	}
	if o.reportHostAddrs {
		header.Set("X-Galaxy-Join-Addresses", strings.Join(hostJoinAddresses(), ","))
		header.Set("X-Galaxy-Agent-Image", strings.TrimSpace(config.AgentImage))
	} else if addresses := strings.TrimSpace(os.Getenv("GALAXY_JOIN_ADDRESSES")); addresses != "" {
		header.Set("X-Galaxy-Join-Addresses", addresses)
	}
	apiURL := strings.TrimRight(strings.TrimSpace(o.configFlags.GetAPIServer()), "/")
	candidates, err := agentServerCandidates(apiURL, identity)
	if err != nil {
		executor.close()
		return err
	}
	var conn *websocket.Conn
	var response *http.Response
	var connectErr error
	effectiveAPIURL := ""
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	for _, candidate := range candidates {
		wsURL, urlErr := agentWebSocketURL(candidate)
		if urlErr != nil {
			connectErr = urlErr
			continue
		}
		conn, response, connectErr = dialer.DialContext(ctx, wsURL, header)
		if connectErr == nil {
			effectiveAPIURL = candidate
			break
		}
	}
	if connectErr != nil {
		executor.close()
		if response != nil {
			return fmt.Errorf("WebSocket 握手失败（HTTP %d）：%w", response.StatusCode, connectErr)
		}
		return connectErr
	}
	if identity.Role == "manager" && effectiveAPIURL != apiURL {
		fmt.Fprintf(
			o.streams.Out,
			"检测到 Manager 地址变化，Galaxy API 地址由 %s 自动恢复为 %s\n",
			apiURL,
			effectiveAPIURL,
		)
		joinAddresses := []string{identity.NodeAddr}
		if err := executor.setGlobalAgent(effectiveAPIURL, true, "", false, joinAddresses); err != nil {
			fmt.Fprintf(o.streams.ErrOut, "自动更新 Global Agent Service 地址失败：%v\n", err)
		} else {
			fmt.Fprintln(o.streams.Out, "Global Agent Service 地址已自动更新，正在滚动恢复所有节点")
		}
	}
	defer conn.Close()
	var writeLock sync.Mutex
	writeMessage := func(message relayMessage) error {
		writeLock.Lock()
		defer writeLock.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(agentWriteTimeout)); err != nil {
			return err
		}
		return conn.WriteJSON(message)
	}
	streams := map[string]*dockerStream{}
	var streamsLock sync.Mutex
	defer func() {
		streamsLock.Lock()
		defer streamsLock.Unlock()
		for _, stream := range streams {
			stream.close()
		}
		executor.close()
	}()
	fmt.Fprintf(o.streams.Out, "连接 %s 已上线（Agent %s，目标 %s）\n", key, config.Name, config.Target)
	// Only a heartbeat acknowledgement proves that the API is responding.
	var lastHeartbeatAck atomic.Int64
	lastHeartbeatAck.Store(time.Now().UnixNano())
	_ = conn.SetReadDeadline(time.Now().Add(agentReadTimeout))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(agentReadTimeout)) })
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			writeLock.Lock()
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "agent stopping"), time.Now().Add(time.Second))
			_ = conn.Close()
			writeLock.Unlock()
		case <-done:
		}
	}()
	go func() {
		ticker := time.NewTicker(agentHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastHeartbeatAck.Load())) >= agentHeartbeatTimeout {
					fmt.Fprintf(o.streams.ErrOut, "连接 %s 的服务器心跳超时，主动断开并重连\n", key)
					_ = conn.Close()
					return
				}
				if err := writeMessage(relayMessage{Type: "heartbeat"}); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				writeLock.Lock()
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				writeLock.Unlock()
				if err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, _, _, healthError := executor.execute(relayMessage{Method: http.MethodGet, Path: "/_ping"})
				if healthError == "" && status == http.StatusOK {
					continue
				}
				fmt.Fprintf(o.streams.ErrOut, "连接 %s 的 Docker 健康检查失败：HTTP %d %s\n", key, status, healthError)
				writeLock.Lock()
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "docker target offline"), time.Now().Add(time.Second))
				_ = conn.Close()
				writeLock.Unlock()
				return
			}
		}
	}()
	defer close(done)
	for {
		var request relayMessage
		if err := conn.ReadJSON(&request); err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(agentReadTimeout))
		if request.Type == "error" {
			return errors.New(request.Error)
		}
		switch request.Type {
		case "heartbeat_ack":
			lastHeartbeatAck.Store(time.Now().UnixNano())
		case "ready":
			if request.MachineCredential != "" {
				if err := updateAgentBootstrapState(o.configPath, key, config.Token, request.MachineCredential, request.CredentialVersion, true, false); err != nil {
					return fmt.Errorf("保存集群机器凭证失败：%w", err)
				}
				config.Token = request.MachineCredential
				config.CredentialVersion = request.CredentialVersion
				config.DeployPending = true
				fmt.Fprintf(o.streams.Out, "集群 %s 已注册，机器凭证版本 v%d 已保存\n", request.SwarmID, request.CredentialVersion)
			}
			if config.DeployPending {
				if identity.Role != "manager" {
					return errors.New("只有 Swarm Manager Agent 可以部署 Global Agent Service")
				}
				if request.RegistryAuth != "" {
					fmt.Fprintln(o.streams.Out, "已从 Galaxy API 获取 Agent 镜像仓库凭证")
				}
				if err := executor.deployGlobalAgent(agentDeployment{
					APIURL:            o.configFlags.GetAPIServer(),
					Image:             config.AgentImage,
					SwarmID:           identity.SwarmID,
					Credential:        config.Token,
					CredentialVersion: config.CredentialVersion,
					JoinAddresses:     hostJoinAddresses(),
					RegistryAuth:      request.RegistryAuth,
				}); err != nil {
					return fmt.Errorf("部署 Global Agent Service 失败：%w", err)
				}
				if err := updateAgentBootstrapState(o.configPath, key, config.Token, config.Token, config.CredentialVersion, false, true); err != nil {
					return fmt.Errorf("更新 Bootstrap 状态失败：%w", err)
				}
				fmt.Fprintln(o.streams.Out, "Global Agent Service 已部署，Bootstrap Agent 将停止")
				return nil
			}
		case "request":
			go func(request relayMessage) {
				response := relayMessage{Type: "response", ID: request.ID}
				response.Status, response.Headers, response.Body, response.Error = executor.execute(request)
				_ = writeMessage(response)
			}(request)
		case "domain_command":
			go func(request relayMessage) {
				response := relayMessage{Type: "domain_response", ID: request.ID}
				response.Status, response.Headers, response.Body, response.Error = executor.executeDomainCommand(request.Command)
				_ = writeMessage(response)
			}(request)
		case "stream_open":
			go func(request relayMessage) {
				stream, err := executor.openStream(request)
				if err != nil {
					_ = writeMessage(relayMessage{Type: "stream_ready", ID: request.ID, Error: err.Error()})
					return
				}
				streamsLock.Lock()
				streams[request.ID] = stream
				streamsLock.Unlock()
				_ = writeMessage(relayMessage{Type: "stream_ready", ID: request.ID, Status: stream.status})
				stream.pump(request.ID, writeMessage)
				streamsLock.Lock()
				delete(streams, request.ID)
				streamsLock.Unlock()
			}(request)
		case "stream_data":
			streamsLock.Lock()
			stream := streams[request.ID]
			streamsLock.Unlock()
			if stream != nil {
				data, _ := base64.StdEncoding.DecodeString(request.Data)
				_ = stream.write(data)
			}
		case "stream_close":
			streamsLock.Lock()
			stream := streams[request.ID]
			delete(streams, request.ID)
			streamsLock.Unlock()
			if stream != nil {
				stream.close()
			}
		}
	}
}

func hostJoinAddresses() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	type candidate struct {
		address  string
		priority int
		index    int
	}
	candidates := make([]candidate, 0)
	seen := make(map[string]struct{})
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || virtualInterface(iface.Name) {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ipText, _, err := net.ParseCIDR(address.String())
			if err != nil {
				continue
			}
			ip := net.ParseIP(ipText.String())
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
				continue
			}
			value := ip.String()
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			priority := 2
			if ip4 := ip.To4(); ip4 != nil {
				priority = 1
				if ip4[0] == 10 || (ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) || (ip4[0] == 192 && ip4[1] == 168) {
					priority = 0
				}
			}
			candidates = append(candidates, candidate{address: value, priority: priority, index: iface.Index})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		return candidates[i].index < candidates[j].index
	})
	result := make([]string, 0, len(candidates))
	for _, item := range candidates {
		result = append(result, item.address)
	}
	return result
}

func virtualInterface(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "cni", "flannel", "tun", "tap"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

type remoteExecutor struct {
	remote     remoteConfig
	mu         sync.Mutex
	transport  *http.Transport
	httpClient *http.Client
}

func (e *remoteExecutor) close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.transport != nil {
		e.transport.CloseIdleConnections()
	}
}

func (e *remoteExecutor) execute(request relayMessage) (int, map[string][]string, string, string) {
	body, err := base64.StdEncoding.DecodeString(request.Body)
	if err != nil {
		return 0, nil, "", "请求正文编码无效"
	}
	path := request.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	httpRequest, err := http.NewRequest(request.Method, "http://docker"+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, "", err.Error()
	}
	httpRequest.Header = request.Headers
	response, err := e.getHTTPClient().Do(httpRequest)
	if err != nil {
		return 0, nil, "", "Docker Agent 请求失败：" + err.Error()
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return 0, nil, "", err.Error()
	}
	return response.StatusCode, response.Header, base64.StdEncoding.EncodeToString(data), ""
}

func (e *remoteExecutor) executeDomainCommand(command string) (int, map[string][]string, string, string) {
	path, ok := map[string]string{
		"swarm.version": "/version",
		"swarm.info":    "/info",
		"swarm.inspect": "/swarm",
	}[command]
	if !ok {
		return 0, nil, "", "不支持的 Agent 领域命令：" + command
	}
	status, headers, body, requestError := e.execute(relayMessage{
		Method: http.MethodGet,
		Path:   path,
	})
	if requestError != "" || status < 400 {
		return status, headers, body, requestError
	}
	raw, _ := base64.StdEncoding.DecodeString(body)
	return status, headers, body, fmt.Sprintf("Docker Engine 返回 HTTP %d：%s", status, strings.TrimSpace(string(raw)))
}

func (e *remoteExecutor) deployGlobalAgent(deployment agentDeployment) error {
	if !e.remote.isLocal() {
		return errors.New("Global Agent Service 只能通过 Manager 本机 Docker Socket 部署")
	}
	if strings.TrimSpace(deployment.APIURL) == "" || strings.TrimSpace(deployment.Credential) == "" {
		return errors.New("Global Agent Service 缺少 API 地址或机器凭证")
	}
	if err := validateAgentServerURL(deployment.APIURL); err != nil {
		return err
	}
	if deployment.CredentialVersion < 1 {
		return errors.New("Global Agent Service 机器凭证版本无效")
	}
	if strings.TrimSpace(deployment.SwarmID) == "" {
		return errors.New("Global Agent Service 缺少 Swarm ID")
	}
	image := strings.TrimSpace(deployment.Image)
	if image == "" {
		image = defaultAgentImage()
	}
	registryAuth := strings.TrimSpace(deployment.RegistryAuth)
	if registryAuth == "" {
		var err error
		registryAuth, err = dockerRegistryAuthHeader(image)
		if err != nil {
			return err
		}
	}

	networkID, err := e.ensureAgentNetwork()
	if err != nil {
		return err
	}
	secretName := fmt.Sprintf("galaxy-agent-credential-v%d", deployment.CredentialVersion)
	secretID, err := e.ensureAgentSecret(secretName, deployment.Credential)
	if err != nil {
		return err
	}

	spec := map[string]any{
		"Name": "galaxy-agent",
		"Labels": map[string]string{
			"com.code-galaxy.component":          "agent",
			"com.code-galaxy.credential-version": strconv.Itoa(deployment.CredentialVersion),
		},
		"TaskTemplate": map[string]any{
			"ContainerSpec": map[string]any{
				"Image":    image,
				"ReadOnly": true,
				"Env": []string{
					"GALAXY_API_URL=" + strings.TrimRight(deployment.APIURL, "/"),
					"GALAXY_BASE_URL=" + strings.TrimRight(deployment.APIURL, "/"),
					"GALAXY_AGENT_CREDENTIAL_FILE=/run/secrets/agent-credential",
					"GALAXY_AGENT_CREDENTIAL_VERSION=" + strconv.Itoa(deployment.CredentialVersion),
					"GALAXY_SWARM_ID=" + deployment.SwarmID,
					"GALAXY_DOCKER_SOCKET=/var/run/docker.sock",
					"GALAXY_JOIN_ADDRESSES=" + strings.Join(deployment.JoinAddresses, ","),
					"HOME=/tmp",
				},
				"Mounts": []map[string]any{
					{"Type": "bind", "Source": "/var/run/docker.sock", "Target": "/var/run/docker.sock"},
					{"Type": "tmpfs", "Target": "/tmp", "TmpfsOptions": map[string]any{"SizeBytes": 16 << 20, "Mode": 448}},
				},
				"Secrets": []map[string]any{{
					"SecretID": secretID, "SecretName": secretName,
					"File": map[string]any{"Name": "agent-credential", "UID": "0", "GID": "0", "Mode": 256},
				}},
				"CapabilityDrop": []string{"ALL"},
			},
			"RestartPolicy": map[string]any{"Condition": "any", "Delay": int64(5 * time.Second)},
			"Placement":     map[string]any{"Constraints": []string{"node.platform.os == linux"}},
			"Networks":      []map[string]string{{"Target": networkID}},
		},
		"Mode": map[string]any{"Global": map[string]any{}},
	}

	var services []struct {
		ID      string `json:"ID"`
		Version struct {
			Index uint64 `json:"Index"`
		} `json:"Version"`
		Spec struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Spec"`
	}
	if err := e.dockerJSON(http.MethodGet, dockerFilteredPath("/services", "name", "galaxy-agent"), nil, &services); err != nil {
		return fmt.Errorf("查询 Global Agent Service 失败：%w", err)
	}
	if len(services) == 0 {
		if err := e.dockerJSONWithHeaders(http.MethodPost, "/services/create", spec, nil, registryAuthHeaders(registryAuth)); err != nil {
			return fmt.Errorf("创建 Global Agent Service 失败：%w", err)
		}
		return nil
	}
	if services[0].Spec.Labels["com.code-galaxy.component"] != "agent" {
		return errors.New("同名 galaxy-agent Service 不受 Galaxy 管理，拒绝覆盖")
	}
	updatePath := agentServiceUpdatePath(services[0].ID, services[0].Version.Index, registryAuth != "")
	if err := e.dockerJSONWithHeaders(http.MethodPost, updatePath, spec, nil, registryAuthHeaders(registryAuth)); err != nil {
		return fmt.Errorf("更新 Global Agent Service 失败：%w", err)
	}
	return nil
}

func (e *remoteExecutor) setGlobalAgent(apiURL string, updateServer bool, image string, updateImage bool, joinAddresses []string) error {
	if !e.remote.isLocal() {
		return errors.New("Global Agent Service 只能通过 Manager 本机 Docker Socket 修改")
	}
	if updateServer {
		if err := validateAgentServerURL(apiURL); err != nil {
			return err
		}
	}
	if updateImage {
		if err := validateAgentImage(image); err != nil {
			return err
		}
	}
	registryAuth := ""
	if updateImage {
		var err error
		registryAuth, err = dockerRegistryAuthHeader(image)
		if err != nil {
			return err
		}
	}
	status, _, encodedInfo, requestError := e.execute(relayMessage{Method: http.MethodGet, Path: "/info"})
	if requestError != "" || status != http.StatusOK {
		if requestError != "" {
			return fmt.Errorf("读取 Manager Swarm ID 失败：%s", requestError)
		}
		return fmt.Errorf("读取 Manager Swarm ID 返回 HTTP %d", status)
	}
	managerIdentity, err := parseNodeIdentity(encodedInfo, "")
	if err != nil {
		return err
	}
	if managerIdentity.Role != "manager" {
		return errors.New("只有 Swarm Manager 可以修改 Global Agent Service")
	}
	var services []struct {
		ID      string `json:"ID"`
		Version struct {
			Index uint64 `json:"Index"`
		} `json:"Version"`
		Spec map[string]any `json:"Spec"`
	}
	if err := e.dockerJSON(http.MethodGet, dockerFilteredPath("/services", "name", "galaxy-agent"), nil, &services); err != nil {
		return fmt.Errorf("查询 Global Agent Service 失败：%w", err)
	}
	if len(services) == 0 {
		return errors.New("未找到 galaxy-agent Service，请先执行 galaxy agent install")
	}
	labels, _ := services[0].Spec["Labels"].(map[string]any)
	if fmt.Sprint(labels["com.code-galaxy.component"]) != "agent" {
		return errors.New("同名 galaxy-agent Service 不受 Galaxy 管理，拒绝修改")
	}
	taskTemplate, ok := services[0].Spec["TaskTemplate"].(map[string]any)
	if !ok {
		return errors.New("galaxy-agent Service TaskTemplate 无效")
	}
	containerSpec, ok := taskTemplate["ContainerSpec"].(map[string]any)
	if !ok {
		return errors.New("galaxy-agent Service ContainerSpec 无效")
	}
	containerSpec["Env"] = mergeAgentServiceEnv(
		containerSpec["Env"],
		apiURL,
		updateServer,
		managerIdentity.SwarmID,
		joinAddresses,
	)
	if updateImage {
		containerSpec["Image"] = image
	}
	if err := normalizeGlobalAgentServiceSpec(services[0].Spec); err != nil {
		return err
	}
	updatePath := agentServiceUpdatePath(services[0].ID, services[0].Version.Index, registryAuth != "")
	if err := e.dockerJSONWithHeaders(
		http.MethodPost,
		updatePath,
		services[0].Spec,
		nil,
		registryAuthHeaders(registryAuth),
	); err != nil {
		return fmt.Errorf("更新 Global Agent Service 失败：%w", err)
	}
	return nil
}

func normalizeGlobalAgentServiceSpec(spec map[string]any) error {
	mode, ok := spec["Mode"].(map[string]any)
	if !ok {
		return errors.New("galaxy-agent Service Mode 无效")
	}
	if _, global := mode["Global"]; !global {
		return errors.New("galaxy-agent 必须是 Global Service")
	}
	// Docker Engine models Global as an empty JSON object. Some Engine/API
	// combinations decode that marker as [] when a ServiceSpec is read into a
	// generic structure; sending the array back makes /services/{id}/update
	// reject the complete spec.
	mode["Global"] = map[string]any{}
	delete(mode, "Replicated")
	delete(mode, "ReplicatedJob")
	delete(mode, "GlobalJob")
	return nil
}

func mergeAgentServiceEnv(raw any, apiURL string, updateServer bool, swarmID string, joinAddresses []string) []string {
	result := make([]string, 0)
	keep := func(entry string) bool {
		if strings.HasPrefix(entry, "GALAXY_JOIN_ADDRESSES=") || strings.HasPrefix(entry, "GALAXY_SWARM_ID=") {
			return false
		}
		return !updateServer || (!strings.HasPrefix(entry, "GALAXY_API_URL=") && !strings.HasPrefix(entry, "GALAXY_BASE_URL="))
	}
	if values, ok := raw.([]any); ok {
		for _, value := range values {
			entry := fmt.Sprint(value)
			if keep(entry) {
				result = append(result, entry)
			}
		}
	} else if values, ok := raw.([]string); ok {
		for _, entry := range values {
			if keep(entry) {
				result = append(result, entry)
			}
		}
	}
	if updateServer {
		result = append(result, "GALAXY_API_URL="+apiURL, "GALAXY_BASE_URL="+apiURL)
	}
	return append(
		result,
		"GALAXY_SWARM_ID="+swarmID,
		"GALAXY_JOIN_ADDRESSES="+strings.Join(joinAddresses, ","),
	)
}

func (e *remoteExecutor) ensureAgentNetwork() (string, error) {
	var networks []struct {
		ID     string            `json:"Id"`
		Name   string            `json:"Name"`
		Driver string            `json:"Driver"`
		Scope  string            `json:"Scope"`
		Labels map[string]string `json:"Labels"`
	}
	if err := e.dockerJSON(http.MethodGet, dockerFilteredPath("/networks", "name", "galaxy-control"), nil, &networks); err != nil {
		return "", fmt.Errorf("查询 Agent Overlay 网络失败：%w", err)
	}
	for _, network := range networks {
		if network.Name == "galaxy-control" && network.ID != "" {
			if network.Driver != "overlay" || network.Scope != "swarm" || network.Labels["com.code-galaxy.component"] != "agent-control-network" {
				return "", errors.New("同名 galaxy-control 网络不受 Galaxy 管理，拒绝复用")
			}
			return network.ID, nil
		}
	}
	var created struct {
		ID string `json:"Id"`
	}
	err := e.dockerJSON(http.MethodPost, "/networks/create", map[string]any{
		"Name": "galaxy-control", "Driver": "overlay", "CheckDuplicate": true,
		"Attachable": false, "Ingress": false, "Options": map[string]string{"encrypted": ""},
		"Labels": map[string]string{"com.code-galaxy.component": "agent-control-network"},
	}, &created)
	if err != nil {
		return "", fmt.Errorf("创建 Agent Overlay 网络失败：%w", err)
	}
	if created.ID == "" {
		return "", errors.New("Docker Engine 未返回 Agent Overlay 网络 ID")
	}
	return created.ID, nil
}

func (e *remoteExecutor) ensureAgentSecret(name, credential string) (string, error) {
	var secrets []struct {
		ID   string `json:"ID"`
		Spec struct {
			Name   string            `json:"Name"`
			Labels map[string]string `json:"Labels"`
		} `json:"Spec"`
	}
	if err := e.dockerJSON(http.MethodGet, dockerFilteredPath("/secrets", "name", name), nil, &secrets); err != nil {
		return "", fmt.Errorf("查询 Agent Credential Secret 失败：%w", err)
	}
	for _, secret := range secrets {
		if secret.Spec.Name == name && secret.ID != "" {
			if secret.Spec.Labels["com.code-galaxy.component"] != "agent-credential" {
				return "", fmt.Errorf("同名 Secret %s 不受 Galaxy 管理，拒绝复用", name)
			}
			return secret.ID, nil
		}
	}
	var created struct {
		ID string `json:"ID"`
	}
	err := e.dockerJSON(http.MethodPost, "/secrets/create", map[string]any{
		"Name": name,
		"Labels": map[string]string{
			"com.code-galaxy.component": "agent-credential",
		},
		"Data": base64.StdEncoding.EncodeToString([]byte(credential)),
	}, &created)
	if err != nil {
		return "", fmt.Errorf("创建 Agent Credential Secret 失败：%w", err)
	}
	if created.ID == "" {
		return "", errors.New("Docker Engine 未返回 Agent Credential Secret ID")
	}
	return created.ID, nil
}

func (e *remoteExecutor) dockerJSON(method, path string, payload any, output any) error {
	return e.dockerJSONWithHeaders(method, path, payload, output, nil)
}

func (e *remoteExecutor) dockerJSONWithHeaders(method, path string, payload any, output any, extraHeaders map[string][]string) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	headers := map[string][]string{"Content-Type": {"application/json"}}
	for key, values := range extraHeaders {
		headers[key] = values
	}
	status, _, encoded, requestError := e.execute(relayMessage{
		Method:  method,
		Path:    path,
		Headers: headers,
		Body:    base64.StdEncoding.EncodeToString(body),
	})
	if requestError != "" {
		return errors.New(requestError)
	}
	raw, decodeErr := base64.StdEncoding.DecodeString(encoded)
	if decodeErr != nil {
		return decodeErr
	}
	if status >= 400 {
		return fmt.Errorf("Docker Engine 返回 HTTP %d：%s", status, strings.TrimSpace(string(raw)))
	}
	if output == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, output)
}

func registryAuthHeaders(auth string) map[string][]string {
	if auth == "" {
		return nil
	}
	return map[string][]string{"X-Registry-Auth": {auth}}
}

func agentServiceUpdatePath(serviceID string, version uint64, hasRegistryAuth bool) string {
	path := fmt.Sprintf("/services/%s/update?version=%d", url.PathEscape(serviceID), version)
	if !hasRegistryAuth {
		path += "&registryAuthFrom=spec"
	}
	return path
}

func dockerRegistryAuthHeader(image string) (string, error) {
	registry := imageRegistry(image)
	configDir := strings.TrimSpace(os.Getenv("DOCKER_CONFIG"))
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return missingRegistryAuth(image)
		}
		configDir = filepath.Join(home, ".docker")
	}
	configPath := filepath.Join(configDir, "config.json")
	info, err := os.Stat(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return missingRegistryAuth(image)
	}
	if err != nil {
		return "", fmt.Errorf("读取 Docker 登录配置失败：%w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", errors.New("Docker 登录配置必须是小于 1 MiB 的普通文件")
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("读取 Docker 登录配置失败：%w", err)
	}
	var config struct {
		Auths map[string]struct {
			Username      string `json:"username,omitempty"`
			Password      string `json:"password,omitempty"`
			Auth          string `json:"auth,omitempty"`
			Email         string `json:"email,omitempty"`
			IdentityToken string `json:"identitytoken,omitempty"`
			RegistryToken string `json:"registrytoken,omitempty"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", fmt.Errorf("Docker 登录配置格式无效：%w", err)
	}
	for server, credential := range config.Auths {
		if normalizeRegistryServer(server) != registry {
			continue
		}
		payload, err := json.Marshal(map[string]string{
			"username":      credential.Username,
			"password":      credential.Password,
			"auth":          credential.Auth,
			"email":         credential.Email,
			"serveraddress": registry,
			"identitytoken": credential.IdentityToken,
			"registrytoken": credential.RegistryToken,
		})
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(payload), nil
	}
	return missingRegistryAuth(image)
}

// missingRegistryAuth 在 Galaxy API 未下发仓库凭证、本机 Docker 也未登录目标
// 仓库时给出提示。只对当前配置的默认 Agent 镜像报错，因为该镜像本应由 API
// 按组织匹配到仓库凭证；显式指定的自定义或公共镜像直接放行，交由 Docker 处理。
func missingRegistryAuth(image string) (string, error) {
	configured := defaultAgentImage()
	if configured == "" || !strings.EqualFold(strings.TrimSpace(image), configured) {
		return "", nil
	}
	registry := strings.TrimPrefix(strings.TrimPrefix(imageRegistry(configured), "https://"), "http://")
	return "", fmt.Errorf(
		"默认 Agent 镜像 %s 未匹配到仓库凭证，本机 Docker 也未登录 %s；请在 Galaxy 管理中心配置该 Registry，或执行 docker login %s",
		configured, registry, registry,
	)
}

func imageRegistry(image string) string {
	first := strings.Split(strings.TrimSpace(image), "/")[0]
	if strings.ContainsAny(first, ".:") || first == "localhost" {
		return normalizeRegistryServer(first)
	}
	return "https://index.docker.io/v1/"
}

func normalizeRegistryServer(server string) string {
	server = strings.TrimSpace(strings.ToLower(server))
	server = strings.TrimPrefix(server, "https://")
	server = strings.TrimPrefix(server, "http://")
	server = strings.TrimSuffix(server, "/")
	server = strings.TrimSuffix(server, "/v1")
	if server == "index.docker.io" || server == "docker.io" {
		return "https://index.docker.io/v1/"
	}
	return server
}

func dockerFilteredPath(path, key, value string) string {
	filters, _ := json.Marshal(map[string][]string{key: {value}})
	return path + "?filters=" + url.QueryEscape(string(filters))
}

func (e *remoteExecutor) getHTTPClient() *http.Client {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.httpClient != nil {
		return e.httpClient
	}
	e.transport = &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return e.openDockerHTTPConnection()
		},
		ForceAttemptHTTP2:     false,
		DisableCompression:    true,
		MaxConnsPerHost:       4,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       5 * time.Minute,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	e.httpClient = &http.Client{Transport: e.transport, Timeout: 60 * time.Second}
	return e.httpClient
}

func (e *remoteExecutor) openDockerHTTPConnection() (net.Conn, error) {
	return dialLocalDockerSocket(e.remote)
}

func (e *remoteExecutor) openStream(request relayMessage) (*dockerStream, error) {
	return openLocalDockerStream(e.remote, request)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func validateAgentServerURL(raw string) error {
	serverURL, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || serverURL.Hostname() == "" {
		return errors.New("Galaxy API 地址无效")
	}
	if serverURL.Scheme != "http" && serverURL.Scheme != "https" {
		return errors.New("Galaxy API 地址必须使用 http 或 https")
	}
	hostname := strings.TrimSuffix(strings.ToLower(serverURL.Hostname()), ".")
	if hostname == "localhost" {
		return errors.New("Galaxy API 地址不能使用 localhost：Global Agent Service 容器无法通过回环地址访问宿主机")
	}
	if address := net.ParseIP(hostname); address != nil && address.IsLoopback() {
		return errors.New("Galaxy API 地址不能使用回环 IP：Global Agent Service 容器无法通过该地址访问管理中心")
	}
	return nil
}

func validateAgentImage(image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return errors.New("Agent 镜像不能为空")
	}
	if len(image) > 512 || strings.ContainsAny(image, " \t\r\n") || strings.Contains(image, "://") {
		return errors.New("Agent 镜像地址无效")
	}
	return nil
}

func (r remoteConfig) isLocal() bool { return strings.EqualFold(strings.TrimSpace(r.Mode), "local") }

func (r remoteConfig) socketPath() (string, error) {
	path := filepath.Clean(strings.TrimSpace(r.DockerSocket))
	if path == "." || !filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') {
		return "", errors.New("Docker Socket 必须是绝对路径")
	}
	return path, nil
}

func dialLocalDockerSocket(remote remoteConfig) (net.Conn, error) {
	path, err := remote.socketPath()
	if err != nil {
		return nil, err
	}
	return net.DialTimeout("unix", path, 5*time.Second)
}

func testLocalDockerManager(remote remoteConfig) error {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		path, err := remote.socketPath()
		if err != nil {
			return nil, err
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", path)
	}}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport, Timeout: 10 * time.Second}).Get("http://docker/info")
	if err != nil {
		return fmt.Errorf("无法访问本机 Docker Socket：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Docker API /info 返回 HTTP %d", response.StatusCode)
	}
	var info struct {
		Swarm struct {
			LocalNodeState   string `json:"LocalNodeState"`
			ControlAvailable bool   `json:"ControlAvailable"`
		} `json:"Swarm"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&info); err != nil {
		return fmt.Errorf("Docker API /info 响应无效：%w", err)
	}
	if !info.Swarm.ControlAvailable {
		return fmt.Errorf("当前 Docker Engine 不是可用的 Swarm Manager（state=%s）", info.Swarm.LocalNodeState)
	}
	return nil
}

type dockerStream struct {
	conn    net.Conn
	stdin   io.WriteCloser
	body    io.ReadCloser
	status  int
	once    sync.Once
	writeMu sync.Mutex
}

func openLocalDockerStream(remote remoteConfig, request relayMessage) (*dockerStream, error) {
	connection, err := dialLocalDockerSocket(remote)
	if err != nil {
		return nil, fmt.Errorf("连接本机 Docker Socket 失败：%w", err)
	}
	body, err := base64.StdEncoding.DecodeString(request.Body)
	if err != nil {
		connection.Close()
		return nil, errors.New("请求正文编码无效")
	}
	path := request.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	httpRequest, err := http.NewRequest(request.Method, "http://docker"+path, bytes.NewReader(body))
	if err != nil {
		connection.Close()
		return nil, err
	}
	httpRequest.Header = request.Headers
	if err := httpRequest.Write(connection); err != nil {
		connection.Close()
		return nil, err
	}
	reader := bufio.NewReader(connection)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("读取 Docker 流响应失败：%w", err)
	}
	fields := strings.Fields(statusLine)
	if len(fields) < 2 {
		connection.Close()
		return nil, fmt.Errorf("Docker 流响应状态行无效：%s", strings.TrimSpace(statusLine))
	}
	status, err := strconv.Atoi(fields[1])
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("Docker 流响应状态码无效：%s", fields[1])
	}
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			connection.Close()
			return nil, fmt.Errorf("读取 Docker 流响应头失败：%w", readErr)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	if status != http.StatusSwitchingProtocols && status != http.StatusOK {
		connection.Close()
		return nil, fmt.Errorf("Docker 流连接返回 HTTP %d", status)
	}
	return &dockerStream{conn: connection, stdin: connection, body: io.NopCloser(reader), status: status}, nil
}

func (s *dockerStream) pump(id string, writeMessage func(relayMessage) error) {
	defer s.close()
	buffer := make([]byte, 32*1024)
	for {
		n, err := s.body.Read(buffer)
		if n > 0 {
			_ = writeMessage(relayMessage{Type: "stream_data", ID: id, Data: base64.StdEncoding.EncodeToString(buffer[:n])})
		}
		if err != nil {
			_ = writeMessage(relayMessage{Type: "stream_closed", ID: id})
			return
		}
	}
}

func (s *dockerStream) write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.stdin.Write(data)
	return err
}
func (s *dockerStream) close() {
	s.once.Do(func() {
		if s.body != nil {
			_ = s.body.Close()
		}
		if s.stdin != nil {
			_ = s.stdin.Close()
		}
		if s.conn != nil {
			_ = s.conn.Close()
		}
	})
}

func loadAgentFile(path string) (agentFile, error) {
	var c agentFile
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = yaml.Unmarshal(data, &c)
	if err != nil {
		return c, err
	}
	if len(c.Connections) == 0 && c.LegacyToken != "" && len(c.LegacyRemote) > 0 {
		return c, errors.New("检测到已废弃的 SSH Agent 配置；请在 Swarm Manager 使用 galaxy-agent bootstrap 重新注册")
	}
	return c, nil
}
func saveAgentFile(path string, c agentFile) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
func hostnameOr(fallback string) string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return fallback
	}
	return name
}

func uniqueConnectionName(connections map[string]agentConnection, base string) string {
	if _, exists := connections[base]; !exists {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s-%d", base, index)
		if _, exists := connections[candidate]; !exists {
			return candidate
		}
	}
}

func selectConnectionKey(connections map[string]agentConnection, proposed string, connection agentConnection) (string, error) {
	matches := []string{}
	signature := connectionSignature(connection)
	for key, current := range connections {
		if (connection.Token != "" && current.Token == connection.Token) || connectionSignature(current) == signature {
			matches = append(matches, key)
		}
	}
	sort.Strings(matches)
	if len(matches) > 1 {
		return "", fmt.Errorf("检测到多个重复连接：%s；请清理 Agent 配置文件后重试", strings.Join(matches, ", "))
	}
	key := proposed
	if len(matches) == 1 {
		key = matches[0]
	}
	if _, exists := connections[key]; exists {
		confirmed := false
		message := fmt.Sprintf("检测到已有连接 %s 指向相同目标，是否使用新 Token 和配置覆盖？", key)
		if err := survey.AskOne(&survey.Confirm{Message: message, Default: true}, &confirmed); err != nil {
			return "", err
		}
		if !confirmed {
			return "", errors.New("已取消更新 Agent 连接")
		}
	}
	return key, nil
}

func validateAgentConnections(connections map[string]agentConnection) error {
	if len(connections) > 1 {
		return errors.New("一个 galaxy-agent 配置只能绑定一个 Swarm 集群")
	}
	tokens := map[string]string{}
	targets := map[string]string{}
	keys := make([]string, 0, len(connections))
	for key := range connections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		connection := connections[key]
		if strings.TrimSpace(connection.Token) == "" {
			return fmt.Errorf("连接 %s 缺少 Agent Token", key)
		}
		if previous, exists := tokens[connection.Token]; exists {
			return fmt.Errorf("连接 %s 与 %s 使用了相同 Token；一条 Token 只能对应一个集群", previous, key)
		}
		tokens[connection.Token] = key
		signature := connectionSignature(connection)
		if previous, exists := targets[signature]; exists {
			return fmt.Errorf("连接 %s 与 %s 指向相同 Docker 目标；请清理 Agent 配置文件后重试", previous, key)
		}
		targets[signature] = key
	}
	return nil
}

func connectionSignature(connection agentConnection) string {
	return "local:" + filepath.Clean(strings.TrimSpace(connection.Remote.DockerSocket))
}

func agentWebSocketURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/agent/connect"
	return u.String(), nil
}

func agentServerCandidates(base string, identity nodeIdentity) ([]string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if err := validateAgentServerURL(base); err != nil {
		return nil, err
	}
	candidates := []string{base}
	if identity.Role != "manager" {
		return candidates, nil
	}
	managerIP := net.ParseIP(strings.TrimSpace(identity.NodeAddr))
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	configuredIP := net.ParseIP(parsed.Hostname())
	if managerIP == nil || managerIP.IsLoopback() || managerIP.IsUnspecified() || configuredIP == nil || configuredIP.Equal(managerIP) {
		return candidates, nil
	}
	fallback := *parsed
	if port := parsed.Port(); port != "" {
		fallback.Host = net.JoinHostPort(managerIP.String(), port)
	} else if managerIP.To4() == nil {
		fallback.Host = "[" + managerIP.String() + "]"
	} else {
		fallback.Host = managerIP.String()
	}
	fallbackURL := strings.TrimRight(fallback.String(), "/")
	if err := validateAgentServerURL(fallbackURL); err == nil {
		candidates = append(candidates, fallbackURL)
	}
	return candidates, nil
}

func parseNodeIdentity(encodedInfo, expectedSwarmID string) (nodeIdentity, error) {
	raw, err := base64.StdEncoding.DecodeString(encodedInfo)
	if err != nil {
		return nodeIdentity{}, errors.New("Docker /info 返回内容编码无效")
	}
	var info struct {
		Name  string `json:"Name"`
		Swarm struct {
			NodeID           string `json:"NodeID"`
			NodeAddr         string `json:"NodeAddr"`
			LocalNodeState   string `json:"LocalNodeState"`
			ControlAvailable bool   `json:"ControlAvailable"`
			Nodes            int    `json:"Nodes"`
			Cluster          *struct {
				ID string `json:"ID"`
			} `json:"Cluster"`
		} `json:"Swarm"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return nodeIdentity{}, fmt.Errorf("解析 Docker /info 失败：%w", err)
	}
	nodeID := strings.TrimSpace(info.Swarm.NodeID)
	if nodeID == "" || strings.EqualFold(strings.TrimSpace(info.Swarm.LocalNodeState), "inactive") {
		return nodeIdentity{}, errors.New("本机尚未加入 Docker Swarm")
	}
	swarmID := ""
	if info.Swarm.Cluster != nil {
		swarmID = strings.TrimSpace(info.Swarm.Cluster.ID)
	}
	expectedSwarmID = strings.TrimSpace(expectedSwarmID)
	if swarmID == "" {
		swarmID = expectedSwarmID
	} else if expectedSwarmID != "" && swarmID != expectedSwarmID {
		return nodeIdentity{}, errors.New("Agent 配置的 Swarm ID 与本机 Docker Swarm 不一致")
	}
	if swarmID == "" {
		return nodeIdentity{}, errors.New("Worker 无法从 Docker /info 获取 Swarm ID，Agent Service 缺少 GALAXY_SWARM_ID")
	}
	role := "worker"
	if info.Swarm.ControlAvailable {
		role = "manager"
	}
	return nodeIdentity{
		SwarmID:    swarmID,
		NodeID:     nodeID,
		NodeAddr:   strings.TrimSpace(info.Swarm.NodeAddr),
		Hostname:   strings.TrimSpace(info.Name),
		Role:       role,
		SwarmNodes: info.Swarm.Nodes,
	}, nil
}

func readySwarmNodeCount(encodedNodes string) (int, error) {
	raw, err := base64.StdEncoding.DecodeString(encodedNodes)
	if err != nil {
		return 0, errors.New("Docker /nodes 返回内容编码无效")
	}
	var nodes []struct {
		Status struct {
			State string `json:"State"`
		} `json:"Status"`
	}
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return 0, fmt.Errorf("解析 Docker /nodes 失败：%w", err)
	}
	ready := 0
	for _, node := range nodes {
		if node.Status.State == "ready" {
			ready++
		}
	}
	if ready < 1 {
		return 0, errors.New("Swarm 没有 Ready 节点")
	}
	return ready, nil
}

func updateAgentBootstrapState(path, key, previous, next string, credentialVersion int, deployPending, disabled bool) error {
	agentConfigWriteLock.Lock()
	defer agentConfigWriteLock.Unlock()
	config, err := loadAgentFile(path)
	if err != nil {
		return err
	}
	connection, exists := config.Connections[key]
	if !exists {
		return fmt.Errorf("Agent 连接 %s 不存在", key)
	}
	if connection.Token != previous && connection.Token != next {
		return fmt.Errorf("Agent 连接 %s 的凭证已被其他进程修改", key)
	}
	connection.Token = next
	connection.CredentialVersion = credentialVersion
	connection.DeployPending = deployPending
	connection.Disabled = disabled
	config.Connections[key] = connection
	return saveAgentFile(path, config)
}

func environmentAgentConnection() (*agentConnection, string, error) {
	credentialFile := strings.TrimSpace(os.Getenv("GALAXY_AGENT_CREDENTIAL_FILE"))
	if credentialFile == "" {
		return nil, "", nil
	}
	credential, err := os.ReadFile(credentialFile)
	if err != nil {
		return nil, "", fmt.Errorf("读取 Agent Credential 失败：%w", err)
	}
	token := strings.TrimSpace(string(credential))
	if token == "" {
		return nil, "", errors.New("Agent Credential 为空")
	}
	hostname := hostnameOr("galaxy-agent")
	socket := strings.TrimSpace(os.Getenv("GALAXY_DOCKER_SOCKET"))
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	version, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("GALAXY_AGENT_CREDENTIAL_VERSION")))
	return &agentConnection{
		Name:              hostname,
		Token:             token,
		Target:            hostname,
		CredentialVersion: version,
		Remote: remoteConfig{
			Mode:         "local",
			DockerSocket: socket,
		},
	}, strings.TrimSpace(os.Getenv("GALAXY_API_URL")), nil
}

// defaultAgentImage 返回 Agent 镜像，优先级为编译期通过 -ldflags 注入的
// buildVariable.AgentImage、环境变量 GALAXY_AGENT_IMAGE，最后为空。
// 发行版不内置任何镜像仓库地址，未配置时必须通过 --image 显式指定。
func defaultAgentImage() string {
	if image := strings.TrimSpace(buildVariable.AgentImage); image != "" {
		return image
	}
	return strings.TrimSpace(os.Getenv(envAgentImage))
}

// requireAgentImage 校验 Agent 镜像已配置，供安装和本地配置流程快速失败。
func requireAgentImage(image string) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("未指定 Agent 镜像：请使用 --image 指定，或设置环境变量 %s", envAgentImage)
	}
	return nil
}
