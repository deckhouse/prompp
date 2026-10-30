package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidExpensiveQueryConfiguration(t *testing.T) {
	// Given
	dataDir := t.TempDir()
	filename := filepath.Join(dataDir, "expensive_queries.yaml")
	require.NoError(t, os.WriteFile(filename, []byte("high_samples: 0\n"), 0o600))
	configFile := filepath.Join(t.TempDir(), "prometheus.yml")
	require.NoError(t, os.WriteFile(configFile, []byte("scrape_configs: []\n"), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), startupTime)
	defer cancel()
	prom := exec.CommandContext(ctx, promPath, "-test.main",
		"--config.file="+configFile, "--storage.tsdb.path="+dataDir, "--web.listen-address=127.0.0.1:0")

	// When
	output, err := prom.CombinedOutput()

	// Then
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.Equal(t, 2, exitError.ExitCode(), string(output))
	require.Contains(t, string(output), filename)
}
