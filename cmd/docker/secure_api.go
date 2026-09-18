package docker

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/kballard/go-shellquote"
	"github.com/spf13/cobra"
)

const (
	defaultDockerConfigPath = "/etc/docker/daemon.json"
	defaultCertificateDir   = "/etc/docker/galaxy-tls"
	defaultSystemdDropIn    = "/etc/systemd/system/docker.service.d/galaxy-tls.conf"
	defaultCertificateDays  = 3650
)

var dnsNamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

type secureAPIOptions struct {
	advertiseHost  string
	listenAddress  string
	certificateDir string
	dockerConfig   string
	days           int
	yes            bool
	force          bool
	noRestart      bool

	ioStreams galaxycfg.IOStreams
	runner    commandRunner
}

type commandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type osCommandRunner struct{}

func (osCommandRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

type configurationBackup struct {
	root                string
	dockerConfigExisted bool
	dropInExisted       bool
	certificateExisted  bool
}

func newCmdSecureAPI(ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := &secureAPIOptions{
		listenAddress:  "0.0.0.0",
		certificateDir: defaultCertificateDir,
		dockerConfig:   defaultDockerConfigPath,
		days:           defaultCertificateDays,
		ioStreams:      ioStreams,
		runner:         osCommandRunner{},
	}

	cmd := &cobra.Command{
		Use:     "secure-api",
		Aliases: []string{"tls"},
		Short:   "为 Docker API 自动启用双向 TLS",
		Long: "生成 CA、Server、Client 证书，安全修改 Docker daemon 配置，重启并验证 2376。\n" +
			"命令会移除 daemon.json 中的 2375 监听，并在修改前创建完整备份。",
		Example: "  sudo galaxy docker secure-api\n" +
			"  sudo galaxy docker secure-api --advertise-host 192.168.1.10 --yes",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.run())
		},
		SilenceUsage: true,
	}

	flags := cmd.Flags()
	flags.StringVar(&o.advertiseHost, "advertise-host", "", "Galaxy API 连接 Manager 时使用的 IP 或 DNS 名称")
	flags.StringVar(&o.listenAddress, "listen-address", o.listenAddress, "Docker API 监听地址")
	flags.StringVar(&o.certificateDir, "cert-dir", o.certificateDir, "证书保存目录")
	flags.StringVar(&o.dockerConfig, "docker-config", o.dockerConfig, "Docker daemon.json 路径")
	flags.IntVar(&o.days, "days", o.days, "证书有效天数")
	flags.BoolVarP(&o.yes, "yes", "y", false, "接受推荐值并跳过最终确认")
	flags.BoolVar(&o.force, "force", false, "替换已有 Galaxy TLS 证书")
	flags.BoolVar(&o.noRestart, "no-restart", false, "只写入配置，不重启和验证 Docker")
	return cmd
}

func (o *secureAPIOptions) run() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("当前只支持使用 systemd 的 Linux 主机，检测到系统：%s", runtime.GOOS)
	}
	if os.Geteuid() != 0 {
		return errors.New("需要 root 权限修改 Docker 配置，请运行：sudo galaxy docker secure-api")
	}
	if _, err := exec.LookPath("openssl"); err != nil {
		return errors.New("未找到 openssl，请先通过系统包管理器安装 OpenSSL")
	}
	if _, err := exec.LookPath("dockerd"); err != nil {
		return errors.New("未找到 dockerd，请确认 Docker Engine 已安装")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("未找到 systemctl，当前命令只支持 systemd 管理的 Docker Engine")
	}

	o.printHeader()
	if err := o.collectAnswers(); err != nil {
		return err
	}
	if err := o.validate(); err != nil {
		return err
	}
	if err := o.confirm(); err != nil {
		return err
	}

	return o.apply()
}

