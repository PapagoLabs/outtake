// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

// Scope is whose clips and previews a caller may see and change.
type Scope struct {
	// Owner is the caller's plex.tv account id, or zero for no caller.
	Owner int
	// Everyone is true for a caller who may see every account's clips, as the
	// owner and admins may.
	Everyone bool
}

// Filter keeps the jobs a scope reaches.
//
// Parameters:
//   - jobs: The jobs to filter.
//
// Returns:
//   - reached: The jobs the scope includes, in their order.
func (scope Scope) Filter(jobs []*Job) []*Job {
	if scope.Everyone {
		return jobs
	}

	reached := make([]*Job, 0, len(jobs))

	for _, job := range jobs {
		if scope.Includes(job.OwnerID) {
			reached = append(reached, job)
		}
	}

	return reached
}

// Includes reports whether the scope reaches a record an account owns.
//
// A scope with no caller reaches nothing but what Everyone grants, so a
// record nobody owns is reached only by the owner and admins.
//
// Parameters:
//   - owner: The plex.tv account id that owns the record, zero for none.
//
// Returns:
//   - included: True when the caller may see and change the record.
func (scope Scope) Includes(owner int) bool {
	return scope.Everyone || (scope.Owner != 0 && owner == scope.Owner)
}
