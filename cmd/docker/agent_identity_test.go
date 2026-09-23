package docker

import (
	"encoding/base64"
	"encoding/json"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentInstallCommandIsAvailableFromGalaxyCLI(t *testing.T) {
	command := NewCmdAgentInstall(galaxycfg.NewConfigFlags(), galaxycfg.IOStreams{})

	install, _, err := command.Find([]string{"install"})
	require.NoError(t, err)
	require.NotNil(t, install.RunE)
	require.NotNil(t, install.Flags().Lookup("bootstrap-token-file"))
	require.NotNil(t, install.Flags().Lookup("bootstrap-token"))
	require.NotNil(t, install.Flags().Lookup("image"))
	require.NotNil(t, install.Flags().Lookup("docker-socket"))
	set, _, err := command.Find([]string{"set"})
	require.NoError(t, err)
	require.NotNil(t, set.RunE)
	require.NotNil(t, set.Flags().Lookup("docker-socket"))
	require.NotNil(t, set.Flags().Lookup("image"))
}

func TestMergeAgentServiceEnvWithServer(t *testing.T) {
	actual := mergeAgentServiceEnv([]any{
		"HOME=/tmp",
		"GALAXY_API_URL=http://127.0.0.1:9501",
		"GALAXY_BASE_URL=http://127.0.0.1:9501",
		"GALAXY_JOIN_ADDRESSES=2001:db8::1",
	}, "http://192.168.1.14:9501", true, "swarm-1", []string{"192.168.1.14", "2001:db8::2"})
	require.Equal(t, []string{
		"HOME=/tmp",
		"GALAXY_API_URL=http://192.168.1.14:9501",
		"GALAXY_BASE_URL=http://192.168.1.14:9501",
		"GALAXY_SWARM_ID=swarm-1",
		"GALAXY_JOIN_ADDRESSES=192.168.1.14,2001:db8::2",
	}, actual)
}

func TestMergeAgentServiceEnvForImageOnlyPreservesServer(t *testing.T) {
	actual := mergeAgentServiceEnv([]string{
		"GALAXY_API_URL=http://192.168.1.14:9501",
		"GALAXY_BASE_URL=http://192.168.1.14:9501",
		"GALAXY_JOIN_ADDRESSES=2001:db8::1",
	}, "", false, "swarm-1", []string{"192.168.1.14"})

	require.Equal(t, []string{
		"GALAXY_API_URL=http://192.168.1.14:9501",
		"GALAXY_BASE_URL=http://192.168.1.14:9501",
		"GALAXY_SWARM_ID=swarm-1",
		"GALAXY_JOIN_ADDRESSES=192.168.1.14",
	}, actual)
}

func TestNormalizeGlobalAgentServiceSpecRestoresEmptyObject(t *testing.T) {
	spec := map[string]any{
		"Mode": map[string]any{
			"Global":     []any{},
			"Replicated": nil,
		},
	}

	require.NoError(t, normalizeGlobalAgentServiceSpec(spec))
	raw, err := json.Marshal(spec)
	require.NoError(t, err)
	require.JSONEq(t, `{"Mode":{"Global":{}}}`, string(raw))
}

func TestNormalizeGlobalAgentServiceSpecRejectsNonGlobalMode(t *testing.T) {
	require.EqualError(t, normalizeGlobalAgentServiceSpec(map[string]any{
		"Mode": map[string]any{"Replicated": map[string]any{"Replicas": 1}},
	}), "galaxy-agent 必须是 Global Service")
}

func TestAgentInstallReadsBootstrapTokenFromFile(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "bootstrap-token")
	require.NoError(t, os.WriteFile(tokenFile, []byte(" secret-token \n"), 0o600))
	options := &agentOptions{agentTokenFile: tokenFile}

	token, err := options.readBootstrapToken()

	require.NoError(t, err)
	require.Equal(t, "secret-token", token)
}

func TestAgentInstallRejectsTokenAndTokenFileTogether(t *testing.T) {
	options := &agentOptions{agentToken: "one", agentTokenFile: "/tmp/another"}

	_, err := options.readBootstrapToken()

	require.EqualError(t, err, "--bootstrap-token 与 --bootstrap-token-file 不能同时使用")
}

func TestValidateAgentServerURLRejectsLoopback(t *testing.T) {
	for _, serverURL := range []string{
		"http://localhost:9501",
		"http://localhost.:9501",
		"http://127.0.0.1:9501",
		"http://[::1]:9501",
	} {
		require.Error(t, validateAgentServerURL(serverURL), serverURL)
	}
	require.NoError(t, validateAgentServerURL("http://192.168.1.14:9501"))
	require.NoError(t, validateAgentServerURL("https://api.example.com"))
}

func TestAgentServerCandidatesRecoverChangedManagerIP(t *testing.T) {
	candidates, err := agentServerCandidates("http://192.168.1.14:9501", nodeIdentity{
		Role:     "manager",
		NodeAddr: "192.168.1.51",
	})

	require.NoError(t, err)
	require.Equal(t, []string{
		"http://192.168.1.14:9501",
		"http://192.168.1.51:9501",
	}, candidates)
}

func TestAgentServerCandidatesDoNotRewriteRemoteDomainOrWorker(t *testing.T) {
	managerCandidates, err := agentServerCandidates("https://api.example.com", nodeIdentity{
		Role:     "manager",
		NodeAddr: "192.168.1.51",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"https://api.example.com"}, managerCandidates)

	workerCandidates, err := agentServerCandidates("http://192.168.1.14:9501", nodeIdentity{
		Role:     "worker",
		NodeAddr: "192.168.1.52",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"http://192.168.1.14:9501"}, workerCandidates)
}

