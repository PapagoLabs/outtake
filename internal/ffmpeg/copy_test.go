// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

// copySource is a realistic source a copy is tested against.
type copySource struct {
	// name names the source and its file.
	name string
	// args encode the source after the input arguments.
	args []string
	// hevc reports that the source is HDR HEVC.
	hevc bool
	// remux names the MP4 source this one is remuxed from, which keeps its
	// color tags where encoding straight to MKV loses them.
	remux string
	// unclean reports that the copy starts on an open GOP keyframe whose
	// leading frames do not decode, so it is refused.
	unclean bool
}

// copyPreset is a preset that copies, with HDR kept.
var copyPreset = clip.QualityPreset{
	CRF:         18,
	Preset:      "slow",
	AudioKbps:   256,
	MaxWidth:    clip.OutputWidth2160p,
	PreserveHDR: true,
	CopyVideo:   true,
}

// sdrSource is a probed 8-bit H.264 source that a copy fits.
func sdrSource() probe.Info {
	return probe.Info{
		Width:       1920,
		VideoCodec:  "h264",
		PixelFormat: "yuv420p",
		AudioTracks: []probe.Track{{Index: 0, Codec: "aac", Channels: 2}},
	}
}

// TestCopyFits covers when a clip may be copied rather than encoded.
//
// A copy needs a source that is already what the encode would produce.
func TestCopyFits(t *testing.T) {
	t.Parallel()

	hdr := sdrSource()

	hdr.VideoCodec = "hevc"
	hdr.PixelFormat = "yuv420p10le"
	hdr.ColorTransfer = clip.TransferPQ

	surround := sdrSource()

	surround.AudioTracks = []probe.Track{{Index: 0, Codec: "aac", Channels: 6}}

	tests := []struct {
		name   string
		mutate func(info *probe.Info, preset *clip.QualityPreset, rect *crop.CropRect)
		source probe.Info
		want   CopyPlan
		fits   bool
	}{
		{
			name:   "an SDR H.264 source",
			source: sdrSource(),
			want:   CopyPlan{HEVC: false, CopyAudio: true},
			fits:   true,
		},
		{
			name:   "an HDR HEVC source kept HDR",
			source: hdr,
			want:   CopyPlan{HEVC: true, CopyAudio: true},
			fits:   true,
		},
		{
			name:   "surround audio is encoded",
			source: surround,
			want:   CopyPlan{HEVC: false, CopyAudio: false},
			fits:   true,
		},
		{
			name:   "a profile that does not copy",
			source: sdrSource(),
			mutate: func(_ *probe.Info, preset *clip.QualityPreset, _ *crop.CropRect) {
				preset.CopyVideo = false
			},
		},
		{
			name:   "a crop",
			source: sdrSource(),
			mutate: func(_ *probe.Info, _ *clip.QualityPreset, rect *crop.CropRect) {
				*rect = crop.CropRect{Width: 1920, Height: 800, X: 0, Y: 140}
			},
		},
		{
			name:   "wider than the profile",
			source: sdrSource(),
			mutate: func(_ *probe.Info, preset *clip.QualityPreset, _ *crop.CropRect) {
				preset.MaxWidth = clip.OutputWidth720p
			},
		},
		{
			name:   "10-bit H.264",
			source: sdrSource(),
			mutate: func(info *probe.Info, _ *clip.QualityPreset, _ *crop.CropRect) {
				info.PixelFormat = "yuv420p10le"
			},
		},
		{
			name:   "SDR HEVC",
			source: sdrSource(),
			mutate: func(info *probe.Info, _ *clip.QualityPreset, _ *crop.CropRect) {
				info.VideoCodec = "hevc"
			},
		},
		{
			name:   "HDR the profile tone maps",
			source: hdr,
			mutate: func(_ *probe.Info, preset *clip.QualityPreset, _ *crop.CropRect) {
				preset.PreserveHDR = false
			},
		},
		{
			name:   "Dolby Vision",
			source: hdr,
			mutate: func(info *probe.Info, _ *clip.QualityPreset, _ *crop.CropRect) {
				info.DolbyVision = probe.DolbyVision{
					Present:                true,
					Profile:                8,
					BaseLayerCompatibility: 1,
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			info := test.source

			info.AudioTracks = slices.Clone(info.AudioTracks)

			preset := copyPreset
			rect := crop.CropRect{}

			if test.mutate != nil {
				test.mutate(&info, &preset, &rect)
			}

			plan, fits := CopyFits(info, preset, rect, 0)
			assert.Equal(t, test.fits, fits)
			assert.Equal(t, test.want, plan)
		})
	}
}

// TestCopyArgsTagHEVCAndChooseTheAudio covers the copy's arguments.
//
// Video is always copied. HEVC is tagged hvc1, and audio is copied or encoded
// the way an encoded clip's audio is.
func TestCopyArgsTagHEVCAndChooseTheAudio(t *testing.T) {
	t.Parallel()

	encoded := strings.Join(
		copyArgs("ffmpeg", "/in.mkv", "/out.mp4", 6*time.Second, 3*time.Second, copyPreset, 1,
			CopyPlan{HEVC: true, CopyAudio: false}),
		" ",
	)

	assert.Contains(t, encoded, "-ss 6.000 -i /in.mkv -t 3.000")
	assert.Contains(t, encoded, "-map 0:a:1?")
	assert.Contains(t, encoded, "-c:v copy -tag:v hvc1")
	assert.Contains(t, encoded, "-c:a aac -b:a 256k -ac 2")
	assert.Contains(t, encoded, "-map_metadata -1 -map_chapters -1")
	assert.Contains(t, encoded, "-avoid_negative_ts make_zero")
	assert.Contains(t, encoded, "-movflags +faststart+write_colr /out.mp4")

	copied := strings.Join(copyArgs("ffmpeg", "/in.mp4", "/out.mp4", 0, time.Second, copyPreset, 0,
		CopyPlan{HEVC: false, CopyAudio: true}), " ")

	assert.Contains(t, copied, "-c:v copy -c:a copy")
	assert.NotContains(t, copied, "hvc1")
}

// TestParseLanding covers reading where a seek landed from framecrc output.
func TestParseLanding(t *testing.T) {
	t.Parallel()

	landing, err := parseLanding("#software: Lavf\n#tb 0: 1/12288\n#media_type 0: video\n" +
		"0,      72704,      73728,      512,    32731, 0xf6a8ee0a\n")
	require.NoError(t, err)
	assert.Equal(t, 6*time.Second, landing)

	landing, err = parseLanding(
		"#tb 0: 1/1000\n0,       5917,       6001,       41,    28052, 0xdd77a69d\n",
	)
	require.NoError(t, err)
	assert.Equal(t, 6001*time.Millisecond, landing)

	for _, broken := range []string{"", "#tb 0: 1/1000\n", "0, 1, 2, 3\n", "#tb 0: 1/1000\n0, 1\n"} {
		_, err = parseLanding(broken)
		require.ErrorIs(t, err, errNoLanding, broken)
	}
}

// TestCopyClipStartsOnTheKeyframe covers copies of realistic sources.
//
// The sources carry B-frames, and the MKV ones open GOPs, whose seeks can land
// on an earlier keyframe. Every copy must start on the keyframe it was
// planned from, run the planned length, and decode without errors. Finding the
// keyframe again from the start the clip then records must give the same
// keyframe, so a regenerate never moves the start further back.
func TestCopyClipStartsOnTheKeyframe(t *testing.T) {
	t.Parallel()

	ffmpegPath, ffprobePath := hdrFFmpeg(t)

	hdrTags := []string{
		"-c:v",
		"libx265",
		"-preset",
		"fast",
		"-pix_fmt",
		"yuv420p10le",
		"-color_primaries",
		"bt2020",
		"-color_trc",
		"smpte2084",
		"-colorspace",
		"bt2020nc",
		"-x265-params",
		"log-level=error:keyint=72:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
	}

	sources := []copySource{
		{
			name: "x264.mp4",
			args: []string{
				"-c:v",
				"libx264",
				"-preset",
				"medium",
				"-g",
				"72",
				"-pix_fmt",
				"yuv420p",
			},
		},
		{name: "x264open.mkv", args: []string{
			"-c:v",
			"libx264",
			"-preset",
			"medium",
			"-g",
			"72",
			"-x264-params",
			"open-gop=1",
			"-pix_fmt",
			"yuv420p",
		}, unclean: true},
		{name: "x265.mp4", args: append(slices.Clone(hdrTags), "-tag:v", "hvc1"), hevc: true},
		{
			name:  "x265.mkv",
			args:  append(slices.Clone(hdrTags), "-tag:v", "hvc1"),
			hevc:  true,
			remux: "x265-source.mp4",
		},
	}

	for _, source := range sources {
		t.Run(source.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			input := filepath.Join(dir, source.name)

			encoded := input
			if source.remux != "" {
				encoded = filepath.Join(dir, source.remux)
			}

			args := append([]string{
				"-hide_banner", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24",
				"-f", "lavfi", "-i", "sine=frequency=440",
				"-t", "12",
			}, source.args...)

			args = append(args, "-c:a", "aac", "-ac", "2", encoded)

			out, err := exec.CommandContext(t.Context(), ffmpegPath, args...).CombinedOutput()
			require.NoError(t, err, string(out))

			if source.remux != "" {
				out, err = exec.CommandContext(
					t.Context(),
					ffmpegPath,
					"-hide_banner",
					"-loglevel",
					"error",
					"-y",
					"-i",
					encoded,
					"-c",
					"copy",
					input,
				).CombinedOutput()
				require.NoError(t, err, string(out))
			}

			runner := NewExecFFmpeg(ffmpegPath, ffprobePath)

			keyframe, found, err := runner.KeyframeBefore(
				t.Context(),
				input,
				7300*time.Millisecond,
				5*time.Second,
			)
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, 6*time.Second, keyframe, "the keyframes are three seconds apart")

			again, found, err := runner.KeyframeBefore(t.Context(), input, keyframe, 5*time.Second)
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, keyframe, again, "the recorded start finds the same keyframe")

			info, err := runner.Probe(t.Context(), input)
			require.NoError(t, err)

			plan, fits := CopyFits(info, copyPreset, crop.CropRect{}, 0)
			require.True(t, fits, "the source fits the copy preset")
			assert.Equal(t, source.hevc, plan.HEVC)

			output := filepath.Join(dir, "clip.mp4")

			err = runner.CopyClip(
				t.Context(),
				input,
				output,
				keyframe,
				9300*time.Millisecond,
				copyPreset,
				0,
				plan,
			)
			if source.unclean {
				require.ErrorIs(
					t,
					err,
					ErrCopyUnclean,
					"an open GOP start is refused, so the clip is encoded",
				)
				assert.NoFileExists(t, output, "a refused copy is never published")

				return
			}

			require.NoError(t, err)

			copied, err := probe.Probe(t.Context(), ffprobePath, output)
			require.NoError(t, err)
			assert.InDelta(t, 3.3, copied.Duration.Seconds(), copyLengthTolerance.Seconds())

			if source.hevc {
				assert.Equal(
					t,
					clip.TransferPQ,
					copied.ColorTransfer,
					"the MP4 carries the source's transfer",
				)
			}

			decodeErrors, err := exec.CommandContext(t.Context(), ffmpegPath,
				"-v", "error", "-i", output, "-f", "null", "-").CombinedOutput()
			require.NoError(t, err)
			assert.Empty(t, strings.TrimSpace(string(decodeErrors)), "the copy decodes cleanly")
		})
	}
}

