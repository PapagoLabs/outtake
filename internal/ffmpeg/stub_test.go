// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var stubDir = sync.OnceValue(func() string {
	dir, err := os.MkdirTemp("", "outtake-stub")
	if err != nil {
		return ""
	}

	return dir
})

func stubScript(t *testing.T, script string) string {
	t.Helper()

	_, statErr := os.Stat("/bin/sh")
	if statErr != nil {
		t.Skip("a POSIX shell is required for the ffmpeg stub")
	}

	dir := stubDir()
	if dir == "" {
		t.Skip("unable to create a directory for the ffmpeg stub")
	}

	path := stubPath(dir, script)

	_, pathErr := os.Stat(path)
	if os.IsNotExist(pathErr) {
		require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	}

	return path
}

func stubPath(dir, script string) string {
	sum := sha256.Sum256([]byte(script))

	return filepath.Join(dir, "stub-"+hex.EncodeToString(sum[:8]))
}

func swapInputLookup(script string) string {
	return script + "target=\n" +
		"want=0\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$want\" = 1 ]; then target=$arg; break; fi\n" +
		"  if [ \"$arg\" = \"-i\" ]; then want=1; fi\n" +
		"done\n"
}

func swapStubScript(payload string) string {
	return swapInputLookup("#!/bin/sh\n") +
		"mv \"${target%/*}/replacement.mkv\" \"$target\"\n" +
		stderrHeredoc(payload)
}

func plainStubScript(payload string) string {
	return "#!/bin/sh\n" + stderrHeredoc(payload)
}

func failingStubScript(payload string) string {
	return plainStubScript(payload) + "exit 1\n"
}

func probeStubScript(payload string) string {
	return "#!/bin/sh\n" + stdoutHeredoc(payload)
}

func probeSwapStubScript(payload string) string {
	return "#!/bin/sh\n" +
		"for target; do :; done\n" +
		"mv \"${target%/*}/replacement.mkv\" \"$target\"\n" +
		stdoutHeredoc(payload)
}

func stdoutHeredoc(payload string) string {
	return "cat <<'STUB_OUT'\n" + terminated(payload) + "STUB_OUT\n"
}

func stderrHeredoc(payload string) string {
	return "cat >&2 <<'STUB_ERR'\n" + terminated(payload) + "STUB_ERR\n"
}

func terminated(payload string) string {
	if strings.HasSuffix(payload, "\n") {
		return payload
	}

	return payload + "\n"
}