func (o *secureAPIOptions) printHeader() {
	_, _ = fmt.Fprintln(o.ioStreams.Out, color.CyanString("\nDocker API 双向 TLS 配置向导"))
	_, _ = fmt.Fprintln(o.ioStreams.Out, "────────────────────────────────────────")
	_, _ = fmt.Fprintln(o.ioStreams.Out, "此向导将生成证书、备份现有配置、关闭 2375，并启用 2376 双向 TLS。")
	_, _ = fmt.Fprintln(o.ioStreams.Out, color.YellowString("Docker 将短暂重启，请先确认当前主机允许维护操作。"))
}

func (o *secureAPIOptions) collectAnswers() error {
	if o.advertiseHost == "" {
		candidates := hostCandidates()
		if o.yes {
			o.advertiseHost = candidates[0]
		} else {
			if err := survey.AskOne(&survey.Select{
				Message: "Galaxy API 应使用哪个地址连接这台 Manager？",
				Options: candidates,
				Default: candidates[0],
			}, &o.advertiseHost); err != nil {
				return err
			}
		}
	}

	if o.yes {
		return nil
	}

	questions := []*survey.Question{
		{
			Name: "listenAddress",
			Prompt: &survey.Input{
				Message: "Docker API 监听地址：",
				Default: o.listenAddress,
			},
		},
		{
			Name: "certificateDir",
			Prompt: &survey.Input{
				Message: "证书保存目录：",
				Default: o.certificateDir,
			},
		},
		{
			Name: "days",
			Prompt: &survey.Input{
				Message: "证书有效天数：",
				Default: strconv.Itoa(o.days),
			},
		},
	}
	answers := struct {
		ListenAddress  string
		CertificateDir string
		Days           string
	}{}
	if err := survey.Ask(questions, &answers); err != nil {
		return err
	}
	o.listenAddress = strings.TrimSpace(answers.ListenAddress)
	o.certificateDir = strings.TrimSpace(answers.CertificateDir)
	days, err := strconv.Atoi(strings.TrimSpace(answers.Days))
	if err != nil {
		return errors.New("证书有效天数必须是整数")
	}
	o.days = days
	return nil
}

func (o *secureAPIOptions) validate() error {
	o.advertiseHost = strings.TrimSpace(o.advertiseHost)
	if !validHost(o.advertiseHost) {
		return fmt.Errorf("无效的连接地址：%s", o.advertiseHost)
	}
	if net.ParseIP(o.listenAddress) == nil {
		return fmt.Errorf("监听地址必须是 IP：%s", o.listenAddress)
	}
	if o.days < 1 || o.days > 36500 {
		return errors.New("证书有效天数必须在 1 到 36500 之间")
	}
	if !filepath.IsAbs(o.certificateDir) || !filepath.IsAbs(o.dockerConfig) {
		return errors.New("证书目录和 Docker 配置文件必须使用绝对路径")
	}
	o.certificateDir = filepath.Clean(o.certificateDir)
	o.dockerConfig = filepath.Clean(o.dockerConfig)
	if o.certificateDir == "/" || pathContains(o.certificateDir, o.dockerConfig) || pathContains(o.certificateDir, defaultSystemdDropIn) {
		return errors.New("证书目录不能是系统根目录，也不能包含 Docker 或 systemd 配置文件")
	}
	backupParent := filepath.Join(filepath.Dir(o.dockerConfig), "galaxy-backup")
	if pathContains(o.certificateDir, backupParent) || pathContains(backupParent, o.certificateDir) {
		return errors.New("证书目录不能与 Galaxy Docker 配置备份目录重叠")
	}
	if info, err := os.Stat(o.certificateDir); err == nil && info.IsDir() && !o.force {
		if o.yes {
			return fmt.Errorf("证书目录已存在：%s；如需替换请增加 --force", o.certificateDir)
		}
		var replace bool
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("证书目录 %s 已存在，备份后重新生成？", o.certificateDir),
			Default: false,
		}, &replace); err != nil {
			return err
		}
		if !replace {
			return errors.New("操作已取消，现有证书和 Docker 配置未修改")
		}
		o.force = true
	}
	return nil
}

