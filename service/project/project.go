package project

import (
	"fmt"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/gogf/gf/encoding/gjson"
	"github.com/gogf/gf/frame/g"
)

type Service struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *httpclient.HttpClient
}

const RepositoryTypeExternal uint32 = 2

type Repository struct {
	Type     uint32 `json:"type"`
	Provider uint32 `json:"provider"`
	CloneURL string `json:"clone_url"`
}

type BuildCluster struct {
	ID    uint32 `json:"id"`
	Title string `json:"title"`
}

type CreateProps struct {
	BuildClusters []*BuildCluster `json:"build_clusters"`
}

type BuildProfile struct {
	DockerfileSource         string                 `json:"dockerfile_source"`
	BuildContext             string                 `json:"build_context"`
	RepositoryDockerfilePath string                 `json:"repository_dockerfile_path"`
	TemplateKey              string                 `json:"template_key"`
	Options                  map[string]interface{} `json:"options"`
}

type CreateRequest struct {
	OrgID          uint32       `json:"org"`
	GroupID        uint32       `json:"group"`
	Title          string       `json:"title"`
	Description    string       `json:"desc"`
	Develop        bool         `json:"develop"`
	BuildClusterID uint32       `json:"build_cluster_id"`
	Repository     Repository   `json:"repository"`
	BuildProfile   BuildProfile `json:"build_profile"`
	DefaultPort    uint32       `json:"default_port"`
	ImageName      string       `json:"image_name"`
	Registries     []string     `json:"registries"`
}

type CreatedProject struct {
	ID      uint32 `json:"id"`
	OrgID   uint32 `json:"org_id"`
	GroupID uint32 `json:"group_id"`
	Title   string `json:"title"`
}

type createResponse struct {
	Project *CreatedProject `json:"project"`
}

type repositoryResponse struct {
	Repository *Repository `json:"repository"`
}

type createPropsResponse struct {
	Props *CreateProps `json:"props"`
}

func NewService(cfgFlags *galaxycfg.ConfigFlags) *Service {
	return &Service{
		cfgFlags: cfgFlags,
		client:   httpclient.NewHttpClient(cfgFlags),
	}
}

