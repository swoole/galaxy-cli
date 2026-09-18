package buildVariable

import (
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	ApiVersion = "v1"
)

var (
	BuildVersion   = "No Version Info"
	BuildGoVersion = runtime.Version()
	BuildTime      = "No Time Info"
	GitVersion     = "No Version Info"
	GitCommit      = "No Version Info"
	GitTreeState   = "No Version Info"
	AgentImage     = ""
	Compiler       = runtime.Compiler
	OS             = runtime.GOOS
	ARCH           = runtime.GOARCH
	CopyRight      = "上海识沃网络科技有限公司"
	Logo           = `
  _______      ___       __          ___      ___   ___ ____    ____ 
 /  _____|    /   \     |  |        /   \     \  \ /  / \   \  /   / 
|  |  __     /  ^  \    |  |       /  ^  \     \  V  /   \   \/   /  
|  | |_ |   /  /_\  \   |  |      /  /_\  \     >   <     \_    _/   
|  |__| |  /  _____  \  |  '----./  _____  \   /  .  \      |  |
 \______| /__/     \__\ |_______/__/     \__\ /__/ \__\     |__|
`
)

type VersionInfo struct {
	Version       string
	GitVersion    string
	GitCommit     string
	GitCommitTime string
	GitTreeState  string
	GoVersion     string
	Compiler      string
	BuildTime     string
	OS            string
	Arch          string
	Copyright     string
}

// Info returns complete version metadata. Release builds can override every
// field with -ldflags; development builds fall back to Go's embedded VCS data.
func Info() VersionInfo {
	info := VersionInfo{
		Version: BuildVersion, GitVersion: GitVersion, GitCommit: GitCommit,
		GitTreeState: GitTreeState, GoVersion: BuildGoVersion, Compiler: Compiler,
		BuildTime: BuildTime, OS: OS, Arch: ARCH, Copyright: CopyRight,
	}
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	settings := make(map[string]string, len(buildInfo.Settings))
	for _, setting := range buildInfo.Settings {
		settings[setting.Key] = setting.Value
	}
	revision := settings["vcs.revision"]
	info.GitCommitTime = settings["vcs.time"]
	if missing(info.GitCommit) && revision != "" {
		info.GitCommit = revision
	}
	if missing(info.GitTreeState) {
		if settings["vcs.modified"] == "true" {
			info.GitTreeState = "dirty"
		} else if revision != "" {
			info.GitTreeState = "clean"
		}
	}
	if missing(info.Version) {
		switch {
		case buildInfo.Main.Version != "" && buildInfo.Main.Version != "(devel)":
			info.Version = buildInfo.Main.Version
		case revision != "":
			info.Version = "devel-" + shortRevision(revision)
		default:
			info.Version = "development"
		}
	}
	if missing(info.GitVersion) {
		info.GitVersion = info.Version
	}
	return info
}

func missing(value string) bool {
	return value == "" || strings.HasPrefix(value, "No ")
}

func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