func (o *secureAPIOptions) confirm() error {
	_, _ = fmt.Fprintln(o.ioStreams.Out, "\n配置摘要")
	_, _ = fmt.Fprintf(o.ioStreams.Out, "  Galaxy API 地址 : https://%s:2376\n", formatHost(o.advertiseHost))
	_, _ = fmt.Fprintf(o.ioStreams.Out, "  Docker 监听     : tcp://%s:2376\n", formatHost(o.listenAddress))
	_, _ = fmt.Fprintf(o.ioStreams.Out, "  服务端证书目录  : %s\n", o.certificateDir)
	_, _ = fmt.Fprintf(o.ioStreams.Out, "  客户端证书包    : %s\n", filepath.Join(o.certificateDir, "client"))
	_, _ = fmt.Fprintf(o.ioStreams.Out, "  有效期          : %d 天\n", o.days)
	_, _ = fmt.Fprintf(o.ioStreams.Out, "  Docker 配置     : %s\n", o.dockerConfig)
	if o.yes {
		return nil
	}
	var confirmed bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "确认生成证书并修改 Docker 配置？",
		Default: false,
	}, &confirmed); err != nil {
		return err
	}
	if !confirmed {
		return errors.New("操作已取消，未修改任何配置")
	}
	return nil
}

func (o *secureAPIOptions) apply() error {
	_, _ = fmt.Fprintln(o.ioStreams.Out, "\n开始配置：")
	if err := os.MkdirAll(filepath.Dir(o.certificateDir), 0o755); err != nil {
		return fmt.Errorf("创建证书父目录失败：%w", err)
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(o.certificateDir), ".galaxy-tls-")
	if err != nil {
		return fmt.Errorf("创建证书临时目录失败：%w", err)
	}
	defer os.RemoveAll(stagingDir)

	o.step(1, 6, "使用 OpenSSL 生成 CA、Server 和 Client 证书")
	if err := o.generateCertificates(stagingDir); err != nil {
		return err
	}

	o.step(2, 6, "备份现有 Docker 配置和证书")
	backup, err := o.createBackup()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.ioStreams.Out, "      备份目录：%s\n", backup.root)

	o.step(3, 6, "安装证书并设置私钥权限")
	if err := o.installCertificates(stagingDir); err != nil {
		_ = o.restore(backup)
		return err
	}

	o.step(4, 6, "合并 daemon.json 并移除不安全的 2375 监听")
	if err := o.configureDockerDaemon(); err != nil {
		_ = o.restore(backup)
		return err
	}
	if err := o.configureSystemdOverride(); err != nil {
		_ = o.restore(backup)
		return err
	}

	if o.noRestart {
		o.step(5, 6, "已按要求跳过 Docker 重启")
		o.step(6, 6, "配置写入完成，尚未验证")
		o.printSuccess(backup.root, false)
		return nil
	}

	o.step(5, 6, "重新加载 systemd 并重启 Docker")
	if err := o.restartDocker(); err != nil {
		rollbackErr := o.rollbackAndRestart(backup)
		if rollbackErr != nil {
			return fmt.Errorf("Docker 重启失败：%v；自动回滚也失败：%v；备份位于 %s", err, rollbackErr, backup.root)
		}
		return fmt.Errorf("Docker 重启失败，已自动恢复原配置：%w", err)
	}

	o.step(6, 6, "使用 Client 证书验证 https://127.0.0.1:2376/_ping")
	if err := o.verifyDockerAPI(); err != nil {
		rollbackErr := o.rollbackAndRestart(backup)
		if rollbackErr != nil {
			return fmt.Errorf("TLS 验证失败：%v；自动回滚也失败：%v；备份位于 %s", err, rollbackErr, backup.root)
		}
		return fmt.Errorf("TLS 验证失败，已自动恢复原配置：%w", err)
	}

	o.printSuccess(backup.root, true)
	return nil
}

