package file

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"galaxy/pkg/hostctl/types"
)

func TestAddRouteAndFlush(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/hosts", []byte("127.0.0.1 localhost\n"), 0644))

	hosts, err := NewWithFs("/hosts", fs)
	require.NoError(t, err)
	require.NoError(t, hosts.AddRoute("galaxy", types.NewRoute("10.0.0.8", "app.example.com")))
	require.NoError(t, hosts.Flush())

	content, err := afero.ReadFile(fs, "/hosts")
	require.NoError(t, err)
	require.Contains(t, string(content), "127.0.0.1 localhost")
	require.Contains(t, string(content), "# profile.on galaxy")
	require.Contains(t, string(content), "10.0.0.8 app.example.com")
}
