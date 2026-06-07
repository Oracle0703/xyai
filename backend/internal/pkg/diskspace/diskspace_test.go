package diskspace

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageOnTempDir(t *testing.T) {
	total, free, err := Usage(t.TempDir())
	require.NoError(t, err)
	require.Greater(t, total, uint64(0))
	require.Greater(t, free, uint64(0))
	require.LessOrEqual(t, free, total)
}

func TestUsageOnMissingPath(t *testing.T) {
	_, _, err := Usage(`Z:\definitely\not\exists\path-for-diskspace-test`)
	require.Error(t, err)
}
