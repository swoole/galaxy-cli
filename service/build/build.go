package build

import (
	"fmt"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/AlecAivazis/survey/v2"
	"github.com/gogf/gf/frame/g"
	"github.com/gogf/gf/text/gstr"
	"time"
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

// BuildList 获取构建列表
func (that *Service) BuildList(req *protoc.BuildListReq) (*protoc.BuildListRsp, error) {

	var buildRsp *protoc.BuildListRsp
	params := g.Map{"org": req.GetOrgId(), "group": req.GetGroupId(), "project": req.GetProjectId()}
	if req.GetPipelineId() > 0 {
		params["pipeline_id"] = req.GetPipelineId()
	}
	if req.GetHookId() > 0 {
		params["hook_id"] = req.GetHookId()
	}
	if req.GetPage() > 0 {
		params["page"] = req.GetPage()
	}
	if req.GetPagesize() > 0 {
		params["pagesize"] = req.GetPagesize()
	}
	err := that.client.Get("/build", params, &buildRsp)
	if err != nil {
		return nil, err
	}
	return buildRsp, nil
}

// Build 发起构建
func (that *Service) Build(req *protoc.BuildReq) (*protoc.BuildRspInfo, error) {

	var buildRsp *protoc.BuildRsp
	err := that.client.Post("/build", req, &buildRsp)
	if err != nil {
		return nil, err
	}
	return buildRsp.GetBuild(), nil
}

// ReBuild 重新发起构建
func (that *Service) ReBuild(req *protoc.BuildReReq) (*protoc.BuildRspInfo, error) {

	var buildRsp *protoc.BuildRsp
	err := that.client.Post("/build/rebuild", req, &buildRsp)
	if err != nil {
		return nil, err
	}
	return buildRsp.GetBuild(), nil
}

// FastBuild 获取快速构建参数
// BuildLog 构建日志
func (that *Service) BuildLog(req *protoc.BuildLogReq) (*protoc.BuildLogRspInfo, error) {

	var buildRsp *protoc.BuildLogRsp
	err := that.client.Get("/build/log", req, &buildRsp)
	if err != nil {
		return nil, err
	}
	return buildRsp.GetBuildlog(), nil
}

func (that *Service) BuildLogWatch(req *protoc.BuildLogReq, log chan []byte) error {
	type currentLog struct {
		Status int32  `json:"status"`
		Log    string `json:"log"`
		Error  string `json:"error"`
	}
	type response struct {
		BuildLog *currentLog `json:"buildlog"`
	}
	params := map[string]interface{}{"org": req.GetOrgId(), "group": req.GetGroupId(), "project": req.GetProjectId(), "build_id": req.GetBuildId()}
	fetch := func() (*currentLog, error) {
		var rsp *response
		if err := that.client.Get("/build/log", params, &rsp); err != nil {
			return nil, err
		}
		return rsp.BuildLog, nil
	}
	first, err := fetch()
	if err != nil {
		return err
	}
	go func() {
		defer close(log)
		sent := 0
		current := first
		for {
			if current != nil {
				if len(current.Log) > sent {
					log <- []byte(current.Log[sent:])
					sent = len(current.Log)
				}
				if current.Status >= 2 {
					if current.Error != "" {
						log <- []byte("\n" + current.Error + "\n")
					}
					return
				}
			}
			time.Sleep(2 * time.Second)
			var e error
			current, e = fetch()
			if e != nil {
				log <- []byte("\n" + e.Error() + "\n")
				return
			}
		}
	}()
	return nil
}

func (that *Service) SelectedBuild(msg string, buildInfos []*protoc.BuildRspInfo) (*protoc.BuildRspInfo, error) {
	if len(buildInfos) == 1 {
		return buildInfos[0], nil
	}
	titles := make([]string, len(buildInfos))
	for i, m := range buildInfos {
		titles[i] = fmt.Sprintf("#%d[%s:%s]%s(%s)", m.GetId(), m.GetBranch(), gstr.SubStr(m.GetCommitId(), 0, 8), m.GetPipeline().GetTitle(), gstr.SubStrRune(gstr.Trim(m.GetRemark()), 0, 10))
	}
	answerIndex := 0
	err := survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
		Default: titles[0],
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return buildInfos[answerIndex], nil
}

func StatusString(expr protoc.BuildStatus) string {
	switch expr {
	case protoc.BuildStatus_BuildStatusPending:
		return "待执行"
	case protoc.BuildStatus_BuildStatusRunning:
		return "执行中"
	case protoc.BuildStatus_BuildStatusSuccess:
		return "成功"
	case protoc.BuildStatus_BuildStatusFailed:
		return "失败"
	}
	return "未知"
}
