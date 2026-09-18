package group

import (
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/AlecAivazis/survey/v2"
	"github.com/gogf/gf/errors/gerror"
)

type Service struct {
	cfgFlags  *galaxycfg.ConfigFlags
	client    *httpclient.HttpClient
	questions []*survey.Question
}

func NewService(cfgFlags *galaxycfg.ConfigFlags) *Service {

	return &Service{
		cfgFlags: cfgFlags,
		client:   httpclient.NewHttpClient(cfgFlags),
	}
}

// Profile 查看项目组详情。
func (that *Service) Profile(req *protoc.GroupProfileReq) (*protoc.GroupProfile, error) {
	var rsp *protoc.GroupProfileRsp
	err := that.client.Get("/group/profile", &req, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.GetGroup(), nil
}

// GetList 获取项目组列表。
func (that *Service) GetList() (*protoc.GroupListRsp, error) {
	req := protoc.GroupListReq{OrgId: that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id}
	var groupRsp *protoc.GroupListRsp
	err := that.client.Get("/group", &req, &groupRsp)
	if err != nil {
		return nil, err
	}
	return groupRsp, nil
}

// Simple 获取简单项目组列表。
func (that *Service) Simple(req *protoc.GroupListReq) ([]*protoc.Group, error) {
	var groupSimpleRsp *protoc.GroupSimpleRsp
	err := that.client.Get("/group/simple", req, &groupSimpleRsp)
	if err != nil {
		return nil, err
	}
	return groupSimpleRsp.GetGroups(), nil
}

func (that *Service) Question(groups []*protoc.Group) (*protoc.Group, error) {
	if len(groups) == 1 {
		return groups[0], nil
	}
	titles := make([]string, len(groups))
	for i, m := range groups {
		titles[i] = m.Title
	}
	survey.SelectQuestionTemplate = selectQuestionTemplate
	answerIndex := 0
	// ask the question
	err := survey.AskOne(&survey.Select{
		Message: "请选择项目组:",
		Options: titles,
		Default: titles[0],
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return groups[answerIndex], nil
}

func (that *Service) SelectedGroup(msg string, groupName string) (*protoc.Group, error) {
	groups, err := that.Simple(&protoc.GroupListReq{OrgId: that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id})
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, gerror.New("你还未创建项目组，请先在 Web 控制台创建项目组")
	}
	if len(groupName) > 0 {
		for _, m := range groups {
			if groupName == m.GetTitle() {
				return m, err
			}
		}
		return nil, gerror.Newf("你选择的项目组 [%s] 不存在，请检查项目组名称", groupName)
	}
	if len(groups) == 1 {
		return groups[0], nil
	}
	titles := make([]string, len(groups))
	for i, m := range groups {
		titles[i] = m.GetTitle()
	}
	defaultTitle := titles[0]
	answerIndex := 0
	err = survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
		Default: defaultTitle,
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return groups[answerIndex], nil
}

var selectQuestionTemplate = `
{{- define "option"}}
    {{- if eq .SelectedIndex .CurrentIndex }}{{color .Config.Icons.SelectFocus.Format }}{{ .Config.Icons.SelectFocus.Text }} {{else}}{{color "default"}}  {{end}}
    {{- .CurrentOpt.Value}}{{ if ne ($.GetDescription .CurrentOpt) "" }} - {{color "cyan"}}{{ $.GetDescription .CurrentOpt }}{{end}}
    {{- color "reset"}}
{{end}}
{{- if .ShowHelp }}{{- color .Config.Icons.Help.Format }}{{ .Config.Icons.Help.Text }} {{ .Help }}{{color "reset"}}{{"\n"}}{{end}}
{{- color .Config.Icons.Question.Format }}{{ .Config.Icons.Question.Text }} {{color "reset"}}
{{- color "default+hb"}}{{ .Message }}{{ .FilterMessage }}{{color "reset"}}
{{- if .ShowAnswer}}{{color "cyan"}} {{.Answer}}{{color "reset"}}{{"\n"}}
{{- else}}
  {{- "  "}}{{- color "cyan"}}[请使用上下箭头移动选择，输入字符过滤过滤, type to filter{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} for more help{{end}}]{{color "reset"}}
  {{- "\n"}}
  {{- range $ix, $option := .PageEntries}}
    {{- template "option" $.IterateOption $ix $option}}
  {{- end}}
{{- end}}`
