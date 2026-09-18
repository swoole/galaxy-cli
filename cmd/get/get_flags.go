package get

import (
	"fmt"
	"galaxy/cmd/util"
	"github.com/spf13/cobra"
	"strings"
)

type PrintFlags struct {
	NoHeaders    *bool
	OutputFormat *string
}

func NewGetPrintFlags() *PrintFlags {
	outputFormat := ""
	noHeaders := false

	return &PrintFlags{
		OutputFormat: &outputFormat,
		NoHeaders:    &noHeaders,
	}
}

// AllowedFormats is the list of formats in which data can be displayed
func (f *PrintFlags) AllowedFormats() []string {

	return nil
}

func (f *PrintFlags) AddFlags(cmd *cobra.Command) {
	if f.OutputFormat != nil {
		cmd.Flags().StringVarP(f.OutputFormat, "output", "o", *f.OutputFormat, fmt.Sprintf(`Output format. One of: (%s).`, strings.Join(f.AllowedFormats(), ", ")))
		util.CheckErr(cmd.RegisterFlagCompletionFunc(
			"output",
			func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				var comps []string
				for _, format := range f.AllowedFormats() {
					if strings.HasPrefix(format, toComplete) {
						comps = append(comps, format)
					}
				}
				return comps, cobra.ShellCompDirectiveNoFileComp
			},
		))
	}
	if f.NoHeaders != nil {
		cmd.Flags().BoolVar(f.NoHeaders, "no-headers", *f.NoHeaders, "当使用默认或自定义列输出格式时，不要打印标题(默认打印标题)。")
	}
}
