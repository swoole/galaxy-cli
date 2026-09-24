package upgrade

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"galaxy/protoc"
)

func TestUpgradeDownload(t *testing.T) {
	payload := []byte("galaxy test binary")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", "18")
		_, _ = writer.Write(payload)
	}))
	defer server.Close()

	upgrade := NewUpgrade(&protoc.UpgradeInfo{Version: "test", Download: server.URL})
	t.Cleanup(func() { _ = os.Remove(upgrade.tmpFile) })

	require.NoError(t, upgrade.Download())
	content, err := os.ReadFile(upgrade.tmpFile)
	require.NoError(t, err)
	require.Equal(t, payload, content)
}