// fakeCopy installs fakes that answer each call a copy makes.
//
// The landing read reports landingMS, the length probe reports length, and
// the decode check reports decodeErrors. The packet list has keyframes at 3
// and 6 seconds.
//
// Parameters:
//   - t: The test the fakes belong to.
//   - landingMS: Where every seek lands, in milliseconds.
//   - length: The copy's probed length, in seconds.
//   - decodeErrors: What the decode check prints, empty for a clean copy.
//
// Returns:
//   - runner: A runner on the fakes.
//   - argv: The file every ffmpeg run's arguments are logged to.
func fakeCopy(t *testing.T, landingMS, length, decodeErrors string) (*ExecFFmpeg, string) {
	t.Helper()

	argv := filepath.Join(t.TempDir(), "argv.log")

	ffmpegPath := ffmpegtest.Install(t, ffmpegtest.Stub{
		Output:        "copied",
		ArgvFile:      argv,
		ArgvSeparator: "----",
		Cases: []ffmpegtest.Case{
			{
				Arg:    "framecrc",
				Stdout: "#tb 0: 1/1000\n0,       5917,       " + landingMS + ",       41,    100, 0x0\n",
			},
			{Arg: "null", Stderr: decodeErrors},
		},
	})

	ffprobePath := ffmpegtest.Install(t, ffmpegtest.Stub{
		Stdout: `{"format": {"duration": "` + length + `"}, "streams": [` +
			`{"codec_type": "video", "codec_name": "h264", "width": 640, "height": 360, "pix_fmt": "yuv420p"}]}`,
		Cases: []ffmpegtest.Case{
			{Arg: "csv=p=0", Stdout: "3.000000,K__\n4.000000,___\n6.000000,K__\n"},
		},
	})

	return NewExecFFmpeg(ffmpegPath, ffprobePath), argv
}

