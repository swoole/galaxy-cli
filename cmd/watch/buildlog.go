package watch

import (
	"fmt"
	"galaxy/pkg/errcode"
	"galaxy/protoc"
	"galaxy/service/build"
)

func (that *Options) BuildLogOutput() error {
	project := that.cfgFlags.ProjectConfig.DefaultProject()
	if project == nil {
		return fmt.Errorf(errcode.ErrorNotInitProject)
	}
	if that.buildId <= 0 {
		req := protoc.BuildListReq{
			OrgId:     project.OrgId,
			GroupId:   project.GroupId,
			ProjectId: project.ProjectId,
		}
		svr := build.NewService(that.cfgFlags)
		list, err := svr.BuildList(&req)
		if err != nil {
			return err
		}
		if len(list.GetData()) < 1 {
			return fmt.Errorf("未找到构建记录")
		}
		buildInfo, err := svr.SelectedBuild("请选择你要监视的构建日志:", list.GetData())
		if err != nil {
			return err
		}
		that.buildId = int32(buildInfo.GetId())
	}
	req := protoc.BuildLogReq{
		OrgId:     project.OrgId,
		GroupId:   project.GroupId,
		ProjectId: project.ProjectId,
		BuildId:   uint32(that.buildId),
	}
	var log = make(chan []byte, 512)
	err := build.NewService(that.cfgFlags).BuildLogWatch(&req, log)
	if err != nil {
		return err
	}
	for {
		select {
		case m, ok := <-log:
			if !ok {
				return nil
			}
			_, _ = fmt.Fprint(that.Out, string(m))
		}
	}
}