// Profile 获取项目详情
func (that *Service) Profile(req *protoc.ProjectProfileReq) (*protoc.ProjectProfile, error) {
	var rsp *protoc.ProjectProfileRsp
	err := that.client.Get("/project/profile", &req, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.GetProject(), nil
}

// GetList 获取项目的列表
func (that *Service) GetList(reqs ...*protoc.ProjectListReq) (*protoc.ProjectListRsp, error) {
	var projectRsp *protoc.ProjectListRsp
	req := &protoc.ProjectListReq{OrgId: that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id}
	if len(reqs) > 0 {
		req = reqs[0]
	}
	err := that.client.Get("/project", req, &projectRsp)
	if err != nil {
		return nil, err
	}
	return projectRsp, nil
}

// CreateProject 使用当前 Project API 契约创建代码项目。
func (that *Service) CreateProject(req *CreateRequest) (*CreatedProject, error) {
	var rsp *createResponse
	if err := that.client.Post("/project", req, &rsp); err != nil {
		return nil, err
	}
	if rsp == nil || rsp.Project == nil {
		return nil, fmt.Errorf("创建项目接口未返回项目信息")
	}
	return rsp.Project, nil
}

// Basic 项目简单信息
func (that *Service) Basic(groupId, projectId uint32) (*protoc.ProjectBasic, error) {
	var req = g.Map{
		"org":     that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id,
		"group":   groupId,
		"project": projectId,
	}
	var projectBasicRsp *protoc.ProjectBasicRsp
	err := that.client.Get("/project/basic", req, &projectBasicRsp)
	if err != nil {
		return nil, err
	}
	projectBasic := projectBasicRsp.GetProject()
	if projectBasic != nil {
		projectBasic.GroupId = groupId
	}
	return projectBasic, nil
}

// Repository 获取项目绑定的外部 Git 仓库。
func (that *Service) Repository(groupID, projectID uint32) (*Repository, error) {
	req := g.Map{
		"org": that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id, "group": groupID, "project": projectID,
	}
	var rsp *repositoryResponse
	if err := that.client.Get("/project/repository", req, &rsp); err != nil {
		return nil, err
	}
	if rsp == nil || rsp.Repository == nil {
		return nil, fmt.Errorf("项目未绑定 Git 仓库")
	}
	return rsp.Repository, nil
}

func (that *Service) Simple(req *protoc.ProjectSimpleReq) ([]*protoc.ProjectSimple, error) {
	var rsp *protoc.ProjectSimpleRsp
	err := that.client.Get("/project/simple", req, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.GetProjects(), nil
}

func (that *Service) DeleteOverview(req *protoc.ProjectProfileReq) (*protoc.ProjectDeleteOverview, error) {
	var rsp *protoc.ProjectDeleteOverview
	err := that.client.Get("/project/deleteoverview", req, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

func (that *Service) SelectedProjectByGroupId(msg string, groupId uint32) (*protoc.ProjectSimple, error) {
	projects, err := that.Simple(&protoc.ProjectSimpleReq{OrgId: that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id, GroupId: groupId})
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, nil
	}

	titles := make([]string, len(projects)+1)
	titles[0] = "创建项目"
	for i, m := range projects {
		titles[i+1] = m.GetTitle()
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
	if answerIndex == 0 {
		return nil, nil
	}
	return projects[answerIndex-1], nil
}

func (that *Service) SelectedProject(msg string, projects []*protoc.ProjectList) (*protoc.ProjectList, error) {
	if len(projects) == 1 {
		return projects[0], nil
	}
	titles := make([]string, len(projects))
	for i, m := range projects {
		titles[i] = m.GetTitle()
	}
	answerIndex := 0
	err := survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return projects[answerIndex], nil
}

func (that *Service) AskTitle(title string) (string, error) {
	var defaultTitle = "请输入你要创建的项目名?"
	if len(title) > 0 {
		defaultTitle = fmt.Sprintf("%s默认(%s):", defaultTitle, title)
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

// ConfirmRemoveProject 确认是否要删除project
func (that *Service) ConfirmRemoveProject(a *protoc.ProjectList) (bool, error) {
	overview, err := that.DeleteOverview(&protoc.ProjectProfileReq{
		OrgId:     a.GetOrgId(),
		GroupId:   a.GetGroup().GetId(),
		ProjectId: a.GetId(),
	})
	if err != nil {
		return false, err
	}
	var str string
	if overview.GetInstances() > 0 {
		str += fmt.Sprintf("确认删除所有(%d个)实例及相关Service、持久化存储、定时任务等资源.\n", overview.GetInstances())
	}
	if overview.GetImages() > 0 {
		str += fmt.Sprintf("确认删除所有(%d个)镜像.\n", overview.GetImages())
	}
	if overview.GetPipelines() > 0 {
		str += fmt.Sprintf("确认删除所有(%d个)流水线.\n", overview.GetPipelines())
	}
	if overview.GetGithooks() > 0 {
		str += fmt.Sprintf("确认删除所有(%d个)流水线Git钩子.\n", overview.GetGithooks())
	}
	if overview.GetMembers() > 0 {
		str += fmt.Sprintf("确认删除所有(%d名)项目成员.\n", overview.GetMembers())
	}
	if overview.GetBuilds() > 0 {
		str += fmt.Sprintf("确认删除所有(%d条)构建记录.\n", overview.GetBuilds())
	}
	if overview.GetDeploys() > 0 {
		str += fmt.Sprintf("确认删除所有(%d条)部署.\n", overview.GetDeploys())
	}
	if len(overview.GetGitrepo()) > 0 {
		str += fmt.Sprintf("确认删除Git仓库: %s.\n", overview.GetGitrepo())
	}
	var confirm bool
	err = survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("%s 您确定要删除项目 %s，删除项目将导致项目内所有数据被清理，不可恢复，您确定知晓此风险？\n如果确认删除请输入:yes,默认:no.", str, color.GreenString(a.GetTitle())),
		Default: false,
	}, &confirm)
	if err != nil {
		return false, err
	}
	if !confirm {
		return false, nil
	}
	return true, nil
}

func (that *Service) Delete(req *protoc.ProjectDeleteReq) error {
	var rsp *gjson.Json
	err := that.client.Delete("/project", req, &rsp)
	if err != nil {
		return err
	}
	return nil
}

func (that *Service) CreateProps(groupID uint32) (*CreateProps, error) {
	var rsp *createPropsResponse
	req := g.Map{"org": that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id, "group": groupID}
	if err := that.client.Get("/project/createprops", req, &rsp); err != nil {
		return nil, err
	}
	if rsp == nil || rsp.Props == nil {
		return nil, fmt.Errorf("项目创建属性接口未返回数据")
	}
	return rsp.Props, nil
}
