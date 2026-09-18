package util

import (
	"bytes"
	"fmt"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/glog"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/yaml"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultErrorExitCode = 1
)

func fatal(msg string, code int) {
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		_, _ = fmt.Fprint(os.Stderr, msg)
	}
	os.Exit(code)
}

var ErrExit = fmt.Errorf("exit")

var fatalErrHandler = fatal

func CheckErr(err error) {
	checkErr(err, fatalErrHandler)
}

func checkErr(err error, handleErr func(string, int)) {
	if err == nil {
		return
	}
	switch {
	case err == ErrExit:
		handleErr("", DefaultErrorExitCode)
	default: // for any other error type
		msg, ok := StandardErrorMessage(err)
		if !ok {
			msg = err.Error()
			if !strings.HasPrefix(msg, "error: ") {
				msg = fmt.Sprintf("error: %s", msg)
			}
		}
		handleErr(msg, DefaultErrorExitCode)
	}
}
func StandardErrorMessage(err error) (string, bool) {
	switch t := err.(type) {
	case *url.Error:
		glog.Infof("Connection error: %s %s: %v", t.Op, t.URL, t.Err)
		switch {
		case strings.Contains(t.Err.Error(), "connection refused"):
			host := t.URL
			if server, err := url.Parse(t.URL); err == nil {
				host = server.Host
			}
			return fmt.Sprintf("The connection to the server %s was refused - did you specify the right host or port?", host), true
		}
		return fmt.Sprintf("Unable to connect to the server: %v", t.Err), true
	}
	return "", false
}

func GetFlagBool(cmd *cobra.Command, flag string) bool {
	b, err := cmd.Flags().GetBool(flag)
	if err != nil {
		glog.Fatalf("error accessing flag %s for command %s: %v", flag, cmd.Name(), err)
	}
	return b
}
func GetFlagDuration(cmd *cobra.Command, flag string) time.Duration {
	d, err := cmd.Flags().GetDuration(flag)
	if err != nil {
		glog.Fatalf("error accessing flag %s for command %s: %v", flag, cmd.Name(), err)
	}
	return d
}
func UsageErrorf(cmd *cobra.Command, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s\nSee '%s -h' for help and examples", msg, cmd.CommandPath())
}

func AddPodRunningTimeoutFlag(cmd *cobra.Command, defaultTimeout time.Duration) {
	cmd.Flags().Duration("pod-running-timeout", defaultTimeout, "The length of time (like 5s, 2m, or 3h, higher than zero) to wait until at least one pod is running")
}
func AddContainerVarFlags(cmd *cobra.Command, p *string, containerName string) {
	cmd.Flags().StringVarP(p, "container", "c", containerName, "Container name. If omitted, use the kubectl.kubernetes.io/default-container annotation for selecting the container to be attached or the first container in the pod will be chosen")
}

func GetPodRunningTimeoutFlag(cmd *cobra.Command) (time.Duration, error) {
	timeout := GetFlagDuration(cmd, "pod-running-timeout")
	if timeout <= 0 {
		return timeout, fmt.Errorf("--pod-running-timeout must be higher than zero")
	}
	return timeout, nil
}

// SaveGalaxyProjectConfig persists an already-resolved Project mapping without
// trying to infer Project identity from a repository URL.
func SaveGalaxyProjectConfig(projectRoot string, projects ...galaxycfg.Project) error {
	return SaveGalaxyProjectConfigForServer(projectRoot, "", projects...)
}

// SaveGalaxyProjectConfigForServer persists the project mapping together with
// the API Server used to resolve it. Subsequent commands can therefore select
// the matching credentials without requiring --server on every invocation.
func SaveGalaxyProjectConfigForServer(projectRoot, server string, projects ...galaxycfg.Project) error {
	if len(projects) == 0 {
		return gerror.New("没有可保存的项目信息")
	}
	for index := range projects {
		if projects[index].ApiVersion == "" {
			projects[index].ApiVersion = buildVariable.ApiVersion
		}
	}
	config := &galaxycfg.ProjectConfig{Server: strings.TrimRight(server, "/"), Projects: projects}
	return config.Save(projectRoot)
}

// ManualStrip is used for dropping comments from a YAML file
func ManualStrip(file []byte) []byte {
	stripped := []byte{}
	lines := bytes.Split(file, []byte("\n"))
	for i, line := range lines {
		trimline := bytes.TrimSpace(line)

		if bytes.HasPrefix(trimline, []byte("#")) && !bytes.HasPrefix(trimline, []byte("#!")) {
			continue
		}
		stripped = append(stripped, line...)
		if i < len(lines)-1 {
			stripped = append(stripped, '\n')
		}
	}
	return stripped
}

// StripComments will transform a YAML file into JSON, thus dropping any comments
// in it. Note that if the given file has a syntax error, the transformation will
// fail and we will manually drop all comments from the file.
func StripComments(file []byte) []byte {
	stripped := file
	stripped, err := yaml.ToJSON(stripped)
	if err != nil {
		stripped = ManualStrip(file)
	}
	return stripped
}
