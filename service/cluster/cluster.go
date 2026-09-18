package cluster

import (
	"fmt"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/AlecAivazis/survey/v2"
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

// Simple 集群
func (that *Service) Simple(req *protoc.ClusterReq) ([]*protoc.Cluster, error) {

	var rsp *protoc.ClusterSimpleListRsp
	err := that.client.Get("/cluster/simple", req, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.GetClusters(), nil
}

func (that *Service) SelectedCluster(msg string, clus []*protoc.Cluster) (*protoc.Cluster, error) {
	if len(clus) == 0 {
		return nil, fmt.Errorf("待选择的集群不能为空")
	}
	if len(clus) == 1 {
		return clus[0], nil
	}
	titles := make([]string, len(clus))
	for i, m := range clus {
		titles[i] = m.GetTitle()
	}
	answerIndex := 0
	// ask the question
	err := survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
		Default: titles[0],
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return clus[answerIndex], nil
}

func TypeString(expr protoc.ClusterType) string {
	switch expr {
	case protoc.ClusterType_TYPE_SELF_PAY:
		return "自费"
	case protoc.ClusterType_TYPE_MANAGED:
		return "托管"
	}
	return "未知"
}

func SourceString(expr protoc.ClusterSource) string {
	switch expr {
	case protoc.ClusterSource_SOURCE_CREATE:
		return "自动创建"
	case protoc.ClusterSource_SOURCE_IMPORT:
		return "自动导入"
	case protoc.ClusterSource_SOURCE_FILL:
		return "手动导入"
	}
	return "未知"
}

func VendorString(expr protoc.ClusterVendor) string {
	switch expr {
	case protoc.ClusterVendor_VENDOR_NONE:
		return "自建"
	case protoc.ClusterVendor_VENDOR_TKE_MANAGED:
		return "腾讯云TKE托管版"
	case protoc.ClusterVendor_VENDOR_ACK_MANAGED:
		return "阿里云ACK托管版"
	case protoc.ClusterVendor_VENDOR_LINODE:
		return "linode"
	}
	return "未知"
}
