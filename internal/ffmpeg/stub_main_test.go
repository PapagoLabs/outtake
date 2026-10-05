// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	const (
		cropdetectLog = "[Parsed_cropdetect_0 @ 0x1] crop=3840:1608:0:216\n" +
			"Stream #0:0: Video: hevc, yuv420p, 3840x2160 [SAR 1:1 DAR 16:9]\n"
		signalstatsLog = "lavfi.signalstats.YMAX=143.0\nlavfi.signalstats.YMAX=158.0\n"
		probeJSON      = `{
  "format": {
    "duration": "12.5",
    "bit_rate": "8000",
    "format_name": "matroska"
  },
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "hevc",
      "width": 3840,
      "height": 2160,
      "color_transfer": "smpte2084"
    },
    {
      "index": 1,
      "codec_type": "audio",
      "codec_name": "eac3",
      "channels": 6,
      "tags": {
        "language": "eng",
        "title": "Surround"
      }
    }
  ]
}`
	)

	for _, script := range []string{
		plainStubScript(cropdetectLog),
		plainStubScript(signalstatsLog),
		plainStubScript("no signalstats output here"),
		failingStubScript(signalstatsLog),
		swapStubScript(cropdetectLog),
		swapStubScript(signalstatsLog),
		probeSwapStubScript(probeJSON),
		probeStubScript(probeJSON),
	} {
		writeStubDirect(script)
	}

	os.Exit(m.Run())
}

func writeStubDirect(script string) {
	dir := stubDir()
	if dir == "" {
		return
	}

	path := stubPath(dir, script)

	_, statErr := os.Stat(path)
	if os.IsNotExist(statErr) {
		_ = os.WriteFile(path, []byte(script), 0o700)
	}
}