// TestCopyClipOnFakes covers each way a copy ends, without a real FFmpeg.
//
// A copy that lands on its keyframe, runs the planned length, and decodes
// cleanly is published. Every other outcome is refused with the error the
// render job falls back to the encode on, and leaves nothing behind.
func TestCopyClipOnFakes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		landingMS    string
		length       string
		decodeErrors string
		want         error
	}{
		{name: "a clean copy", landingMS: "6000", length: "3.3"},
		{
			name:      "a seek that lands on the keyframe before",
			landingMS: "3000",
			length:    "3.3",
			want:      ErrCopyUnavailable,
		},
		{
			name:      "a seek that lands past the keyframe",
			landingMS: "9000",
			length:    "3.3",
			want:      ErrCopyUnavailable,
		},
		{
			name:      "a copy of the wrong length",
			landingMS: "6000",
			length:    "6.3",
			want:      ErrCopyMismatch,
		},
		{
			name:         "a copy that does not decode",
			landingMS:    "6000",
			length:       "3.3",
			decodeErrors: "mmco: unref short failure",
			want:         ErrCopyUnclean,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner, argv := fakeCopy(t, test.landingMS, test.length, test.decodeErrors)
			output := filepath.Join(t.TempDir(), "clip.mp4")

			err := runner.CopyClip(
				t.Context(),
				"/media/source.mkv",
				output,
				6*time.Second,
				9300*time.Millisecond,
				copyPreset,
				0,
				CopyPlan{HEVC: false, CopyAudio: true},
			)

			if test.want != nil {
				require.ErrorIs(t, err, test.want)
				assert.NoFileExists(t, output, "a refused copy is never published")

				return
			}

			require.NoError(t, err)
			assert.FileExists(t, output)

			logged, err := os.ReadFile(argv)
			require.NoError(t, err)
			assert.Contains(t, string(logged), "-ss\n6.000\n-i\n/media/source.mkv\n-t\n3.300\n",
				"the copy seeks to the keyframe and runs to the end mark")
		})
	}
}

