// Package manifest defines the minimal task-envelope data model: content
// refs and append-only slot revisions, per
// docs/architecure/task-envelope-design.md section 5 (trimmed for this
// slice — no facts, no derived views, no schema annotations).
package manifest

// Ref is a content descriptor for a blob stored in blobstore: enough to
// locate and verify it, never the content itself. Bucket makes the ref
// self-locating (design doc section 5.1's "urls" field, trimmed to a
// single bucket name since this slice has one store) rather than relying
// on callers to know the blobstore's bucket out of band.
type Ref struct {
	MediaType string
	Digest    string // "sha256:<hex>"
	Size      int64
	Bucket    string // Minio bucket holding "{tenant}/sha256/{hash}"
}

// Revision is one append-only entry in a Slot: a produced Ref plus the
// minimal lineage needed to know why it exists.
type Revision struct {
	Ref        Ref
	ProducedBy string // e.g. "ops-buddy#1"
	Reason     string // short free text, e.g. "initial"
}

// Slot holds every revision ever produced for one named role in a task
// (e.g. "plan", "review"), plus a pointer to the current one. Revisions
// are never removed or replaced.
type Slot struct {
	Current   Ref
	Revisions []Revision
}

// AppendRevision appends rev to slot's revision list and moves Current to
// it, returning the updated Slot. slot may be the zero value (first
// revision for a not-yet-existing slot).
func AppendRevision(slot Slot, rev Revision) Slot {
	slot.Revisions = append(slot.Revisions, rev)
	slot.Current = rev.Ref
	return slot
}
