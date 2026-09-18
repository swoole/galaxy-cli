package cmd

import (
	"github.com/spf13/pflag"
	"os"
	"runtime"
	"runtime/pprof"
)

var (
	profileName   string
	profileOutput string
)

func addProfilingFlags(flags *pflag.FlagSet) {
	flags.StringVar(&profileName, "profile", "none", "要捕获profile的文件名. 参数为这些选项中的其中一种(none|cpu|heap|goroutine|threadcreate|block|mutex)")
	flags.StringVar(&profileOutput, "profile-output", "profile.pprof", "捕获profile数据后要保存的文件名")
}

func initProfiling() error {

	return nil
}

func flushProfiling() error {
	switch profileName {
	case "none":
		return nil
	case "cpu":
		pprof.StopCPUProfile()
	case "heap":
		runtime.GC()
		fallthrough
	default:
		profile := pprof.Lookup(profileName)
		if profile == nil {
			return nil
		}
		f, err := os.Create(profileOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		profile.WriteTo(f, 0)
	}

	return nil
}
