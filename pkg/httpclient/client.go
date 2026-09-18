package httpclient

import (
	"fmt"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/logger"
	"galaxy/protoc"
	"github.com/gogf/gf/encoding/gjson"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/net/ghttp"
	"github.com/gogf/gf/os/glog"
	"github.com/gogf/gf/text/gstr"
	"github.com/gogf/gf/util/guid"
	"io"
)

type HttpClient struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *ghttp.Client
}

func NewHttpClient(cfgFlags *galaxycfg.ConfigFlags) *HttpClient {
	c := ghttp.NewClient()
	c.SetContentType("application/json")
	c.SetHeader("X-CG-Version", buildVariable.BuildVersion)
	return &HttpClient{
		client:   c,
		cfgFlags: cfgFlags,
	}
}

func (that *HttpClient) Get(uri string, req interface{}, ret interface{}) error {
	return that.Request("GET", that.getUrl(uri), req, ret)
}

func (that *HttpClient) Post(uri string, req interface{}, ret interface{}) error {
	return that.Request("POST", that.getUrl(uri), req, ret)
}

func (that *HttpClient) Put(uri string, req interface{}, ret interface{}) error {
	return that.Request("Put", that.getUrl(uri), req, ret)
}
func (that *HttpClient) Delete(uri string, req interface{}, ret interface{}) error {
	return that.Request("Delete", that.getUrl(uri), req, ret)
}
func (that *HttpClient) Request(method string, uri string, req interface{}, ret interface{}) (err error) {
	requestData := normalizeScopeParameters(req)
	defer func() {
		if glog.GetLevel()&glog.LEVEL_DEBU > 0 {
			glog.Debugf("请求 %s %s %s", method, uri, gjson.New(redactRequestForLog(requestData)).MustToJsonString())
			if err != nil {
				glog.Debugf("响应错误 %s", err.Error())
			} else {
				glog.Debugf("响应 %s", gjson.New(ret).MustToJsonString())
			}
		}
	}()

	var try = 0
Loop:
	if try > 1 {
		err = fmt.Errorf("用户未登录. 请使用 galaxy login 命令登录")
		return
	}
	if that.cfgFlags != nil && len(*that.cfgFlags.BearerToken) > 0 {
		that.client.SetHeader("Authorization", fmt.Sprintf("Bearer %s", *that.cfgFlags.BearerToken))
	}
	that.client.SetHeader("X-CG-RequestId", guid.S())
	response, err1 := that.client.DoRequest(method, uri, requestData)
	if err1 != nil {
		err = err1
		return
	}
	defer response.Close()
	if response.StatusCode != 200 {
		err = fmt.Errorf("请求 %s %s 失败!\n错误码: %d ", method, uri, response.StatusCode)
		return
	}
	rsp := &protoc.Response{}
	json := gjson.New(response.ReadAll())
	err = json.Struct(&rsp)
	if err != nil {
		return err
	}
	if rsp.GetCode() == 401 {
		err = LoginBySecret(that.cfgFlags)
		if err != nil {
			return
		}
		try++
		goto Loop
	}
	if rsp.GetCode() != 0 {
		err = gerror.New(rsp.GetMsg())
		return
	}
	if ret != nil {
		err = json.GetJson("data").Struct(ret)
		if err != nil {
			return
		}
	}
	return nil
}

func redactRequestForLog(req interface{}) interface{} {
	data := gjson.New(req).Map()
	if data == nil {
		return req
	}
	redacted := make(map[string]interface{}, len(data))
	for key, value := range data {
		switch gstr.ToLower(key) {
		case "password", "token", "secret", "private_key", "content", "files":
			redacted[key] = "[REDACTED]"
		default:
			redacted[key] = value
		}
	}
	return redacted
}

// normalizeScopeParameters keeps Go/protobuf identifiers descriptive while
// enforcing the public API contract: hierarchy scope parameters never expose
// database column names such as org_id, group_id or project_id.
func normalizeScopeParameters(req interface{}) interface{} {
	if req == nil {
		return nil
	}
	switch req.(type) {
	case string, []byte:
		return req
	}
	data := gjson.New(req).Map()
	if data == nil {
		return req
	}
	for databaseName, publicName := range map[string]string{
		"org_id":     "org",
		"group_id":   "group",
		"project_id": "project",
	} {
		if value, ok := data[databaseName]; ok {
			if _, exists := data[publicName]; !exists {
				data[publicName] = value
			}
			delete(data, databaseName)
		}
	}
	return data
}

func (that *HttpClient) RequestWatch(method string, uri string, req interface{}, ret chan []byte) error {

	if len(*that.cfgFlags.BearerToken) > 0 {
		that.client.SetHeader("Authorization", fmt.Sprintf("Bearer %s", *that.cfgFlags.BearerToken))
	}
	that.client.SetHeader("X-CG-RequestId", guid.S())
	response, err := that.client.DoRequest(method, that.getUrl(uri), req)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("请求 %s 失败!\n StatusCode: %d ", that.getUrl(uri), response.StatusCode)
	}
	go func() {
		for {
			b := make([]byte, 512)
			_, err = response.Body.Read(b)
			ret <- b
			if err != nil {
				_ = response.Close()
				close(ret)
				if err == io.EOF {
					return
				}
				logger.Println(err)
			}
		}
	}()
	return nil
}

func (that *HttpClient) getUrl(path string) string {
	return fmt.Sprintf("%s/%s", that.cfgFlags.GetAPIServer(), gstr.TrimLeftStr(path, "/"))
}
