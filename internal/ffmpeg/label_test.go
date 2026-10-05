// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

func TestSourceQuality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		info    probe.Info
		want    string
		wantHDR bool
	}{
		{
			name:    "a 4k pq source reads as 4k hdr10",
			info:    probe.Info{Height: 2160, ColorTransfer: clip.TransferPQ},
			want:    "4K HDR10",
			wantHDR: true,
		},
		{
			name:    "a 1080p hlg source reads as 1080p hlg",
			info:    probe.Info{Height: 1080, ColorTransfer: clip.TransferHLG},
			want:    "1080p HLG",
			wantHDR: true,
		},
		{
			name:    "an sdr source carries no transfer name",
			info:    probe.Info{Height: 1080, ColorTransfer: nameBT709},
			want:    "1080p",
			wantHDR: false,
		},
		{
			name:    "a short pq alias is still hdr",
			info:    probe.Info{Height: 720, ColorTransfer: clip.TransferPQAlias},
			want:    "720p HDR10",
			wantHDR: true,
		},
		{
			name:    "an unknown height drops the resolution class",
			info:    probe.Info{ColorTransfer: clip.TransferPQ},
			want:    "HDR10",
			wantHDR: true,
		},
		{
			name:    "an unprobed source has no label",
			info:    probe.Info{},
			want:    "",
			wantHDR: false,
		},
		{
			name:    "an odd height keeps its own number",
			info:    probe.Info{Height: 576},
			want:    "576p",
			wantHDR: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, SourceQuality(test.info))
			assert.Equal(t, test.wantHDR, IsHDRSource(test.info.ColorTransfer))
		})
	}
}
