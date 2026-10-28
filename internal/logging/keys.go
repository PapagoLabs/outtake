// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package logging

// Field keys name the fields of a log line, so every package writes the same
// field under the same key.
const (
	// KeyArgs is the argv of an FFmpeg or FFprobe run.
	KeyArgs = "args"
	// KeyCount is how many items a list held.
	KeyCount = "count"
	// KeyDir is the directory a line is about.
	KeyDir = "dir"
	// KeyDuration is how long a request took.
	KeyDuration = "duration"
	// KeyError is a failure's error chain, written as text.
	KeyError = "error"
	// KeyExitCode is the exit code of a finished process.
	KeyExitCode = "exit_code"
	// KeyFFmpegThreads is the thread budget each FFmpeg run gets.
	KeyFFmpegThreads = "ffmpeg_threads"
	// KeyFormat is the output format a command wrote.
	KeyFormat = "format"
	// KeyHost is the Host header of a request.
	KeyHost = "host"
	// KeyInterval is how often a poll runs.
	KeyInterval = "interval"
	// KeyJobID is the id of the clip a render job makes.
	KeyJobID = "job_id"
	// KeyListenAddr is the address the server listens on.
	KeyListenAddr = "listen_addr"
	// KeyMethod is the HTTP method of a request.
	KeyMethod = "method"
	// KeyOutput is the output a process wrote.
	KeyOutput = "output"
	// KeyOutputLen is the length in bytes of the output a process wrote.
	KeyOutputLen = "output_len"
	// KeyPanic is the value a recovered panic carried.
	KeyPanic = "panic"
	// KeyPath is the file path or request path a line is about.
	KeyPath = "path"
	// KeyPlexUserID is the plex.tv account id of a user.
	KeyPlexUserID = "plex_user_id"
	// KeyPreviewID is the id of a preview render.
	KeyPreviewID = "preview_id"
	// KeyRef is the short reference a failure is shown and logged with.
	KeyRef = "ref"
	// KeyRemoved is how many items a cleanup removed.
	KeyRemoved = "removed"
	// KeyReplacedBy is the setting that replaces a retired one.
	KeyReplacedBy = "replaced_by"
	// KeyResponse is the body of an HTTP response.
	KeyResponse = "response"
	// KeyServer is the name of a Plex Media Server.
	KeyServer = "server"
	// KeySetting is the name of a setting.
	KeySetting = "setting"
	// KeyStack is the goroutine stack of a recovered panic.
	KeyStack = "stack"
	// KeyStatus is the HTTP status a request was answered with.
	KeyStatus = "status"
	// KeyStatusCode is the HTTP status of a response a request received.
	KeyStatusCode = "status_code"
	// KeyType is the output type of a clip.
	KeyType = "type"
	// KeyURL is the URL of an outgoing request.
	KeyURL = "url"
	// KeyUsername is the plex.tv username of a user.
	KeyUsername = "username"
	// KeyWorkers is how many render workers run.
	KeyWorkers = "workers"
)
