package upgrade

import (
	"fmt"
	"galaxy/protoc"
	"github.com/gogf/gf/crypto/gmd5"
	"github.com/gogf/gf/errors/gcode"
	"github.com/gogf/gf/errors/gerror"
	"github.com/gogf/gf/frame/g"
	"github.com/gogf/gf/os/gfile"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"io"
	"os"
	"time"
)

type Upgrade struct {
	versionInfo *protoc.UpgradeInfo
	tmpFile     string
	self        string
}

func NewUpgrade(info *protoc.UpgradeInfo) *Upgrade {
	return &Upgrade{
		versionInfo: info,
		self:        gfile.SelfPath(),
	}
}

// Download 下载文件包
func (that *Upgrade) Download() error {
	fmt.Printf("下载: %s\n", that.versionInfo.Download)
	response, err := g.Client().Get(that.versionInfo.Download)
	if err != nil {
		return err
	}
	defer response.Close()
	if response.StatusCode != 200 {
		return gerror.NewCode(gcode.New(response.StatusCode, fmt.Sprintf("下载[%s]失败", that.versionInfo.Download), nil))
	}
	p := mpb.New(
		mpb.WithWidth(60),
		mpb.WithRefreshRate(200*time.Millisecond),
	)
	bar := p.New(response.ContentLength,
		mpb.BarStyle().Rbound("|"),
		mpb.PrependDecorators(
			decor.CountersKibiByte("% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_GO, 90),
			decor.Name(" ] "),
			decor.EwmaSpeed(decor.UnitKiB, "% .2f", 60),
		),
	)
	proxyReader := bar.ProxyReader(response.Body)
	defer proxyReader.Close()
	that.tmpFile = fmt.Sprintf("%s/galaxy_%s.tmp", gfile.TempDir(), that.versionInfo.GetVersion())
	if gfile.Exists(that.tmpFile) {
		err = gfile.Remove(that.tmpFile)
		if err != nil {
			return err
		}
	}
	f, err := gfile.Create(that.tmpFile)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, proxyReader)
	if err != nil {
		return err
	}
	p.Wait()
	return nil
}

func (that *Upgrade) Check() error {

	hash := gmd5.MustEncryptFile(that.tmpFile)
	if hash != that.versionInfo.Md5 {
		return gerror.New("版本校验失败")
	}
	return nil
}

func (that *Upgrade) Replace() error {
	err := gfile.Move(that.tmpFile, that.self)
	if err != nil {
		return err
	}
	err = gfile.Chmod(that.self, os.FileMode(0755))
	return err
}
