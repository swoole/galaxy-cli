package upgrade

import (
	"galaxy/protoc"
	"github.com/gogf/gf/test/gtest"
	"testing"
)

func TestUpgrade_Download(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		info := &protoc.UpgradeInfo{
			Version:   "v1.0.0",
			Notes:     "版本",
			Md5:       "c6b9bc61e5a8b142cf5469fea56b3531",
			Os:        "linux",
			Arch:      "amd64",
			Size:      18737320,
			Download:  "http://localhost/galaxy-1.0.0-linux-x86_64",
			CreatedAt: 1665299920,
		}
		up := NewUpgrade(info)
		err := up.Download()
		t.Assert(err, nil)
	})
}
