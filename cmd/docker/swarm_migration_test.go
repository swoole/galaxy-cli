package docker

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfirmSwarmMigrationOnlyAcceptsFullYes(t *testing.T) {
	var output bytes.Buffer

	err := confirmSwarmMigration(strings.NewReader("yes\n"), &output, "迁移 mysql", false)

	require.NoError(t, err)
	require.Contains(t, output.String(), "迁移确认")
	require.Contains(t, output.String(), "╭")
	require.Contains(t, output.String(), "确认通过，开始迁移")
}

func TestConfirmSwarmMigrationRejectsOtherInput(t *testing.T) {
	for _, answer := range []string{"\n", "y\n", "no\n", "YES-NOW\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			var output bytes.Buffer

			err := confirmSwarmMigration(strings.NewReader(answer), &output, "迁移 mysql", false)

			require.ErrorIs(t, err, errSwarmMigrationCancelled)
			require.Contains(t, output.String(), "未修改 Docker 资源")
		})
	}
}

func TestConfirmSwarmMigrationCanBeExplicitlySkipped(t *testing.T) {
	var output bytes.Buffer

	err := confirmSwarmMigration(nil, &output, "迁移 mysql", true)

	require.NoError(t, err)
	require.Contains(t, output.String(), "--yes")
}

func TestConfirmationReadsDoNotConsumeFollowingAnswer(t *testing.T) {
	input := strings.NewReader("yes\nyes\n")
	var output bytes.Buffer

	require.NoError(t, confirmSwarmMigration(input, &output, "迁移 mysql", false))
	answer, err := readConfirmationLine(input)

	require.NoError(t, err)
	require.Equal(t, "yes", answer)
}
