package autocompletion

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/completion"
	"galaxy/pkg/galaxycfg"
	"github.com/fatih/color"
	"github.com/gogf/gf/os/gfile"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"syscall"
)

type Options struct {
	completion *completion.Completion
	cfgFlags   *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func NewCmdAutoCompletion(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:     "autocompletion",
		Short:   "生成并保存自动补全脚本",
		Example: `galaxy autocompletion`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "windows" {
		that.completion = completion.NewCompletion()
	}
	return nil
}

func (that *Options) Validate() error {
	if runtime.GOOS != "windows" {
		if syscall.Getppid() == 1 {
			return nil
		}
		u, err := user.Current()
		if err != nil {
			return err
		}
		if u.Username != "root" {
			//fmt.Printf("Auto completion 命令必须使用root用户运行,你可以执行 %s\n", color.GreenString(fmt.Sprintf("sudo galaxy autocompletion && source %s", that.completion.GetPath())))
			err = rootAutoCompletion()
			if err != nil {
				return err
			}
			return cmdutil.ErrExit
		}
	}

	return nil
}

func (that *Options) Run() error {
	if runtime.GOOS != "windows" {
		_, err := that.completion.Save()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(that.Out, "生成并保存自动补全脚本完成,为了实时看到效果你应该执行命令  %s \n", color.GreenString(fmt.Sprintf("source %s", that.completion.GetPath())))
	}
	if runtime.GOOS == "windows" {
		_, _ = fmt.Fprintf(that.Out, "自动补全功能支持 PowerShell 请确保在PowerShell执行以下命令:\n")
		_, _ = fmt.Fprintf(that.Out, "请执行命令: ` %s `\n", color.GreenString(fmt.Sprintf("%s completion powershell >> $PROFILE", gfile.SelfPath())))
		_, _ = fmt.Fprintf(that.Out, "如果需要立即生效,再次执行命令: ` %s `\n", color.YellowString(fmt.Sprintf("%s completion powershell | Out-String | Invoke-Expression", gfile.SelfPath())))
	}
	return nil
}

func rootAutoCompletion() error {
	if syscall.Getppid() == 1 {
		return nil
	}
	filePath := gfile.SelfPath()

	arg0, e := exec.LookPath(filePath)
	if e != nil {
		return e
	}

	cmd := exec.Command("sudo", arg0, "autocompletion")
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
