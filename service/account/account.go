package account

import (
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/AlecAivazis/survey/v2"
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

// IdConfirm 输入密码再次确认
func (that *Service) IdConfirm() (string, error) {
	var password string
	prompt := &survey.Password{
		Message: "请输入你的密码验证:",
	}
	err := survey.AskOne(prompt, &password, nil)
	if err != nil {
		return "", err
	}
	req := &protoc.AccountIdConfirmReq{
		Password: password,
	}
	var rsp *protoc.AccountIdConfirmRsp
	err = that.client.Post("/account/idconfirm", req, &rsp)
	if err != nil {
		return "", err
	}
	return rsp.GetConfirmToken(), nil
}