func TestDockerRegistryAuthHeaderUsesDockerLoginConfig(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", configDir)
	config, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			"registry.example.com": map[string]string{"auth": "encoded-login"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600))

	header, err := dockerRegistryAuthHeader("registry.example.com/galaxy/galaxy-agent:1.0.1")

	require.NoError(t, err)
	raw, err := base64.RawURLEncoding.DecodeString(header)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"auth":"encoded-login"`)
	require.Contains(t, string(raw), `"serveraddress":"registry.example.com"`)
}

func TestDockerRegistryAuthHeaderRequiresLoginForConfiguredDefaultImage(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	t.Setenv(envAgentImage, "registry.example.com/galaxy/galaxy-agent:1.0.1")

	_, err := dockerRegistryAuthHeader("registry.example.com/galaxy/galaxy-agent:1.0.1")

	require.EqualError(t, err, "默认 Agent 镜像 registry.example.com/galaxy/galaxy-agent:1.0.1 未匹配到仓库凭证，本机 Docker 也未登录 registry.example.com；请在 Galaxy 管理中心配置该 Registry，或执行 docker login registry.example.com")
}

func TestDockerRegistryAuthHeaderSkipsGuidanceForCustomImage(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	t.Setenv(envAgentImage, "registry.example.com/galaxy/galaxy-agent:1.0.1")

	header, err := dockerRegistryAuthHeader("registry.example.com/other/app:2.0.0")

	require.NoError(t, err)
	require.Empty(t, header)
}

func TestDefaultAgentImageFromEnvironment(t *testing.T) {
	t.Setenv(envAgentImage, "registry.example.com/galaxy/galaxy-agent:9.9.9")

	image := defaultAgentImage()

	require.Equal(t, "registry.example.com/galaxy/galaxy-agent:9.9.9", image)
	require.NoError(t, requireAgentImage(image))
}

func TestDefaultAgentImageEnvironmentOverridesBuildDefault(t *testing.T) {
	original := buildVariable.AgentImage
	buildVariable.AgentImage = "registry.example.com/galaxy/galaxy-agent:1.0.0"
	t.Cleanup(func() { buildVariable.AgentImage = original })
	t.Setenv(envAgentImage, "mirror.example.com/galaxy-agent:1.0.0")

	require.Equal(t, "mirror.example.com/galaxy-agent:1.0.0", defaultAgentImage())
}

func TestRequireAgentImageRejectsEmpty(t *testing.T) {
	require.Error(t, requireAgentImage(""))
}

func TestAgentServiceUpdatePathUsesNewRegistryAuthWhenProvided(t *testing.T) {
	require.Equal(
		t,
		"/services/service-1/update?version=7",
		agentServiceUpdatePath("service-1", 7, true),
	)
	require.Equal(
		t,
		"/services/service-1/update?version=7&registryAuthFrom=spec",
		agentServiceUpdatePath("service-1", 7, false),
	)
}

func TestParseNodeIdentity(t *testing.T) {
	info := base64.StdEncoding.EncodeToString([]byte(`{
		"Name":"manager-1",
		"Swarm":{
			"NodeID":"node-1",
			"NodeAddr":"10.0.0.11",
			"ControlAvailable":true,
			"Nodes":3,
			"Cluster":{"ID":"swarm-1"}
		}
	}`))

	identity, err := parseNodeIdentity(info, "")
	require.NoError(t, err)
	require.Equal(t, "swarm-1", identity.SwarmID)
	require.Equal(t, "node-1", identity.NodeID)
	require.Equal(t, "10.0.0.11", identity.NodeAddr)
	require.Equal(t, "manager-1", identity.Hostname)
	require.Equal(t, "manager", identity.Role)
	require.Equal(t, 3, identity.SwarmNodes)
}

func TestParseNodeIdentityRejectsNonSwarmEngine(t *testing.T) {
	info := base64.StdEncoding.EncodeToString([]byte(`{"Name":"host","Swarm":{"NodeID":""}}`))

	_, err := parseNodeIdentity(info, "")
	require.EqualError(t, err, "本机尚未加入 Docker Swarm")
}

func TestParseWorkerNodeIdentityUsesConfiguredSwarmID(t *testing.T) {
	info := base64.StdEncoding.EncodeToString([]byte(`{
		"Name":"worker-1",
		"Swarm":{
			"NodeID":"node-2",
			"NodeAddr":"10.0.0.12",
			"LocalNodeState":"active",
			"ControlAvailable":false
		}
	}`))

	identity, err := parseNodeIdentity(info, "swarm-1")

	require.NoError(t, err)
	require.Equal(t, "swarm-1", identity.SwarmID)
	require.Equal(t, "node-2", identity.NodeID)
	require.Equal(t, "worker", identity.Role)
}

func TestParseWorkerNodeIdentityRequiresConfiguredSwarmID(t *testing.T) {
	info := base64.StdEncoding.EncodeToString([]byte(`{
		"Name":"worker-1",
		"Swarm":{"NodeID":"node-2","LocalNodeState":"active"}
	}`))

	_, err := parseNodeIdentity(info, "")

	require.EqualError(t, err, "Worker 无法从 Docker /info 获取 Swarm ID，Agent Service 缺少 GALAXY_SWARM_ID")
}

func TestReadySwarmNodeCount(t *testing.T) {
	nodes := base64.StdEncoding.EncodeToString([]byte(`[
		{"Status":{"State":"ready"}},
		{"Status":{"State":"down"}},
		{"Status":{"State":"ready"}}
	]`))

	count, err := readySwarmNodeCount(nodes)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}
