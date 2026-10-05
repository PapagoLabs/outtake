// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

var previewStubDir = sync.OnceValue(func() string {
	dir, err := os.MkdirTemp("", "outtake-preview-stub")
	if err != nil {
		return ""
	}

	return dir
})

func TestMain(m *testing.M) {
	const previewStubProbeJSON = `{
  "format": {
    "duration": "120.0",
    "bit_rate": "8000",
    "format_name": "matroska"
  },
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "h264",
      "width": 1920,
      "height": 1080,
      "color_transfer": "bt709"
    },
    {
      "index": 1,
      "codec_type": "audio",
      "codec_name": "aac",
      "channels": 2
    }
  ]
}
`

	dir := previewStubDir()
	if dir != "" {
		for _, script := range []string{
			previewEncodeStubScript(false),
			previewEncodeStubScript(true),
			previewProbeStubScript(previewStubProbeJSON),
		} {
			writePreviewStubDirect(dir, script)
		}
	}

	os.Exit(m.Run())
}

func previewEncodeStubScript(failing bool) string {
	body := "printf 'encoded' > \"$last\"\n"

	if failing {
		body = "printf 'partial' > \"$last\"\nexit 1\n"
	}

	return "#!/bin/sh\n" +
		"for arg in \"$@\"; do last=$arg; done\n" +
		"for arg in \"$@\"; do printf '%s\\n' \"$arg\" >> \"${last}.argv\"; done\n" +
		body
}

func previewProbeStubScript(payload string) string {
	return "#!/bin/sh\ncat <<'STUB_OUT'\n" + payload + "STUB_OUT\n"
}

func previewStubPath(dir, script string) string {
	sum := sha256.Sum256([]byte(script))

	return filepath.Join(dir, "stub-"+hex.EncodeToString(sum[:8]))
}

func writePreviewStubDirect(dir, script string) {
	path := previewStubPath(dir, script)

	_, statErr := os.Stat(path)
	if os.IsNotExist(statErr) {
		_ = os.WriteFile(path, []byte(script), 0o700)
	}
}

func stagingFFmpeg(t *testing.T, encodeErr bool) *ffmpeg.ExecFFmpeg {
	t.Helper()

	const previewStubProbeJSON = `{
  "format": {
    "duration": "120.0",
    "bit_rate": "8000",
    "format_name": "matroska"
  },
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "h264",
      "width": 1920,
      "height": 1080,
      "color_transfer": "bt709"
    },
    {
      "index": 1,
      "codec_type": "audio",
      "codec_name": "aac",
      "channels": 2
    }
  ]
}
`

	_, statErr := os.Stat("/bin/sh")
	if statErr != nil {
		t.Skip("a POSIX shell is required for the ffmpeg stub")
	}

	dir := previewStubDir()
	if dir == "" {
		t.Skip("unable to create a directory for the ffmpeg stub")
	}

	return ffmpeg.NewExecFFmpeg(
		previewStubPath(dir, previewEncodeStubScript(encodeErr)),
		previewStubPath(dir, previewProbeStubScript(previewStubProbeJSON)),
	)
}

func recordedTargets(t *testing.T, store *blob.Storage) []string {
	t.Helper()

	logs, err := filepath.Glob(filepath.Join(store.BasePath(), "previews", "*"+".argv"))
	if err != nil {
		return nil
	}

	targets := make([]string, 0, len(logs))

	for _, log := range logs {
		data, readErr := os.ReadFile(log)
		if readErr != nil {
			continue
		}

		argv := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")

		targets = append(targets, argv[len(argv)-1])
	}

	return targets
}

func recordedArgv(t *testing.T, target string) []string {
	t.Helper()

	data, err := os.ReadFile(target + ".argv")
	require.NoError(t, err, "the stub records the argv it was invoked with")

	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func exitError(err error) bool {
	var target *exec.ExitError

	return errors.As(err, &target)
}
