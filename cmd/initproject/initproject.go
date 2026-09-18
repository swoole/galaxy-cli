package initproject

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	git2 "galaxy/pkg/git"
	utilpointer "galaxy/pkg/utils/pointer"
	"galaxy/protoc"
	"galaxy/service/group"
	"galaxy/service/organization"
	"galaxy/service/project"
	"github.com/AlecAivazis/survey/v2"
	"github.com/go-git/go-git/v5"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
	"strings"
)

type Options struct {
	orgName         *string // 组织名
	groupName       *string // 项目组名称
	remoteName      *string // 远程服务的名称
	remoteUrl       string  //远程服务的url
	defaultTitle    string  // 通过git地址解析出来的项目名称
	isGitRepository bool    // 当前目录是否是git存储库
	forceCreate     bool    // 是否强制创建新项目
	cfgFlags        *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

type existingProjectService interface {
	Basic(groupID, projectID uint32) (*protoc.ProjectBasic, error)
	Repository(groupID, projectID uint32) (*project.Repository, error)
}

// TODO 增加init的示例说明

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams:  ioStreams,
		cfgFlags:   cfgFlags,
		orgName:    utilpointer.String(""),
		groupName:  utilpointer.String(""),
		remoteName: utilpointer.String(""),
	}
}

func NewCmdInit(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "初始化项目",
		Long:    "初始化项目",
		Example: `galaxy init`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringVar(o.orgName, "org-name", "", "组织名称")
	cmd.Flags().StringVar(o.groupName, "group-name", "", "项目组名称")
	cmd.Flags().StringVar(o.remoteName, "remote-name", "", "Git远程名称")
	cmd.Flags().BoolVar(&o.forceCreate, "create", false, "强制创建新项目")
	return cmd
}

func (that *Options) Complete() error {
	gitRepo, err := git.PlainOpen(that.cfgFlags.GalaxyProjectRoot())
	if err != nil && err != git.ErrRepositoryNotExists {
		return err
	}
	if gitRepo == nil {
		that.isGitRepository = false
		return nil
	}
	that.isGitRepository = true
	remoteS, err := gitRepo.Remotes()
	if err != nil {
		return err
	}
	remoteInfo, err := that.selectedGitRemote("当前仓库有多个远程仓库,请选择你要使用的仓库:", remoteS, *that.remoteName)
	if err != nil {
		return err
	}
	if len(remoteInfo.Config().URLs) < 1 {
		return fmt.Errorf("未找到Git仓库[%s]的远程地址", remoteInfo.Config().Name)
	}
	that.remoteUrl = remoteInfo.Config().URLs[0]
	that.defaultTitle = gitRepositoryName(that.remoteUrl)

	return nil
}

func (that *Options) Validate() error {

	return nil
}

