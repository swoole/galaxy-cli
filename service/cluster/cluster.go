package cluster

import (
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
)

type Service struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *httpclient.HttpClient
}

type ListResponse struct {
	Clusters []*ListItem `json:"clusters"`
}

type ListItem struct {
	ID               uint32         `json:"id"`
	Title            string         `json:"title"`
	Endpoint         string         `json:"endpoint"`
	OrchestratorType string         `json:"orchestrator_type"`
	Status           int            `json:"status"`
	Version          string         `json:"version"`
	CreatorInfo      *Creator       `json:"creator_info"`
	CreatedAt        int64          `json:"created_at"`
	Envs             []*Environment `json:"envs"`
}

type Creator struct {
	Nickname string `json:"nickname"`
}

type Environment struct {
	ID    uint32 `json:"id"`
	Title string `json:"title"`
}

func NewService(cfgFlags *galaxycfg.ConfigFlags) *Service {
	return &Service{
		cfgFlags: cfgFlags,
		client:   httpclient.NewHttpClient(cfgFlags),
	}
}

// List 集群
func (that *Service) List(orgID, groupID, projectID uint32) ([]*ListItem, error) {
	var rsp *ListResponse
	params := map[string]interface{}{"org": orgID}
	if groupID > 0 {
		params["group"] = groupID
	}
	if projectID > 0 {
		params["project"] = projectID
	}
	err := that.client.Get("/cluster", params, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.Clusters, nil
}
