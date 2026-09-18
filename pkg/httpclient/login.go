package httpclient

import (
	"galaxy/pkg/galaxycfg"
	"galaxy/protoc"
	"github.com/gogf/gf/encoding/gjson"
)

type Service struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *HttpClient
}

func NewService(cfgFlags *galaxycfg.ConfigFlags) *Service {

	return &Service{
		cfgFlags: cfgFlags,
		client:   NewHttpClient(cfgFlags),
	}
}
func (that *Service) Login(cliLogin *protoc.CliLoginReq) (*protoc.LoginReturn, error) {
	loginReturn := &protoc.LoginReturn{}
	err := that.client.Post("/cli/login", gjson.New(cliLogin, true).MustToJsonString(), &loginReturn)
	if err != nil {
		return nil, err
	}

	return loginReturn, nil
}