func (that *Options) Run() error {
	org, err := organization.NewService(that.cfgFlags).SelectedOrg("请选择项目所在的组织", *that.orgName)
	if err != nil {
		return err
	}
	that.cfgFlags.GalaxyConfig.SetOrgId(org.GetId())
	//that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id = org.GetId()
	// 要初始化的目录中没有 .git，或者强制创建新项目。
	if !that.isGitRepository {
		err = that.FirstInitProject()
	} else {
		err = that.ExistInitProject()
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(that.IOStreams.Out, "%s\n", "项目初始化成功")
	return nil
}

func (that *Options) FirstInitProject() error {
	selectedGroup, err := group.NewService(that.cfgFlags).SelectedGroup("请选择项目所在的项目组", *that.groupName)
	if err != nil {
		return err
	}
	projectService := project.NewService(that.cfgFlags)
	var selected *protoc.ProjectSimple
	if !that.forceCreate {
		selected, err = projectService.SelectedProjectByGroupId("请选择你要初始化的项目", selectedGroup.GetId())
		if err != nil {
			return err
		}
	}
	var configured galaxycfg.Project
	if selected != nil {
		configured, err = that.cloneExistingProject(projectService, selectedGroup.GetId(), selected.GetId())
	} else {
		configured, err = that.createAndCloneProject(projectService, selectedGroup.GetId())
	}
	if err != nil {
		return err
	}
	return cmdutil.SaveGalaxyProjectConfigForServer(that.cfgFlags.GalaxyProjectRoot(), that.cfgFlags.GetAPIServer(), configured)
}

func (that *Options) ExistInitProject() error {
	selectedGroup, err := group.NewService(that.cfgFlags).SelectedGroup("请选择项目所在的项目组", *that.groupName)
	if err != nil {
		return err
	}
	projectService := project.NewService(that.cfgFlags)
	var selected *protoc.ProjectSimple
	if !that.forceCreate {
		selected, err = projectService.SelectedProjectByGroupId("请选择当前仓库对应的项目", selectedGroup.GetId())
		if err != nil {
			return err
		}
	}
	var configured galaxycfg.Project
	if selected == nil {
		configured, err = that.createCodeProject(projectService, selectedGroup.GetId(), that.remoteUrl)
	} else {
		configured, err = that.configureExistingWorkingTree(projectService, selectedGroup.GetId(), selected)
	}
	if err != nil {
		return err
	}
	return cmdutil.SaveGalaxyProjectConfigForServer(that.cfgFlags.GalaxyProjectRoot(), that.cfgFlags.GetAPIServer(), configured)
}

func (that *Options) configureExistingWorkingTree(service existingProjectService, groupID uint32, selected *protoc.ProjectSimple) (galaxycfg.Project, error) {
	basic, err := service.Basic(groupID, selected.GetId())
	if err != nil {
		return galaxycfg.Project{}, err
	}
	if basic == nil {
		return galaxycfg.Project{}, fmt.Errorf("项目基础信息不存在")
	}
	title := basic.GetTitle()
	if title == "" {
		title = selected.GetTitle()
	}

	// 已关闭构建的项目以已有镜像为来源，不绑定 Git 仓库。当前目录即使是
	// Git 工作区，也只建立 Galaxy 项目关联，不读取或比较远程仓库地址。
	if !basic.GetDevelop() {
		return that.projectConfig(groupID, selected.GetId(), title, ""), nil
	}

	repository, err := service.Repository(groupID, selected.GetId())
	if err != nil {
		return galaxycfg.Project{}, err
	}
	if !sameGitURL(repository.CloneURL, that.remoteUrl) {
		return galaxycfg.Project{}, fmt.Errorf("所选项目绑定的仓库 %s 与当前目录远程仓库 %s 不一致", repository.CloneURL, that.remoteUrl)
	}
	return that.projectConfig(groupID, selected.GetId(), title, repository.CloneURL), nil
}

func (that *Options) cloneExistingProject(service *project.Service, groupID, projectID uint32) (galaxycfg.Project, error) {
	basic, err := service.Basic(groupID, projectID)
	if err != nil {
		return galaxycfg.Project{}, err
	}
	if basic == nil {
		return galaxycfg.Project{}, fmt.Errorf("项目基础信息不存在")
	}
	if !basic.GetDevelop() {
		return that.projectConfig(groupID, projectID, basic.GetTitle(), ""), nil
	}
	repository, err := service.Repository(groupID, projectID)
	if err != nil {
		return galaxycfg.Project{}, err
	}
	if repository.Type != project.RepositoryTypeExternal || repository.CloneURL == "" {
		return galaxycfg.Project{}, fmt.Errorf("项目没有可克隆的外部 Git 仓库")
	}
	if err = git2.GitClone(that.IOStreams, that.cfgFlags.GalaxyProjectRoot(), repository.CloneURL); err != nil {
		return galaxycfg.Project{}, err
	}
	return that.projectConfig(groupID, projectID, basic.GetTitle(), repository.CloneURL), nil
}

func (that *Options) createAndCloneProject(service *project.Service, groupID uint32) (galaxycfg.Project, error) {
	gitURL, err := service.InputGitScr()
	if err != nil {
		return galaxycfg.Project{}, err
	}
	gitURL = gstr.Trim(gitURL)
	if gitURL == "" {
		return galaxycfg.Project{}, fmt.Errorf("Git 仓库地址不能为空")
	}
	if err = git2.GitClone(that.IOStreams, that.cfgFlags.GalaxyProjectRoot(), gitURL); err != nil {
		return galaxycfg.Project{}, err
	}
	that.remoteUrl = gitURL
	that.defaultTitle = gitRepositoryName(gitURL)
	return that.createCodeProject(service, groupID, gitURL)
}

func (that *Options) createCodeProject(service *project.Service, groupID uint32, gitURL string) (galaxycfg.Project, error) {
	title, err := service.AskTitle(that.defaultTitle)
	if err != nil {
		return galaxycfg.Project{}, err
	}
	props, err := service.CreateProps(groupID)
	if err != nil {
		return galaxycfg.Project{}, err
	}
	clusterID, err := selectBuildCluster(props.BuildClusters)
	if err != nil {
		return galaxycfg.Project{}, err
	}
	provider, err := selectGitProvider(gitURL)
	if err != nil {
		return galaxycfg.Project{}, err
	}
	created, err := service.CreateProject(&project.CreateRequest{
		OrgID: that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id, GroupID: groupID, Title: title,
		Develop: true, BuildClusterID: clusterID, DefaultPort: 8080, Registries: []string{},
		Repository: project.Repository{Type: project.RepositoryTypeExternal, Provider: provider, CloneURL: gitURL},
		BuildProfile: project.BuildProfile{
			DockerfileSource: "repository", BuildContext: ".", RepositoryDockerfilePath: "Dockerfile",
			Options: map[string]interface{}{
				"mirror":    map[string]interface{}{"key": "official", "url": ""},
				"os_mirror": map[string]interface{}{"key": "aliyun", "sources": map[string]interface{}{}},
			},
		},
	})
	if err != nil {
		return galaxycfg.Project{}, err
	}
	return that.projectConfig(groupID, created.ID, created.Title, gitURL), nil
}

func (that *Options) projectConfig(groupID, projectID uint32, title, gitURL string) galaxycfg.Project {
	return galaxycfg.Project{OrgId: that.cfgFlags.GalaxyConfig.GetDefaultOrg().Id, GroupId: groupID, ProjectId: projectID, Title: title, GitSrc: gitURL}
}

func selectBuildCluster(clusters []*project.BuildCluster) (uint32, error) {
	if len(clusters) == 0 {
		return 0, fmt.Errorf("当前项目组没有可用的 BuildKit 构建集群")
	}
	if len(clusters) == 1 {
		return clusters[0].ID, nil
	}
	options := make([]string, len(clusters))
	for index, cluster := range clusters {
		options[index] = cluster.Title
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: "请选择构建集群:", Options: options}, &selected); err != nil {
		return 0, err
	}
	return clusters[selected].ID, nil
}

