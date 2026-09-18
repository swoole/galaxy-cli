package version

import (
	"errors"
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/utils/templates"
	"github.com/fatih/color"
	"github.com/gogf/gf/encoding/gjson"
	"github.com/gogf/gf/frame/g"
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/text/gstr"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

type Version struct {
}

var (
	versionExample = templates.Examples(`
		# Print the client versions for the current context
		galaxy version`)
)

type Options struct {
	Output string

	args []string
	galaxycfg.IOStreams
}

func newOptions(ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
	}
}

func NewCmdVersion(ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(ioStreams)
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "打印客户端版本信息.",
		Long:    "打印当前上下文的客户端版本信息.",
		Example: versionExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringVarP(&o.Output, "output", "o", o.Output, "输入 'yaml' 或 'json'.")
	return cmd
}

// Complete completes all the required options
func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	that.args = args
	return nil
}

func (that *Options) Validate() error {
	if len(that.args) != 0 {
		return errors.New(fmt.Sprintf("多余的参数: %v", that.args))
	}

	if that.Output != "" && that.Output != "yaml" && that.Output != "json" {
		return errors.New(`--output 参数必须传入 'yaml' 或 'json'`)
	}

	return nil
}

func (that *Options) Run() error {
	versionInfo := buildVariable.Info()
	info := gjson.New(g.Map{
		"Version":       versionInfo.Version,
		"GitVersion":    versionInfo.GitVersion,
		"GitCommit":     versionInfo.GitCommit,
		"GitCommitTime": versionInfo.GitCommitTime,
		"GitTreeState":  versionInfo.GitTreeState,
		"GoVersion":     versionInfo.GoVersion,
		"Compiler":      versionInfo.Compiler,
		"BuildTime":     versionInfo.BuildTime,
		"OS":            versionInfo.OS,
		"Arch":          versionInfo.Arch,
		"Copyright":     versionInfo.Copyright,
	})
	switch that.Output {
	case "":
		_, _ = fmt.Fprintf(that.Out, "%s", color.YellowString(gstr.TrimLeftStr(buildVariable.Logo, "\n")))
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Version", 15), versionInfo.Version)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Git Version", 15), versionInfo.GitVersion)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Git Commit", 15), versionInfo.GitCommit)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Git Commit Time", 15), versionInfo.GitCommitTime)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Git Tree State", 15), versionInfo.GitTreeState)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Go Version", 15), versionInfo.GoVersion)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Compiler", 15), versionInfo.Compiler)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Build Time", 15), versionInfo.BuildTime)
		_, _ = fmt.Fprintf(that.Out, "%s: %s/%s\n", text.AlignLeft.Apply("Platform", 15), versionInfo.OS, versionInfo.Arch)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Copyright", 15), versionInfo.Copyright)
		_, _ = fmt.Fprintf(that.Out, "%s: %s\n", text.AlignLeft.Apply("Install Path", 15), gfile.SelfPath())
	case "yaml":
		_, _ = fmt.Fprintln(that.Out, info.MustToYamlString())
	case "json":
		_, _ = fmt.Fprintln(that.Out, info.MustToJsonString())
	default:
		// There is a bug in the program if we hit this case.
		// However, we follow a policy of never panicking.
		return fmt.Errorf("VersionOptions were not validated: --output=%q should have been rejected", that.Output)
	}
	return nil
}
