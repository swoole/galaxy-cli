package instance

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
type RuntimeList struct {
	Runtimes []*Runtime `json:"runtimes"`
}
type Runtime struct {
	ID           uint32          `json:"id"`
	Name         string          `json:"name"`
	ServiceName  string          `json:"service_name"`
	RuntimeRef   string          `json:"runtime_ref"`
	DesiredCount uint32          `json:"desired_count"`
	RunningCount uint32          `json:"running_count"`
	Status       string          `json:"status"`
	Health       string          `json:"health"`
	CreatedAt    int64           `json:"created_at"`
	Env          *RuntimeRefName `json:"env"`
	Cluster      *RuntimeRefName `json:"cluster"`
	Release      *RuntimeRelease `json:"release"`
	Spec         *RuntimeSpec    `json:"spec"`
}
type RuntimeRefName struct {
	ID    uint32 `json:"id"`
	Title string `json:"title"`
}
type RuntimeRelease struct {
	ID      uint32 `json:"id"`
	Version string `json:"version"`
	Remark  string `json:"remark"`
}
type RuntimeSpec struct {
	Ports []*RuntimePort `json:"ports"`
}
type RuntimePort struct {
	Target    uint32 `json:"target"`
	Published uint32 `json:"published"`
	Protocol  string `json:"protocol"`
	Mode      string `json:"mode"`
}

func NewService(flags *galaxycfg.ConfigFlags) *Service {
	return &Service{cfgFlags: flags, client: httpclient.NewHttpClient(flags)}
}
func (s *Service) Runtimes(orgID, groupID, projectID uint32) ([]*Runtime, error) {
	var rsp *RuntimeList
	if err := s.client.Get("/deploy/runtime", map[string]interface{}{"org": orgID, "group": groupID, "project": projectID}, &rsp); err != nil {
		return nil, err
	}
	return rsp.Runtimes, nil
}
func (s *Service) SelectedRuntime(message string, orgID, groupID, projectID uint32, preferred string) (*Runtime, error) {
	runtimes, err := s.Runtimes(orgID, groupID, projectID)
	if err != nil {
		return nil, err
	}
	if len(runtimes) == 0 {
		return nil, fmt.Errorf("当前项目没有可用的 Runtime")
	}
	for _, runtime := range runtimes {
		if preferred != "" && (runtime.Name == preferred || runtime.ServiceName == preferred || runtime.RuntimeRef == preferred || fmt.Sprint(runtime.ID) == preferred) {
			return runtime, nil
		}
	}
	if preferred != "" {
		return nil, fmt.Errorf("未找到 Runtime %q", preferred)
	}
	if len(runtimes) == 1 {
		return runtimes[0], nil
	}
	titles := make([]string, len(runtimes))
	for i, runtime := range runtimes {
		title := runtime.Name
		if title == "" {
			title = runtime.ServiceName
		}
		if runtime.Env != nil && runtime.Cluster != nil {
			title = fmt.Sprintf("%s [%s/%s]", title, runtime.Env.Title, runtime.Cluster.Title)
		}
		titles[i] = title
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: message, Options: titles, Default: titles[0]}, &selected); err != nil {
		return nil, err
	}
	return runtimes[selected], nil
}
func (s *Service) ScaleRuntime(orgID, groupID, projectID, runtimeID uint32, replicas int) error {
	return s.client.Put("/deploy/runtime/scale", map[string]interface{}{"org": orgID, "group": groupID, "project": projectID, "runtime_id": runtimeID, "replicas": replicas}, nil)
}
func (s *Service) RestartRuntime(orgID, groupID, projectID, runtimeID uint32) error {
	return s.client.Post("/deploy/runtime/restart", map[string]interface{}{"org": orgID, "group": groupID, "project": projectID, "runtime_id": runtimeID}, nil)
}
