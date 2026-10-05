// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	const probeJSON = `{
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

	for _, script := range []string{
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
