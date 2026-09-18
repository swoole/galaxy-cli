package docker

import (
	"bytes"
	"galaxy/pkg/galaxycfg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestComposeMigrationRequiresProjectBeforeCheck(t *testing.T) {
	var output bytes.Buffer
	command := newCmdComposeMigration(galaxycfg.IOStreams{
		In: &output, Out: &output, ErrOut: &output,
	})
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"migrate"})

	err := command.Execute()

	require.EqualError(t, err, `required flag(s) "project" not set`)
	require.NotContains(t, output.String(), "Compose → Swarm 迁移检查")
}

func TestComposeMigrateIsTheDefaultExecutionCommand(t *testing.T) {
	command := newCmdComposeMigration(galaxycfg.IOStreams{})

	migrate, _, err := command.Find([]string{"migrate"})
	require.NoError(t, err)
	require.NotNil(t, migrate.RunE)
	require.NotNil(t, migrate.Flags().Lookup("yes"))
	require.NotNil(t, migrate.Flags().Lookup("wait"))
	require.NotNil(t, migrate.Flags().Lookup("source-hash"))
	require.False(t, migrate.HasSubCommands())
}

func TestConfirmComposeCleanupRequiresIndependentFullYes(t *testing.T) {
	var output bytes.Buffer
	cleanup, err := confirmComposeCleanup(
		strings.NewReader("yes\n"),
		&output,
		"mysql",
		[]string{"container-id"},
		[]string{"/srv/mysql/compose.yml"},
		nil,
	)

	require.NoError(t, err)
	require.True(t, cleanup)
	require.Contains(t, output.String(), "迁移后清理")
	require.Contains(t, output.String(), "/srv/mysql/compose.yml")
	require.Contains(t, output.String(), "命名卷及其数据不会删除")

	output.Reset()
	cleanup, err = confirmComposeCleanup(
		strings.NewReader("y\n"),
		&output,
		"mysql",
		[]string{"container-id"},
		[]string{"/srv/mysql/compose.yml"},
		nil,
	)
	require.NoError(t, err)
	require.False(t, cleanup)
	require.Contains(t, output.String(), "旧资源已保留")
}