func (o *secureAPIOptions) step(current, total int, message string) {
	_, _ = fmt.Fprintf(o.ioStreams.Out, "  %s %s\n", color.CyanString("[%d/%d]", current, total), message)
}

func (o *secureAPIOptions) generateCertificates(dir string) error {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "docker-manager"
	}
	serverSANs := []string{"IP:127.0.0.1", "DNS:localhost"}
	if ip := net.ParseIP(o.advertiseHost); ip != nil {
		serverSANs = append(serverSANs, "IP:"+ip.String())
	} else {
		serverSANs = append(serverSANs, "DNS:"+o.advertiseHost)
	}
	if validHost(hostname) {
		serverSANs = append(serverSANs, "DNS:"+hostname)
	}
	serverSANs = uniqueStrings(serverSANs)

	serverExtensions := "subjectAltName=" + strings.Join(serverSANs, ",") + "\n" +
		"extendedKeyUsage=serverAuth\nkeyUsage=digitalSignature,keyEncipherment\n"
	clientExtensions := "extendedKeyUsage=clientAuth\nkeyUsage=digitalSignature,keyEncipherment\n"
	if err := os.WriteFile(filepath.Join(dir, "server-ext.cnf"), []byte(serverExtensions), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "client-ext.cnf"), []byte(clientExtensions), 0o600); err != nil {
		return err
	}

	commands := [][]string{
		{"genrsa", "-out", filepath.Join(dir, "ca-key.pem"), "4096"},
		{"req", "-new", "-x509", "-days", strconv.Itoa(o.days), "-sha256", "-key", filepath.Join(dir, "ca-key.pem"), "-out", filepath.Join(dir, "ca.pem"), "-subj", "/CN=Galaxy Docker CA", "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign"},
		{"genrsa", "-out", filepath.Join(dir, "server-key.pem"), "4096"},
		{"req", "-new", "-sha256", "-key", filepath.Join(dir, "server-key.pem"), "-out", filepath.Join(dir, "server.csr"), "-subj", "/CN=" + sanitizeCommonName(o.advertiseHost)},
		{"x509", "-req", "-days", strconv.Itoa(o.days), "-sha256", "-in", filepath.Join(dir, "server.csr"), "-CA", filepath.Join(dir, "ca.pem"), "-CAkey", filepath.Join(dir, "ca-key.pem"), "-CAcreateserial", "-out", filepath.Join(dir, "server-cert.pem"), "-extfile", filepath.Join(dir, "server-ext.cnf")},
		{"genrsa", "-out", filepath.Join(dir, "client-key.pem"), "4096"},
		{"req", "-new", "-sha256", "-key", filepath.Join(dir, "client-key.pem"), "-out", filepath.Join(dir, "client.csr"), "-subj", "/CN=Galaxy API Docker Client"},
		{"x509", "-req", "-days", strconv.Itoa(o.days), "-sha256", "-in", filepath.Join(dir, "client.csr"), "-CA", filepath.Join(dir, "ca.pem"), "-CAkey", filepath.Join(dir, "ca-key.pem"), "-CAcreateserial", "-out", filepath.Join(dir, "client-cert.pem"), "-extfile", filepath.Join(dir, "client-ext.cnf")},
		{"verify", "-CAfile", filepath.Join(dir, "ca.pem"), filepath.Join(dir, "server-cert.pem")},
		{"verify", "-CAfile", filepath.Join(dir, "ca.pem"), filepath.Join(dir, "client-cert.pem")},
	}
	for _, args := range commands {
		output, err := o.runner.Run("openssl", args...)
		if err != nil {
			return fmt.Errorf("OpenSSL 执行失败（openssl %s）：%w\n%s", args[0], err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func (o *secureAPIOptions) createBackup() (*configurationBackup, error) {
	timestamp := fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), os.Getpid())
	root := filepath.Join(filepath.Dir(o.dockerConfig), "galaxy-backup", timestamp)
	backup := &configurationBackup{root: root}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("创建备份目录失败：%w", err)
	}

	if fileExists(o.dockerConfig) {
		backup.dockerConfigExisted = true
		if err := copyFile(o.dockerConfig, filepath.Join(root, "daemon.json"), 0o600); err != nil {
			return nil, fmt.Errorf("备份 daemon.json 失败：%w", err)
		}
	}
	if fileExists(defaultSystemdDropIn) {
		backup.dropInExisted = true
		if err := copyFile(defaultSystemdDropIn, filepath.Join(root, "galaxy-tls.conf"), 0o600); err != nil {
			return nil, fmt.Errorf("备份 systemd 配置失败：%w", err)
		}
	}
	if directoryExists(o.certificateDir) {
		backup.certificateExisted = true
		if err := copyDirectory(o.certificateDir, filepath.Join(root, "certificates")); err != nil {
			return nil, fmt.Errorf("备份证书失败：%w", err)
		}
	}
	return backup, nil
}

