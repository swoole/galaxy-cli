package galaxycfg

import (
	"fmt"
	"galaxy/pkg/buildVariable"
	"github.com/gogf/gf/encoding/gjson"
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/text/gstr"
	"net/url"
)

// User 用户信息
type User struct {
	Id       int    `yaml:"Id"`
	Nickname string `yaml:"Nickname"`
	Avatar   string `yaml:"Avatar"`
	Email    string `yaml:"Email"`
}

// Organization 组织信息
type Organization struct {
	Id             uint32  `yaml:"Id"`
	Title          string  `yaml:"Title"`
	DefaultGroupId uint32  `yaml:"DefaultGroupId"`
	Groups         []Group `yaml:"Groups"`
}

type Server struct {
	ApiVersion   string         `yaml:"ApiVersion"`
	ApiServer    string         `yaml:"ApiServer"`
	Token        string         `yaml:"Token"`
	Secret       string         `yaml:"Secret"` // 账号密码加密过的字符串
	User         User           `yaml:"User"`
	DefaultOrgId uint32         `yaml:"OrgId"`
	Orgs         []Organization `yaml:"Orgs"`
}

type Group struct {
	Id    uint32 `yaml:"Id"`
	Title string `yaml:"Title"`
}
type GalaxyConfig struct {
	serverUrl string
	Servers   []*Server `yaml:"servers"`
}

func NewGalaxyConfig(serverUrl string) *GalaxyConfig {
	var u1 string
	if len(serverUrl) > 0 {
		u, err := url.Parse(gstr.Trim(serverUrl, " "))
		if err != nil {
			panic(fmt.Sprintf("Galaxy Base Url: %s 格式不正确,err:%v", serverUrl, err))
		}
		u1 = fmt.Sprintf("%s", u.Hostname())
		if len(u.Port()) > 0 {
			u1 = fmt.Sprintf("%s:%s", u1, u.Port())
		}
	}
	return &GalaxyConfig{
		serverUrl: u1,
	}
}

// Load 初始化配置文件中的配置
func (that *GalaxyConfig) Load(path string) error {
	j, err := gjson.LoadYaml(gfile.GetBytes(path))
	if err != nil {
		return err
	}
	if j.IsNil() {
		return nil
	}
	err = j.Struct(&that)
	if err != nil {
		return err
	}
	return nil
}

func (that *GalaxyConfig) Save(path string) error {
	return gfile.PutBytes(path, gjson.New(that).MustToYaml())
}

func (that *GalaxyConfig) GetServer() *Server {
	for _, server := range that.Servers {
		if server.ApiServer == that.serverUrl {
			return server
		}
	}
	svr := &Server{ApiServer: that.serverUrl, ApiVersion: buildVariable.ApiVersion}
	that.Servers = append(that.Servers, svr)
	return svr
}
func (that *GalaxyConfig) GetApiVersion() string {
	server := that.GetServer()
	if server == nil {
		return ""
	}
	return server.ApiVersion
}
func (that *GalaxyConfig) GetToken() string {
	server := that.GetServer()
	if server == nil {
		return ""
	}
	return server.Token
}

func (that *GalaxyConfig) SetToken(token string) {
	server := that.GetServer()
	if server == nil {
		return
	}
	server.Token = token
	return
}
func (that *GalaxyConfig) SetOrgId(orgId uint32) {
	server := that.GetServer()
	if server == nil {
		return
	}
	server.DefaultOrgId = orgId
}

func (that *GalaxyConfig) SetUser(user User) {
	server := that.GetServer()
	if server == nil {
		return
	}
	server.User = user
}
func (that *GalaxyConfig) GetUser() User {
	server := that.GetServer()
	if server == nil {
		return User{}
	}
	return server.User
}
func (that *GalaxyConfig) GetSecret() string {
	server := that.GetServer()
	if server == nil {
		return ""
	}
	return server.Secret
}

// GetDefaultOrg 获取默认的组织信息
func (that *GalaxyConfig) GetDefaultOrg() *Organization {
	server := that.GetServer()

	if len(server.Orgs) == 0 {
		return &Organization{}
	}
	if len(server.Orgs) == 1 {
		return &server.Orgs[0]
	}
	for _, org := range server.Orgs {
		if server.DefaultOrgId == org.Id {
			return &org
		}
	}
	return &server.Orgs[0]
}

// GetOrgByName 根据组织名获取组织信息
func (that *GalaxyConfig) GetOrgByName(orgName string) *Organization {
	server := that.GetServer()
	if server == nil {
		return nil
	}
	if len(server.Orgs) == 0 {
		return nil
	}

	for _, org := range server.Orgs {
		if orgName == org.Title {
			return &org
		}
	}
	return nil
}

// GetOrgById 根据组织id获取组织信息
func (that *GalaxyConfig) GetOrgById(id uint32) *Organization {
	server := that.GetServer()
	if server == nil {
		return nil
	}
	if len(server.Orgs) == 0 {
		return nil
	}
	for _, org := range server.Orgs {
		if id == org.Id {
			return &org
		}
	}
	return nil
}
func (that *GalaxyConfig) GetOrgList() []Organization {
	server := that.GetServer()
	if server == nil {
		return nil
	}
	return server.Orgs
}
func (that *GalaxyConfig) SetOrg(org *Organization) {
	server := that.GetServer()
	if server == nil {
		return
	}
	for k, o := range server.Orgs {
		if org.Id == o.Id {
			server.Orgs[k] = *org
		}
	}
}

func (that *Organization) GetDefaultGroup() *Group {
	if len(that.Groups) == 0 {
		return nil
	}
	if len(that.Groups) == 1 {
		return &that.Groups[0]
	}
	for _, org := range that.Groups {
		if that.DefaultGroupId == org.Id {
			return &org
		}
	}
	return &that.Groups[0]
}
