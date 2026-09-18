package pipelines

import (
	"fmt"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/AlecAivazis/survey/v2"
	"github.com/gogf/gf/frame/g"
)

type Service struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *httpclient.HttpClient
}

func NewService(cfgFlags *galaxycfg.ConfigFlags) *Service {
	return &Service{
		cfgFlags: cfgFlags,
		client:   httpclient.NewHttpClient(cfgFlags),
	}
}

// List 获取流水线列表
func (that *Service) List(req *protoc.PipelineListReq) (*protoc.PipeLineListRsp, error) {
	var rsp *protoc.PipeLineListRsp
	err := that.client.Get("/pipeline", req, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

// Simple 简单流水线列表
func (that *Service) Simple(orgId, groupId, projectId uint32) ([]*protoc.PipelineSimple, error) {
	var req = g.Map{
		"org":                         orgId,
		"group":                       groupId,
		"project":                     projectId,
		"with_framework":              1,
		"with_framework_version":      1,
		"with_framework_lang_version": 1,
	}
	var simplePipelineRsp *protoc.PipelineSimpleRsp
	err := that.client.Get("/pipeline/simple", req, &simplePipelineRsp)
	if err != nil {
		return nil, err
	}
	return simplePipelineRsp.GetPipelines(), nil
}

// AskPipelines 提问选择流水线
func (that *Service) AskPipelines(msg string, pipeline []*protoc.PipelineSimple, pipelineName string) (*protoc.PipelineSimple, error) {

	if len(pipeline) == 0 {
		return nil, fmt.Errorf("没有可用的流水线")
	}

	if len(pipeline) == 1 {
		if len(pipelineName) > 0 && pipeline[0].GetTitle() != pipelineName {
			return nil, fmt.Errorf("流水线 %s 不存在", pipelineName)
		}
		return pipeline[0], nil
	}

	titles := make([]string, len(pipeline))
	for i, m := range pipeline {
		titles[i] = m.Title
		if m.Title == pipelineName {
			return m, nil
		}
	}
	defaultPipelineTitle := that.cfgFlags.OptionsCache().Get(galaxycfg.BuildPipeLineLatestSelected, titles[0])
	answerIndex := 0
	// ask the question
	err := survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
		Default: defaultPipelineTitle.String(),
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	result := pipeline[answerIndex]
	that.cfgFlags.OptionsCache().Set(galaxycfg.BuildPipeLineLatestSelected, result.GetTitle())
	return result, nil
}
