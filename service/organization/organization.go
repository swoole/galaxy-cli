package organization

import (
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/AlecAivazis/survey/v2"
	"github.com/gogf/gf/errors/gerror"
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

// Profile 获取组织详情
func (that *Service) Profile(orgId uint32) (*protoc.OrgProfile, error) {
	var rsp *protoc.OrgProfileRsp
	err := that.client.Get("org/profile", g.Map{"org": orgId}, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.GetOrg(), nil
}

// Simple 组织简单信息
func (that *Service) Simple() ([]*protoc.Organization, error) {

	var orgs *protoc.Orgs
	err := that.client.Get("/org/simple", "{}", &orgs)
	if err != nil {
		return nil, err
	}
	return orgs.GetOrgs(), nil
}

// SelectedOrg 选择组织
func (that *Service) SelectedOrg(msg string, orgName string) (*protoc.Organization, error) {
	orgs, err := that.Simple()
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, gerror.New("你还未创建组织，赶快去创建吧！")
	}
	// 是否强制选择 orgName,如果强制选择组织，则需要优先判断
	if len(orgName) > 0 {
		for _, m := range orgs {
			if orgName == m.GetTitle() {
				return m, err
			}
		}
		return nil, gerror.Newf("你选择的组织[%s]不存在,请查看组织名是否正确。", orgName)
	}

	if len(orgs) == 1 {
		return orgs[0], nil
	}

	var defaultTitle = orgName
	titles := make([]string, len(orgs))
	for i, m := range orgs {
		titles[i] = m.GetTitle()
	}
	dOrg := that.cfgFlags.GalaxyConfig.GetDefaultOrg()
	if dOrg != nil {
		defaultTitle = dOrg.Title
	}
	answerIndex := 0
	err = survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
		Default: defaultTitle,
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return orgs[answerIndex], nil
}

func OrgROLEString(expr protoc.OrgROLE) string {
	switch expr {
	case protoc.OrgROLE_ROLE_Normal:
		return "未知"
	case protoc.OrgROLE_ROLE_MANAGER:
		return "管理员"
	case protoc.OrgROLE_ROLE_PM:
		return "产品经理"
	case protoc.OrgROLE_ROLE_DEV:
		return "研发"
	case protoc.OrgROLE_ROLE_TESTER:
		return "测试"
	case protoc.OrgROLE_ROLE_OPS:
		return "运维"
	case protoc.OrgROLE_ROLE_DIRECTOR:
		return "负责人"
	}
	return "未知"
}

func OrgAUTHString(expr protoc.OrgAUTH) string {
	switch expr {
	case protoc.OrgAUTH_AUTH_NO:
		return "未认证"
	case protoc.OrgAUTH_AUTH_COMPANY:
		return "企业认证"
	case protoc.OrgAUTH_AUTH_PERSONAL:
		return "个人认证"

	}
	return "未知"
}

func OrgTYPEString(expr protoc.OrgTYPE) string {
	switch expr {
	case protoc.OrgTYPE_TYPE_PERSONAL:
		return "个人"
	case protoc.OrgTYPE_TYPE_COMPANY:
		return "公司"
	}
	return "未知"
}

func OrgSTATUSString(expr protoc.OrgSTATUS) string {
	switch expr {
	case protoc.OrgSTATUS_STATUS_NORMAL:
		return "正常"
	case protoc.OrgSTATUS_STATUS_IN_DISSOLUTION:
		return "解散中"
	}
	return "未知"
}
