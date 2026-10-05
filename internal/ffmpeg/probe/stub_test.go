// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

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
		t.Skip("a POSIX shell is required for the ffprobe stub")
	}

	dir := stubDir()
	if dir == "" {
		t.Skip("unable to create a directory for the ffprobe stub")
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

func terminated(payload string) string {
	if strings.HasSuffix(payload, "\n") {
		return payload
	}

	return payload + "\n"
}
