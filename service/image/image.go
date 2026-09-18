package image

import (
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
)

type Service struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *httpclient.HttpClient
}

type ArtifactList struct {
	Data  []*Artifact `json:"data"`
	Total uint32      `json:"total"`
}
type Artifact struct {
	ID        uint32         `json:"id"`
	Reference string         `json:"reference"`
	Digest    string         `json:"digest"`
	Size      uint64         `json:"size"`
	CreatedAt int64          `json:"created_at"`
	Build     *ArtifactBuild `json:"build"`
}
type ArtifactBuild struct {
	ID          uint32           `json:"id"`
	Branch      string           `json:"branch"`
	CommitID    string           `json:"commit_id"`
	Remark      string           `json:"remark"`
	CreatedAt   int64            `json:"created_at"`
	CreatorInfo *ArtifactCreator `json:"creator_info"`
}
type ArtifactCreator struct {
	Nickname string `json:"nickname"`
}

func (that *Service) Artifacts(req *protoc.ImageListReq) (*ArtifactList, error) {
	params := map[string]interface{}{"org": req.GetOrgId(), "group": req.GetGroupId(), "project": req.GetProjectId(), "pagesize": 100}
	var rsp *ArtifactList
	if err := that.client.Get("/build/artifacts", params, &rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func NewService(cfgFlags *galaxycfg.ConfigFlags) *Service {
	return &Service{
		cfgFlags: cfgFlags,
		client:   httpclient.NewHttpClient(cfgFlags),
	}
}
