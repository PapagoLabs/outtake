// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/clip"
)

func TestSummarize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		give  []clip.Status
		want  Summary
		total int
	}{
		{
			name: "no clips at all",
		},
		{
			name: "a queued clip is pending",
			give: []clip.Status{clip.StatusPending},
			want: Summary{Total: 1, Pending: 1},
		},
		{
			name: "a rendering clip is pending",
			give: []clip.Status{clip.StatusProcessing},
			want: Summary{Total: 1, Pending: 1},
		},
		{
			name: "a finished clip is completed",
			give: []clip.Status{clip.StatusCompleted},
			want: Summary{Total: 1, Completed: 1},
		},
		{
			name: "a clip that failed is failed",
			give: []clip.Status{clip.StatusFailed},
			want: Summary{Total: 1, Failed: 1},
		},
		{
			name: "a stopped clip is counted but bucketed nowhere",
			give: []clip.Status{clip.StatusCancelled},
			want: Summary{Total: 1},
		},
		{
			name: "every status at once",
			give: []clip.Status{
				clip.StatusPending,
				clip.StatusProcessing,
				clip.StatusCompleted,
				clip.StatusCompleted,
				clip.StatusFailed,
				clip.StatusCancelled,
			},
			want: Summary{Total: 6, Pending: 2, Completed: 2, Failed: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			clips := make([]*clip.Job, 0, len(test.give))
			for i, status := range test.give {
				clips = append(clips, &clip.Job{ID: string(rune('a' + i)), Status: status})
			}

			assert.Equal(t, test.want, Summarize(clips))
		})
	}
}