// TestCopyClipReportsAFailedLandingRead covers a seek read that fails, which
// is not a reason to encode but a failure of its own.
func TestCopyClipReportsAFailedLandingRead(t *testing.T) {
	t.Parallel()

	runner := NewExecFFmpeg(
		ffmpegtest.Install(t, ffmpegtest.Stub{ExitCode: 1}),
		ffmpegtest.Install(t, ffmpegtest.Stub{}),
	)

	err := runner.CopyClip(t.Context(), "/media/source.mkv", filepath.Join(t.TempDir(), "clip.mp4"),
		6*time.Second, 9*time.Second, copyPreset, 0, CopyPlan{HEVC: false, CopyAudio: true})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrCopyUnavailable)
}

// TestKeyframeBeforeOnAFakeProbe covers the keyframe search through the
// runner, and a probe that fails.
func TestKeyframeBeforeOnAFakeProbe(t *testing.T) {
	t.Parallel()

	runner, _ := fakeCopy(t, "6000", "3.3", "")

	keyframe, found, err := runner.KeyframeBefore(
		t.Context(),
		"/media/source.mkv",
		7300*time.Millisecond,
		5*time.Second,
	)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 6*time.Second, keyframe)

	broken := NewExecFFmpeg("ffmpeg", ffmpegtest.Install(t, ffmpegtest.Stub{ExitCode: 1}))

	_, _, err = broken.KeyframeBefore(t.Context(), "/media/source.mkv", time.Second, time.Second)
	require.Error(t, err)
}
