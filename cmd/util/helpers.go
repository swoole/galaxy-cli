package util

import (
	"bytes"
	"fmt"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/os/glog"
	"net/url"
	"os"
	"strings"
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
