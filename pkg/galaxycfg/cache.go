package galaxycfg

import (
	"fmt"
	"github.com/gogf/gf/container/gvar"
	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/os/glog"
	"github.com/gogf/gf/util/gconv"
)

const (
	BuildPipeLineLatestSelected = "BuildPipeLineLatestSelected"
)

type OptionsCache struct {
	cfgFlags *ConfigFlags
}

func NewOptionsCache(cfgFlags *ConfigFlags) *OptionsCache {
	return &OptionsCache{
		cfgFlags: cfgFlags,
	}
}

func (that *OptionsCache) Get(key string, defaultVal ...interface{}) *gvar.Var {
	path := fmt.Sprintf("%s%s%s", that.getCacheDir(), gfile.Separator, key)
	if !gfile.Exists(path) {
		if len(defaultVal) > 0 {
			return gvar.New(defaultVal[0])
		}
		return gvar.New(nil)
	}
	content := gfile.GetContents(path)
	if len(content) > 0 {
		return gvar.New(content)
	}
	if len(defaultVal) > 0 {
		return gvar.New(defaultVal[0])
	}
	return gvar.New(nil)
}

func (that *OptionsCache) Set(key string, val interface{}) {
	cacheDir := that.getCacheDir()
	if !gfile.Exists(cacheDir) {
		err := gfile.Mkdir(cacheDir)
		if err != nil {
			glog.Warning(err)
			return
		}
	}
	path := fmt.Sprintf("%s%s%s", cacheDir, gfile.Separator, key)
	if !gfile.IsWritable(cacheDir) {
		glog.Warning(fmt.Errorf("缓存目录 %s 不可写", cacheDir))
		return
	}
	err := gfile.PutContents(path, gconv.String(val))
	if err != nil {
		glog.Warning(err)
	}
	return
}

func (that *OptionsCache) getCacheDir() string {
	return fmt.Sprintf("%s%s.galaxy%scache", that.cfgFlags.GalaxyProjectRoot(), gfile.Separator, gfile.Separator)
}
