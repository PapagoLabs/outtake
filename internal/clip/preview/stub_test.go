// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

// previewStubProbeJSON is the ffprobe answer for a plain high-definition source.
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

func TestMain(m *testing.M) {
	ffmpegtest.Dispatch()

	os.Exit(m.Run())
}

// stagingFFmpeg returns a runner whose ffmpeg records its argv beside the file
// it writes and whose ffprobe reports a plain high-definition source.
//
// Parameters:
//   - t: The test the runner belongs to.
//   - encodeErr: Whether the encode writes a partial file and fails.
//
// Returns:
//   - runner: An ffmpeg runner backed by fakes.
func stagingFFmpeg(t *testing.T, encodeErr bool) *ffmpeg.ExecFFmpeg {
	t.Helper()

	encode := ffmpegtest.Stub{ArgvBesideOutput: true, Output: "encoded"}
	if encodeErr {
		encode = ffmpegtest.Stub{ArgvBesideOutput: true, Output: "partial", ExitCode: 1}
	}

	return ffmpeg.NewExecFFmpeg(
		ffmpegtest.Install(t, encode),
		ffmpegtest.Install(t, ffmpegtest.Stub{Stdout: previewStubProbeJSON}),
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
