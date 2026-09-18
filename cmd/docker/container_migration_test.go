package docker

import (
	"bytes"
	"galaxy/pkg/galaxycfg"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestContainerMigrateIsTheDefaultExecutionCommand(t *testing.T) {
	command := newCmdContainerMigration(galaxycfg.IOStreams{})

	migrate, _, err := command.Find([]string{"migrate"})
	require.NoError(t, err)
	require.NotNil(t, migrate.RunE)
	require.NotNil(t, migrate.Flags().Lookup("yes"))
	require.NotNil(t, migrate.Flags().Lookup("wait"))
	require.NotNil(t, migrate.Flags().Lookup("source-hash"))
	require.False(t, migrate.HasSubCommands())
}

func TestConfirmStandaloneContainerCleanupRequiresIndependentFullYes(t *testing.T) {
	var output bytes.Buffer
	cleanup, err := confirmStandaloneContainerCleanup(
		strings.NewReader("yes\n"), &output, "redis", "0123456789abcdef",
	)

	require.NoError(t, err)
	require.True(t, cleanup)
	require.Contains(t, output.String(), "迁移后清理")
	require.Contains(t, output.String(), "0123456789ab")
	require.Contains(t, output.String(), "Volume")

	output.Reset()
	cleanup, err = confirmStandaloneContainerCleanup(
		strings.NewReader("no\n"), &output, "redis", "0123456789abcdef",
	)
	require.NoError(t, err)
	require.False(t, cleanup)
	require.Contains(t, output.String(), "旧容器已保留")
}

func TestParseStandaloneContainersFiltersComposeAndSwarmTasks(t *testing.T) {
	rows, err := parseStandaloneContainers([]byte(
		"{\"ID\":\"ccc\",\"Names\":\"worker\",\"Image\":\"worker:1\",\"Status\":\"Up 1 hour\",\"Labels\":\"team=core\"}\n" +
			"{\"ID\":\"aaa\",\"Names\":\"api\",\"Image\":\"api:1\",\"Status\":\"Up 2 hours\",\"Labels\":\"com.docker.compose.project=demo\"}\n" +
			"{\"ID\":\"bbb\",\"Names\":\"task\",\"Image\":\"task:1\",\"Status\":\"Up 3 hours\",\"Labels\":\"com.docker.swarm.service.name=demo_task\"}\n",
	))

	require.NoError(t, err)
	require.Equal(t, []standaloneContainer{{
		ID: "ccc", Name: "worker", Image: "worker:1", Status: "Up 1 hour",
	}}, rows)
}

func TestGenerateContainerStackPreservesRuntimeSettings(t *testing.T) {
	inspect := &containerInspect{}
	inspect.Name = "/api"
	inspect.Config.Image = "registry.example.com/api:1"
	inspect.Config.Cmd = []string{"serve", "--port", "8080"}
	inspect.Config.Env = []string{"APP_ENV=prod"}
	inspect.Config.Labels = map[string]string{
		"team":                           "core",
		"com.docker.compose.project":     "old",
		"com.docker.swarm.service.name":  "old_api",
		"com.docker.compose.config-hash": "ignored",
	}
	inspect.HostConfig.PortBindings = map[string][]containerPort{
		"8080/tcp": {{HostIP: "127.0.0.1", HostPort: "18080"}},
	}
	inspect.HostConfig.RestartPolicy = containerRestartPolicy{Name: "on-failure", MaximumRetryCount: 3}
	inspect.Mounts = []containerMount{
		{Type: "bind", Source: "/srv/api", Destination: "/app/data", RW: true},
		{Type: "volume", Name: "api-data", Source: "/var/lib/docker/volumes/api-data/_data", Destination: "/data"},
	}

	generated := generateContainerStack(inspect, "api", "manager-1")

	require.Empty(t, generated.Failures)
	require.Contains(t, generated.Warnings, "绑定挂载 /srv/api 依赖原节点本地路径")
	require.Contains(t, generated.Warnings, "端口 18080 原先只绑定 127.0.0.1；Swarm host 发布模式会监听节点全部地址")

	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(generated.Content), &document))
	service := stringMap(stringMap(document["services"])["api"])
	require.Equal(t, "registry.example.com/api:1", service["image"])
	require.Equal(t, []any{"serve", "--port", "8080"}, service["command"])
	require.Equal(t, map[string]any{"team": "core"}, stringMap(service["labels"]))
	deploy := stringMap(service["deploy"])
	require.Equal(t, 1, deploy["replicas"])
	placement := stringMap(deploy["placement"])
	require.Equal(t, []any{"node.hostname == manager-1"}, placement["constraints"])
	require.Equal(t, map[string]any{"external": true}, stringMap(stringMap(document["volumes"])["api-data"]))
}

func TestContainerNoRestartPolicyBecomesLongRunningSwarmPolicy(t *testing.T) {
	deploy := map[string]any{}

	addContainerRestartPolicy(deploy, containerRestartPolicy{Name: "no"})

	require.Equal(t, map[string]any{"condition": "any"}, deploy["restart_policy"])
}

func TestGenerateContainerStackBlocksUnsafeContainerModes(t *testing.T) {
	inspect := &containerInspect{}
	inspect.Config.Image = "demo:latest"
	inspect.HostConfig.Privileged = true
	inspect.HostConfig.AutoRemove = true
	inspect.HostConfig.NetworkMode = "host"
	inspect.HostConfig.Devices = []map[string]any{{"PathOnHost": "/dev/kvm"}}

	generated := generateContainerStack(inspect, "demo", "manager-1")

	require.Contains(t, generated.Failures, "privileged 容器不能等价转换为 Swarm Service")
	require.Contains(t, generated.Failures, "容器启用了 --rm；停止时会被自动删除，无法安全回滚")
	require.Contains(t, generated.Failures, "容器使用不兼容的网络模式 host")
	require.Contains(t, generated.Failures, "容器使用了 --device；请迁移后手工设计 Swarm 设备约束")
}

func TestSwarmSafeName(t *testing.T) {
	require.Equal(t, "my-container", swarmSafeName("My Container"))
	require.Equal(t, "migrated-container", swarmSafeName("---"))
}