func (o *secureAPIOptions) installCertificates(stagingDir string) error {
	if err := os.RemoveAll(o.certificateDir); err != nil {
		return fmt.Errorf("清理旧证书目录失败：%w", err)
	}
	if err := os.MkdirAll(filepath.Join(o.certificateDir, "client"), 0o700); err != nil {
		return fmt.Errorf("创建证书目录失败：%w", err)
	}

	serverFiles := map[string]os.FileMode{
		"ca.pem":          0o644,
		"ca-key.pem":      0o600,
		"server-cert.pem": 0o644,
		"server-key.pem":  0o600,
	}
	for name, mode := range serverFiles {
		if err := copyFile(filepath.Join(stagingDir, name), filepath.Join(o.certificateDir, name), mode); err != nil {
			return err
		}
	}
	clientFiles := map[string]string{
		"ca.pem":          "ca.pem",
		"client-cert.pem": "cert.pem",
		"client-key.pem":  "key.pem",
	}
	for source, destination := range clientFiles {
		mode := os.FileMode(0o644)
		if destination == "key.pem" {
			mode = 0o600
		}
		if err := copyFile(filepath.Join(stagingDir, source), filepath.Join(o.certificateDir, "client", destination), mode); err != nil {
			return err
		}
	}
	return nil
}

func (o *secureAPIOptions) configureDockerDaemon() error {
	config := map[string]any{}
	if data, err := os.ReadFile(o.dockerConfig); err == nil {
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("现有 daemon.json 不是有效 JSON，已停止修改：%w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取 daemon.json 失败：%w", err)
	}

	hosts := stringSlice(config["hosts"])
	filtered := make([]string, 0, len(hosts)+2)
	for _, host := range hosts {
		if isInsecureDockerTCP(host) || isTLS2376Host(host) || host == "unix:///var/run/docker.sock" {
			continue
		}
		filtered = append(filtered, host)
	}
	filtered = append(filtered, "unix:///var/run/docker.sock", "tcp://"+formatHost(o.listenAddress)+":2376")
	config["hosts"] = uniqueStrings(filtered)
	config["tls"] = true
	config["tlsverify"] = true
	config["tlscacert"] = filepath.Join(o.certificateDir, "ca.pem")
	config["tlscert"] = filepath.Join(o.certificateDir, "server-cert.pem")
	config["tlskey"] = filepath.Join(o.certificateDir, "server-key.pem")

	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(o.dockerConfig), 0o755); err != nil {
		return err
	}
	if err := writeAtomicFile(o.dockerConfig, encoded, 0o600); err != nil {
		return err
	}
	output, err := o.runner.Run("dockerd", "--validate", "--config-file", o.dockerConfig)
	if err != nil {
		return fmt.Errorf("Docker 配置校验失败：%w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (o *secureAPIOptions) configureSystemdOverride() error {
	output, err := o.runner.Run("systemctl", "cat", "docker.service")
	if err != nil {
		return fmt.Errorf("无法读取 docker.service：%w\n%s", err, strings.TrimSpace(string(output)))
	}
	execStart := activeExecStart(string(output))
	if execStart == "" {
		return errors.New("无法识别 docker.service 的 ExecStart，未修改 systemd 配置")
	}
	if !containsHostFlag(execStart) && !fileExists(defaultSystemdDropIn) {
		return nil
	}

	cleaned, err := removeDockerListenerFlags(execStart)
	if err != nil {
		return err
	}
	content := "# Generated by galaxy docker secure-api\n[Service]\nExecStart=\nExecStart=" + cleaned + "\n"
	if err := os.MkdirAll(filepath.Dir(defaultSystemdDropIn), 0o755); err != nil {
		return err
	}
	return writeAtomicFile(defaultSystemdDropIn, []byte(content), 0o644)
}

func (o *secureAPIOptions) restartDocker() error {
	if output, err := o.runner.Run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败：%w\n%s", err, strings.TrimSpace(string(output)))
	}
	if output, err := o.runner.Run("systemctl", "restart", "docker.service"); err != nil {
		status, _ := o.runner.Run("systemctl", "status", "docker.service", "--no-pager", "--lines=20")
		return fmt.Errorf("systemctl restart docker 失败：%w\n%s\n%s", err, strings.TrimSpace(string(output)), strings.TrimSpace(string(status)))
	}
	return nil
}

