package galaxycfg

import (
	"fmt"
	"github.com/gogf/gf/encoding/gjson"
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/text/gstr"
	"github.com/jiangxin/goconfig"
)

type ProjectConfig struct {
	projectRoot       string    // 项目根目录
	projectFile       string    // 项目配置文件
	IsLoadProjectFile bool      // 是否载入配置文件
	Server            string    `yaml:"Server"`           // 当前项目使用的 Galaxy API Server
	DefaultProjectId  uint32    `yaml:"DefaultProjectId"` // 默认项目
	Projects          []Project `yaml:"Projects"`         // 项目信息
}

type Project struct {
	ApiVersion string `yaml:"ApiVersion"`
	OrgId      uint32 `yaml:"OrgId"`   // 组织ID
	GroupId    uint32 `yaml:"GroupId"` // 项目组 ID
	ProjectId  uint32 `yaml:"ProjectId"`
	Title      string `yaml:"Title"`
	GitSrc     string `yaml:"GitSrc"` // 仓库地址
}

func NewProjectGalaxy() *ProjectConfig {
	return &ProjectConfig{
		IsLoadProjectFile: false,
	}
}

func (that *ProjectConfig) init() {
	//如果
	if len(that.projectRoot) == 0 {
		var projectRoot string
		gitCfgFile, _ := goconfig.FindGitConfig(gfile.Pwd())
		if len(gitCfgFile) > 0 {
			projectRoot = gstr.Replace(gitCfgFile, "/.git/config", "")
		} else {
			projectRoot = gfile.Pwd()
		}
		that.projectRoot = projectRoot
	}
	that.projectFile = fmt.Sprintf("%s%s.galaxy%sproject.yaml", gstr.TrimRight(that.projectRoot, gfile.Separator), gfile.Separator, gfile.Separator)

}

func (that *ProjectConfig) Load(projectRoot string) error {
	that.projectRoot = projectRoot
	that.init()
	if !gfile.Exists(that.projectFile) {
		return nil
	}
	json, err := gjson.LoadYaml(gfile.GetBytes(that.projectFile))
	if err != nil {
		return err
	}
	that.IsLoadProjectFile = true
	return json.Struct(that)
}

// DefaultProject 返回默认项目。
func (that *ProjectConfig) DefaultProject() *Project {
	for _, project := range that.Projects {
		if that.DefaultProjectId == project.ProjectId {
			return &project
		}
	}
	return nil
}

// InProject 判断当前目录是否为项目目录。
func (that *ProjectConfig) InProject(projectRoot string) bool {
	that.projectRoot = projectRoot
	that.init()
	return gfile.Exists(that.projectFile)
}

func (that *ProjectConfig) Save(projectRoot string) error {
	that.projectRoot = projectRoot
	that.init()
	cfg := gjson.New("")
	if that.Server != "" {
		_ = cfg.Set("Server", gstr.TrimRight(that.Server, "/"))
	}
	//_ = cfg.Set("ApiVersion", buildVariable.ApiVersion)
	//_ = cfg.Set("OrgId", that.OrgId)
	//_ = cfg.Set("GitSrc", that.GitSrc)
	if len(that.Projects) > 0 && that.DefaultProjectId == 0 {
		_ = cfg.Set("DefaultProjectId", that.Projects[0].ProjectId)
	} else {
		_ = cfg.Set("DefaultProjectId", that.DefaultProjectId)
	}
	_ = cfg.Set("Projects", that.Projects)
	galaxyDir := gstr.Replace(that.projectFile, fmt.Sprintf("%sproject.yaml", gfile.Separator), "")
	if !gfile.IsDir(galaxyDir) {
		if err := gfile.Mkdir(galaxyDir); err != nil {
			return err
		}
	}
	return gfile.PutContents(that.projectFile, cfg.MustToYamlString())
}

func (that *ProjectConfig) GetProjectByName(name string) *Project {
	if len(name) == 0 {
		return nil
	}
	for _, project := range that.Projects {
		if project.Title == name {
			return &project
		}
	}
	return nil
}

// Remove 移除配置文件
func (that *ProjectConfig) Remove(projectRoot string) error {
	that.projectRoot = projectRoot
	that.init()
	return gfile.Remove(that.projectFile)
}

// RemoveProject 移除项目配置。
func (that *ProjectConfig) RemoveProject(projectRoot string, projectId uint32) error {
	that.projectRoot = projectRoot
	that.init()
	if !gfile.Exists(that.projectFile) {
		return nil
	}
	projects := that.Projects
	if len(projects) > 1 {
		for k, a := range projects {
			if a.ProjectId == projectId {
				that.Projects = append(that.Projects[0:k], that.Projects[k:]...)
			}
		}
	} else {
		if projects[0].ProjectId == projectId {
			that.Projects = nil
		}
	}

	if len(that.Projects) == 0 {
		return that.Remove(projectRoot)
	}
	that.DefaultProjectId = 0
	return that.Save(projectRoot)
}
