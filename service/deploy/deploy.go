package deploy

import (
	"fmt"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"github.com/AlecAivazis/survey/v2"
)

type Service struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *httpclient.HttpClient
}

type Options struct {
	Environments       []*Environment     `json:"environments"`
	Clusters           []*Cluster         `json:"clusters"`
	EnvironmentCluster []*EnvironmentLink `json:"environment_clusters"`
	Artifacts          []*ArtifactOption  `json:"artifacts"`
	Networks           []*Network         `json:"networks"`
	SuggestedPort      int                `json:"suggested_published_port"`
}

type Environment struct {
	ID          uint32 `json:"id"`
	Title       string `json:"title"`
	Deployable  bool   `json:"deployable"`
	Unavailable string `json:"unavailable_reason"`
}
type Cluster struct {
	ID    uint32 `json:"id"`
	Title string `json:"title"`
}
type EnvironmentLink struct {
	EnvID     uint32 `json:"env_id"`
	ClusterID uint32 `json:"cluster_id"`
}
type ArtifactOption struct {
	ID          uint32 `json:"id"`
	Reference   string `json:"reference"`
	DefaultPort int    `json:"default_port"`
}
type Network struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}
type ReleaseList struct {
	Page     uint32     `json:"page"`
	PageSize uint32     `json:"pagesize"`
	Total    uint32     `json:"total"`
	Data     []*Release `json:"data"`
}
type Release struct {
	ID          uint32           `json:"id"`
	OrgID       uint32           `json:"org_id"`
	GroupID     uint32           `json:"group_id"`
	ProjectID   uint32           `json:"project_id"`
	EnvID       uint32           `json:"env_id"`
	ClusterID   uint32           `json:"cluster_id"`
	ArtifactID  uint32           `json:"artifact_id"`
	Version     string           `json:"version"`
	Remark      string           `json:"remark"`
	Status      string           `json:"status"`
	Operation   string           `json:"operation"`
	CreatedAt   int64            `json:"created_at"`
	Env         *Ref             `json:"env"`
	Cluster     *Ref             `json:"cluster"`
	Artifact    *ReleaseArtifact `json:"artifact"`
	Runtime     *ReleaseRuntime  `json:"runtime"`
	CreatorInfo *Creator         `json:"creator_info"`
}
type Ref struct {
	ID    uint32 `json:"id"`
	Title string `json:"title"`
}
type Creator struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
}
type ReleaseArtifact struct {
	ID        uint32 `json:"id"`
	Reference string `json:"reference"`
}
type ReleaseRuntime struct {
	ID          uint32 `json:"id"`
	Name        string `json:"name"`
	ServiceName string `json:"service_name"`
	Status      string `json:"status"`
}

func (that *Service) Releases(orgID, groupID, projectID uint32) (*ReleaseList, error) {
	var rsp *ReleaseList
	if err := that.client.Get("/deploy", map[string]interface{}{"org": orgID, "group": groupID, "project": projectID, "pagesize": 100}, &rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (that *Service) RollbackRelease(orgID, groupID, projectID, releaseID uint32) error {
	return that.client.Post("/deploy/rollback", map[string]interface{}{"org": orgID, "group": groupID, "project": projectID, "release_id": releaseID}, nil)
}

func (that *Service) ReleaseOptions(orgID, groupID, projectID, clusterID uint32) (*Options, error) {
	params := map[string]interface{}{"org": orgID, "group": groupID, "project": projectID}
	if clusterID > 0 {
		params["cluster_id"] = clusterID
	}
	var rsp *Options
	if err := that.client.Get("/deploy/options", params, &rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (that *Service) UpdateArtifact(orgID, groupID, projectID, runtimeID, artifactID uint32, remark string) error {
	return that.client.Post("/deploy/artifact/update", map[string]interface{}{
		"org": orgID, "group": groupID, "project": projectID, "runtime_id": runtimeID,
		"artifact_id": artifactID, "remark": remark,
	}, nil)
}

func (that *Service) CreateRelease(orgID, groupID, projectID, envID, clusterID, artifactID uint32, version, remark string, spec map[string]interface{}) error {
	return that.client.Post("/deploy", map[string]interface{}{
		"org": orgID, "group": groupID, "project": projectID, "env_id": envID,
		"cluster_id": clusterID, "artifact_id": artifactID, "version": version,
		"remark": remark, "spec": spec,
	}, nil)
}

func (that *Service) SelectTarget(options *Options) (uint32, uint32, error) {
	var envs []*Environment
	for _, env := range options.Environments {
		if env.Deployable {
			envs = append(envs, env)
		}
	}
	if len(envs) == 0 {
		return 0, 0, fmt.Errorf("当前项目没有可部署环境")
	}
	env := envs[0]
	if len(envs) > 1 {
		titles := make([]string, len(envs))
		for i, item := range envs {
			titles[i] = item.Title
		}
		selected := 0
		if err := survey.AskOne(&survey.Select{Message: "请选择环境", Options: titles, Default: titles[0]}, &selected); err != nil {
			return 0, 0, err
		}
		env = envs[selected]
	}
	allowed := map[uint32]bool{}
	for _, link := range options.EnvironmentCluster {
		if link.EnvID == env.ID {
			allowed[link.ClusterID] = true
		}
	}
	var clusters []*Cluster
	for _, cluster := range options.Clusters {
		if allowed[cluster.ID] {
			clusters = append(clusters, cluster)
		}
	}
	if len(clusters) == 0 {
		return 0, 0, fmt.Errorf("环境 %s 没有已授权且在线的 Swarm 集群", env.Title)
	}
	cluster := clusters[0]
	if len(clusters) > 1 {
		titles := make([]string, len(clusters))
		for i, item := range clusters {
			titles[i] = item.Title
		}
		selected := 0
		if err := survey.AskOne(&survey.Select{Message: "请选择 Swarm 集群", Options: titles, Default: titles[0]}, &selected); err != nil {
			return 0, 0, err
		}
		cluster = clusters[selected]
	}
	return env.ID, cluster.ID, nil
}

func NewService(cfgFlags *galaxycfg.ConfigFlags) *Service {
	return &Service{
		cfgFlags: cfgFlags,
		client:   httpclient.NewHttpClient(cfgFlags),
	}
}

func (that *Service) AskInstanceName(title string) (string, error) {
	var defaultTitle = "请输入你要创建的实例名(注意实例名必须项目内唯一):"
	if len(title) > 0 {
		defaultTitle = fmt.Sprintf("%s 默认(%s):", defaultTitle, title)
	}
	var qs = []*survey.Question{
		{
			Name:   "title",
			Prompt: &survey.Input{Message: defaultTitle},
		},
	}
	answers := struct {
		Title string // survey will match the question and field names
	}{}
	// perform the questions
	err := survey.Ask(qs, &answers)
	if err != nil {
		return "", err
	}
	if len(answers.Title) == 0 {
		return title, nil
	}
	return answers.Title, nil
}
