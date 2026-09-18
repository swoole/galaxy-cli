package logout

import (
	"fmt"
	cmdutil "galaxy/cmd/util"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	"galaxy/protoc"
	"github.com/gogf/gf/os/gfile"
	"github.com/spf13/cobra"
)

type Options struct {
	cfgFlags *galaxycfg.ConfigFlags
	galaxycfg.IOStreams
}

func newOptions(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		cfgFlags:  cfgFlags,
	}
}

func NewCmdLogout(cfgFlags *galaxycfg.ConfigFlags, ioStreams galaxycfg.IOStreams) *cobra.Command {
	o := newOptions(cfgFlags, ioStreams)
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "退出 CodeGalaxy 平台登录",
		Example: `	galaxy logout`,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (that *Options) Complete(cmd *cobra.Command, args []string) error {

	return nil
}

func (that *Options) Validate() error {

	return nil
}

func (that *Options) Run() error {
	if !gfile.Exists(that.cfgFlags.GetGalaxyConfigFile()) {
		_, _ = fmt.Fprintln(that.Out, "Logout Success.")
	}
	err := that.logout()
	if err != nil {
		return err
	}
	err = gfile.Remove(that.cfgFlags.GetGalaxyConfigFile())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(that.Out, "Logout Success.")
	return nil
}

// Logout 退出登录
func (that *Options) logout() error {
	err := httpclient.NewHttpClient(that.cfgFlags).Post("/logout", "{}", &protoc.Response{})
	if err != nil {
		return err
	}
	return nil
}