func TestRemoveComposeConfigFilesOnlyRemovesListedFiles(t *testing.T) {
	directory := t.TempDir()
	composeFile := filepath.Join(directory, "compose.yml")
	envFile := filepath.Join(directory, ".env")
	require.NoError(t, os.WriteFile(composeFile, []byte("services: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(envFile, []byte("SECRET=keep\n"), 0o600))

	require.NoError(t, removeComposeConfigFiles([]string{composeFile}))

	_, err := os.Stat(composeFile)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(envFile)
	require.NoError(t, err)
}

func TestParseComposeProjectsSupportsArrayAndSorts(t *testing.T) {
	projects, err := parseComposeProjects([]byte(`[
		{"Name":"web","Status":"running(2)","ConfigFiles":"/srv/web/compose.yml"},
		{"Name":"api","Status":"exited(1)","ConfigFiles":"/srv/api/compose.yml"}
	]`))

	require.NoError(t, err)
	require.Equal(t, []composeProject{
		{Name: "api", Status: "exited(1)", ConfigFiles: "/srv/api/compose.yml"},
		{Name: "web", Status: "running(2)", ConfigFiles: "/srv/web/compose.yml"},
	}, projects)
}

func TestParseComposeProjectsSupportsJSONLines(t *testing.T) {
	projects, err := parseComposeProjects([]byte(
		"{\"Name\":\"api\",\"Status\":\"running(1)\",\"ConfigFiles\":\"compose.yml\"}\n" +
			"{\"Name\":\"worker\",\"Status\":\"exited(1)\",\"ConfigFiles\":\"worker.yml\"}\n",
	))

	require.NoError(t, err)
	require.Len(t, projects, 2)
	require.Equal(t, "worker", projects[1].Name)
}

func TestSelectComposeProjectUsesRequestedProject(t *testing.T) {
	projects := []composeProject{
		{Name: "api", ConfigFiles: "/srv/api/compose.yml"},
		{Name: "mysql", ConfigFiles: "/srv/mysql/compose.yml"},
	}

	selected, err := selectComposeProject("mysql", projects)

	require.NoError(t, err)
	require.Equal(t, projects[1], selected)
}

func TestSelectComposeProjectRejectsMissingProject(t *testing.T) {
	_, err := selectComposeProject("", []composeProject{{
		Name: "mysql", ConfigFiles: "/srv/mysql/compose.yml",
	}})

	require.EqualError(t, err, "必须指定 --project")
}

func TestSplitComposeConfigFiles(t *testing.T) {
	require.Equal(t,
		[]string{"/srv/app/compose.yml", "/srv/app/compose.prod.yml"},
		splitComposeConfigFiles("/srv/app/compose.yml,/srv/app/compose.prod.yml"),
	)
}

func TestAnalyzeComposeMigrationConfigReportsResourcesAndRisks(t *testing.T) {
	summary := analyzeComposeMigrationConfig(`
services:
  api:
    image: registry.example.com/api:1
    build: .
    depends_on:
      - db
    volumes:
      - /srv/data:/data
  db:
    volumes:
      - db-data:/var/lib/db
volumes:
  db-data: {}
networks:
  default: {}
configs:
  app: {file: app.conf}
secrets:
  password: {file: password.txt}
`)

	require.Equal(t, 2, summary.Services)
	require.Equal(t, 1, summary.Networks)
	require.Equal(t, 1, summary.Volumes)
	require.Equal(t, 1, summary.Configs)
	require.Equal(t, 1, summary.Secrets)
	require.Equal(t, []string{"registry.example.com/api:1"}, summary.Images)
	require.Contains(t, summary.Failures, "Service db 没有 image；Swarm 不会执行 Compose build")
	require.Contains(t, summary.Warnings, "Service api 的 build 会被 Swarm 忽略；迁移前必须构建并推送 image")
	require.Contains(t, summary.Warnings, "Service api 使用绑定挂载 /srv/data；迁移时将固定到当前节点")
}

func TestApplyComposeBindPlacementConstrainsServicesWithNodeLocalStorage(t *testing.T) {
	result := applyComposeBindPlacement(`
services:
  api:
    image: api:1
    volumes:
      - type: bind
        source: /srv/api
        target: /data
    deploy:
      placement:
        constraints:
          - node.labels.zone == primary
  worker:
    image: worker:1
    volumes:
      - worker-data:/data
volumes:
  worker-data: {}
`, "manager-1")

	require.Empty(t, result.Failures)
	require.Equal(t, []string{"api", "worker"}, result.Services)

	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(result.Content), &document))
	services := stringMap(document["services"])
	api := stringMap(services["api"])
	apiConstraints := stringSlice(stringMap(stringMap(api["deploy"])["placement"])["constraints"])
	require.Equal(t, []string{"node.labels.zone == primary", "node.hostname == manager-1"}, apiConstraints)
	worker := stringMap(services["worker"])
	workerConstraints := stringSlice(stringMap(stringMap(worker["deploy"])["placement"])["constraints"])
	require.Equal(t, []string{"node.hostname == manager-1"}, workerConstraints)
}

func TestApplyComposeBindPlacementRejectsHostnameConflict(t *testing.T) {
	result := applyComposeBindPlacement(`
services:
  api:
    image: api:1
    volumes:
      - /srv/api:/data
    deploy:
      placement:
        constraints:
          - node.hostname == manager-2
`, "manager-1")

	require.Contains(t, result.Failures,
		"Service api 已约束到其他节点 manager-2，与绑定挂载所在节点 manager-1 冲突")
}

func TestApplyComposeBindPlacementDoesNotPinRemoteVolumeDriver(t *testing.T) {
	result := applyComposeBindPlacement(`
services:
  api:
    image: api:1
    volumes:
      - shared:/data
volumes:
  shared:
    driver: nfs
`, "manager-1")

	require.Empty(t, result.Failures)
	require.Empty(t, result.Services)
	require.Equal(t, "\nservices:\n  api:\n    image: api:1\n    volumes:\n      - shared:/data\nvolumes:\n  shared:\n    driver: nfs\n", result.Content)
}

func TestNormalizeComposeStackPortsConvertsNumericStrings(t *testing.T) {
	result, err := normalizeComposeStackConfig(`
name: mysql
services:
  mysql:
    image: mysql:8.0
    ports:
      - mode: ingress
        target: 3306
        published: "3306"
        protocol: tcp
`)

	require.NoError(t, err)
	require.Equal(t, []string{"mysql"}, result.Services)
	require.True(t, result.RemovedRootName)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(result.Content), &document))
	require.NotContains(t, document, "name")
	mysql := stringMap(stringMap(document["services"])["mysql"])
	port := anySlice(mysql["ports"])[0].(map[string]any)
	require.Equal(t, 3306, port["target"])
	require.Equal(t, 3306, port["published"])
}

func TestNormalizedComposeV2ConfigPassesDockerStackSchema(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI is not installed")
	}
	result, err := normalizeComposeStackConfig(`
name: mysql
services:
  mysql:
    image: mysql:8.0
    ports:
      - mode: ingress
        target: 3306
        published: "3306"
        protocol: tcp
    volumes:
      - type: volume
        source: mysql_data
        target: /var/lib/mysql
volumes:
  mysql_data:
    name: mysql_mysql_data
`)
	require.NoError(t, err)

	plan := swarmMigrationPlan{Stack: "mysql", Content: result.Content}
	require.NoError(t, plan.validate())
}

func TestCommandOutputDoesNotMixSuccessfulStderrIntoJSON(t *testing.T) {
	stdout, diagnostic, err := commandOutput(exec.Command(
		"sh", "-c", `printf '[{"State":"running"}]'; printf 'obsolete warning' >&2`,
	))

	require.NoError(t, err)
	require.JSONEq(t, `[{"State":"running"}]`, string(stdout))
	require.Empty(t, diagnostic)
}

func TestParseComposeContainerStates(t *testing.T) {
	total, running, stopped, err := parseComposeContainerStates([]byte(
		"{\"Name\":\"api-1\",\"State\":\"running\"}\n" +
			"{\"Name\":\"worker-1\",\"State\":\"exited\"}\n",
	))

	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, 1, running)
	require.Equal(t, 1, stopped)
}

func TestPrintMigrationChecksShowsDecision(t *testing.T) {
	var output bytes.Buffer
	printMigrationChecks(&output, "demo", "demo", "/srv/demo", composeMigrationSummary{Services: 1}, []composeMigrationCheck{
		{Status: "通过", Name: "配置", Detail: "有效"},
		{Status: "警告", Name: "卷", Detail: "使用本地卷"},
	}, "abc")

	require.Contains(t, output.String(), "✓ 可以迁移")
	require.Contains(t, output.String(), "1 项警告")
	require.Contains(t, output.String(), "配置指纹")
	require.Contains(t, output.String(), "abc")
	require.Contains(t, output.String(), "╭")
}