func (o *secureAPIOptions) verifyDockerAPI() error {
	caPEM, err := os.ReadFile(filepath.Join(o.certificateDir, "client", "ca.pem"))
	if err != nil {
		return err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return errors.New("无法解析客户端 CA 证书")
	}
	certificate, err := tls.LoadX509KeyPair(
		filepath.Join(o.certificateDir, "client", "cert.pem"),
		filepath.Join(o.certificateDir, "client", "key.pem"),
	)
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			RootCAs:      caPool,
			Certificates: []tls.Certificate{certificate},
			ServerName:   o.advertiseHost,
		}},
	}
	response, err := client.Get("https://127.0.0.1:2376/_ping")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	if response.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "OK" {
		return fmt.Errorf("Docker API 返回 %s：%s", response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (o *secureAPIOptions) rollbackAndRestart(backup *configurationBackup) error {
	if err := o.restore(backup); err != nil {
		return err
	}
	if output, err := o.runner.Run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("回滚后 daemon-reload 失败：%w\n%s", err, strings.TrimSpace(string(output)))
	}
	if output, err := o.runner.Run("systemctl", "restart", "docker.service"); err != nil {
		return fmt.Errorf("回滚后 Docker 重启失败：%w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (o *secureAPIOptions) restore(backup *configurationBackup) error {
	if backup.dockerConfigExisted {
		if err := copyFile(filepath.Join(backup.root, "daemon.json"), o.dockerConfig, 0o600); err != nil {
			return err
		}
	} else if err := os.Remove(o.dockerConfig); err != nil && !os.IsNotExist(err) {
		return err
	}

	if backup.dropInExisted {
		if err := copyFile(filepath.Join(backup.root, "galaxy-tls.conf"), defaultSystemdDropIn, 0o644); err != nil {
			return err
		}
	} else if err := os.Remove(defaultSystemdDropIn); err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := os.RemoveAll(o.certificateDir); err != nil {
		return err
	}
	if backup.certificateExisted {
		if err := copyDirectory(filepath.Join(backup.root, "certificates"), o.certificateDir); err != nil {
			return err
		}
	}
	return nil
}

func (o *secureAPIOptions) printSuccess(backupDir string, verified bool) {
	_, _ = fmt.Fprintln(o.ioStreams.Out, color.GreenString("\n✓ Docker API 双向 TLS 配置完成"))
	if verified {
		_, _ = fmt.Fprintln(o.ioStreams.Out, color.GreenString("✓ Client 证书认证和 2376 API 响应验证通过"))
	}
	_, _ = fmt.Fprintf(o.ioStreams.Out, "\n请在 Galaxy 集群配置中使用：https://%s:2376\n", formatHost(o.advertiseHost))
	_, _ = fmt.Fprintf(o.ioStreams.Out, "需要导入 Galaxy API 的客户端证书：%s\n", filepath.Join(o.certificateDir, "client"))
	_, _ = fmt.Fprintf(o.ioStreams.Out, "原配置备份：%s\n", backupDir)
	_, _ = fmt.Fprintln(o.ioStreams.Out, color.YellowString("请使用防火墙限制 2376，仅允许 Galaxy API 服务器访问。不要复制 ca-key.pem。"))
}

func hostCandidates() []string {
	result := make([]string, 0)
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "br-") || strings.HasPrefix(iface.Name, "veth") {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && !ip.IsLoopback() {
				result = append(result, ip.String())
			}
		}
	}
	if hostname, err := os.Hostname(); err == nil && validHost(hostname) {
		result = append(result, hostname)
	}
	result = uniqueStrings(result)
	if len(result) == 0 {
		return []string{"127.0.0.1"}
	}
	return result
}

func validHost(value string) bool {
	if value == "" || strings.ContainsAny(value, "\r\n,/") {
		return false
	}
	return net.ParseIP(value) != nil || dnsNamePattern.MatchString(value)
}

func sanitizeCommonName(value string) string {
	return strings.NewReplacer("/", "-", "\\", "-", "=", "-", ",", "-").Replace(value)
}

func formatHost(value string) string {
	if ip := net.ParseIP(value); ip != nil && strings.Contains(value, ":") {
		return "[" + ip.String() + "]"
	}
	return value
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if values, ok := value.([]string); ok {
			return values
		}
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func isInsecureDockerTCP(host string) bool {
	return strings.HasPrefix(host, "tcp://") && strings.HasSuffix(host, ":2375")
}

func isTLS2376Host(host string) bool {
	return strings.HasPrefix(host, "tcp://") && strings.HasSuffix(host, ":2376")
}

func activeExecStart(unit string) string {
	lines := strings.Split(strings.ReplaceAll(unit, "\\\n", ""), "\n")
	result := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ExecStart=") && line != "ExecStart=" {
			result = strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		}
	}
	return result
}

