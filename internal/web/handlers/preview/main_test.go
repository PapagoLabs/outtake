// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"os"
	"testing"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
)

func TestMain(m *testing.M) {
	ffmpegtest.Dispatch()

	os.Exit(m.Run())
}