func selectGitProvider(gitURL string) (uint32, error) {
	lowerURL := gstr.ToLower(gitURL)
	for host, provider := range map[string]uint32{"github.com": 1, "gitee.com": 2, "gitlab": 3, "gitea": 4} {
		if gstr.Contains(lowerURL, host) {
			return provider, nil
		}
	}
	options := []string{"GitHub", "码云", "GitLab", "Gitea"}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: "请选择 Git 服务类型:", Options: options}, &selected); err != nil {
		return 0, err
	}
	return uint32(selected + 1), nil
}

func sameGitURL(left, right string) bool {
	normalize := func(value string) string {
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		return strings.TrimSuffix(value, ".git")
	}
	return normalize(left) == normalize(right)
}

func gitRepositoryName(gitURL string) string {
	parts := gstr.Split(gstr.TrimRight(gitURL, "/"), "/")
	name := parts[len(parts)-1]
	if gstr.Contains(name, ":") {
		colonParts := gstr.Split(name, ":")
		name = colonParts[len(colonParts)-1]
	}
	return strings.TrimSuffix(name, ".git")
}

// 选择远程地址
func (that *Options) selectedGitRemote(msg string, remotes []*git.Remote, remoteName string) (*git.Remote, error) {

	if len(remotes) == 1 {
		remote := remotes[0]
		if len(remoteName) > 0 && remoteName != remote.Config().Name {
			return nil, fmt.Errorf("未找到名为[%s]的 Git远程名称", remoteName)
		}
		return remotes[0], nil
	}
	titles := make([]string, len(remotes))
	for i, m := range remotes {
		titles[i] = fmt.Sprintf("%s(%s)", m.Config().Name, gstr.Join(m.Config().URLs, ";"))
		if len(remoteName) > 0 && m.Config().Name == remoteName {
			return m, nil
		}
	}
	answerIndex := 0
	err := survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return remotes[answerIndex], nil
}