func containsHostFlag(command string) bool {
	fields, err := shellquote.Split(command)
	if err != nil {
		return true
	}
	for _, field := range fields {
		if field == "-H" || field == "--host" || strings.HasPrefix(field, "-H=") || strings.HasPrefix(field, "--host=") {
			return true
		}
	}
	return false
}

func removeDockerListenerFlags(command string) (string, error) {
	fields, err := shellquote.Split(command)
	if err != nil {
		return "", fmt.Errorf("无法解析 Docker ExecStart：%w", err)
	}
	if len(fields) == 0 || !strings.Contains(filepath.Base(fields[0]), "dockerd") {
		return "", fmt.Errorf("无法安全修改 Docker ExecStart：%s", command)
	}
	result := make([]string, 0, len(fields))
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		if field == "-H" || field == "--host" || field == "--tlscacert" || field == "--tlscert" || field == "--tlskey" {
			index++
			continue
		}
		if field == "--tls" || field == "--tlsverify" || strings.HasPrefix(field, "-H=") || strings.HasPrefix(field, "--host=") ||
			strings.HasPrefix(field, "--tlscacert=") || strings.HasPrefix(field, "--tlscert=") || strings.HasPrefix(field, "--tlskey=") {
			continue
		}
		result = append(result, field)
	}
	if len(result) == 0 {
		return "", errors.New("清理 Docker ExecStart 参数后没有可执行命令")
	}
	return shellquote.Join(result...), nil
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".galaxy-config-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func copyFile(source, destination string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	return writeAtomicFile(destination, data, mode)
}

func copyDirectory(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
