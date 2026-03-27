package nodeagent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/momaek/tolato/internal/server/domain"
)

func handleUpgrade(ctx context.Context, downloadURL, targetVersion string, emitter ChunkEmitter) ExecutionResult {
	execPath, err := os.Executable()
	if err != nil {
		return failureResult(1, fmt.Sprintf("failed to resolve executable path: %v", err))
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return failureResult(1, fmt.Sprintf("failed to resolve symlinks: %v", err))
	}

	_ = emitter.Emit(domain.ExecutionStreamStdout, fmt.Sprintf("[upgrade] current binary: %s\n", execPath))
	_ = emitter.Emit(domain.ExecutionStreamStdout, fmt.Sprintf("[upgrade] downloading %s (target version: %s)\n", downloadURL, targetVersion))

	tmpPath := execPath + ".upgrade.tmp"
	defer os.Remove(tmpPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return failureResult(1, fmt.Sprintf("failed to create download request: %v", err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failureResult(1, fmt.Sprintf("download failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return failureResult(1, fmt.Sprintf("download returned status %d", resp.StatusCode))
	}

	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return failureResult(1, fmt.Sprintf("failed to create temp file: %v", err))
	}

	written, err := io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		return failureResult(1, fmt.Sprintf("failed to write binary: %v", err))
	}

	_ = emitter.Emit(domain.ExecutionStreamStdout, fmt.Sprintf("[upgrade] downloaded %d bytes\n", written))

	if written == 0 {
		return failureResult(1, "downloaded binary is empty")
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		return failureResult(1, fmt.Sprintf("failed to replace binary: %v", err))
	}

	_ = emitter.Emit(domain.ExecutionStreamStdout, fmt.Sprintf("[upgrade] binary replaced successfully, version=%s\n", targetVersion))
	_ = emitter.Emit(domain.ExecutionStreamStdout, "[upgrade] agent will exit for systemd restart\n")

	exitCode := 0
	return ExecutionResult{
		Status:   domain.ExecutionStatusSuccess,
		ExitCode: &exitCode,
	}
}
