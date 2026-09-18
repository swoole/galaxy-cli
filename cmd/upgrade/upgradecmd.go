package upgrade

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/buildVariable"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/frame/g"
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/os/gtime"
	"github.com/gogf/gf/text/gstr"
	"github.com/spf13/cobra"
)

type Options struct {
	version  string
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func NewOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		cfgFlags:  cfgFlags,
		IOStreams: ioStreams,
	}
}

func NewCmdUpgrade(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := NewOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "升级 galaxy 命令",
		Example: `	galaxy upgrade`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		that.version = args[0]
	}
	return nil
}

func (that *Options) Validate() error {
	if !gfile.IsWritable(gfile.SelfDir()) {
		return fmt.Errorf("当前用户没有写入 %s 的权限,请执行 `%s` 命令进行升级.", color.YellowString(gfile.SelfPath()), color.GreenString("sudo galaxy upgrade"))
	}
	return nil
}

func (that *Options) Run() error {
	var upgradeInfo *protoc.UpgradeInfo
	err := httpclient.NewHttpClient(that.cfgFlags).Post("/cli/upgrade",
		g.Map{"os": buildVariable.OS, "arch": buildVariable.ARCH, "version": that.version},
		&upgradeInfo)
	//err := httpclient.NewHttpClient(that.cfgFlags).Post("/cli/upgrade",
	//	g.Map{"os": "linux", "arch": "amd64", "version": "v0.0.2"},
	//	&upgradeInfo)
	if err != nil {
		if e, ok := err.(*gerror.Error); ok {
			if e.Code().Code() == 110001 {
				_, _ = fmt.Fprintf(that.Out, "%s\n", e.Code().Message())
				return nil
			}
		}
		return err
	}
	// 如果要更新的版本不是测试版本
	if upgradeInfo.Version != "test" {
		n := gstr.CompareVersion(upgradeInfo.Version, buildVariable.BuildVersion)
		if n == 0 {
			_, _ = fmt.Fprintf(that.Out, "你的版本%s已经是最新版本\n", buildVariable.BuildVersion)
			return nil
		}
	}
	_, _ = fmt.Fprintf(that.Out, "开始升级版本: %s\n", upgradeInfo.Version)
	_, _ = fmt.Fprintf(that.Out, "Md5: %s\n", upgradeInfo.Md5)
	_, _ = fmt.Fprintf(that.Out, "Size: %s\n", humanize.Bytes(uint64(upgradeInfo.Size)))
	_, _ = fmt.Fprintf(that.Out, "发布时间: %s\n", gtime.NewFromTimeStamp(upgradeInfo.CreatedAt).String())
	_, _ = fmt.Fprintf(that.Out, "版本说明: \n%s\n", upgradeInfo.Notes)
	up := NewUpgrade(upgradeInfo)
	err = up.Download()
	if err != nil {
		return err
	}
	err = up.Check()
	if err != nil {
		return err
	}
	err = up.Replace()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(that.Out, "升级完成")
	return nil
}
