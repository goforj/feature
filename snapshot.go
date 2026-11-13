package feature

// nativeSnapshotter preserves the native definition and clock capture contract through wrappers.
type nativeSnapshotter interface {
	Snapshot() *View
}

// Snapshot captures native definitions and evaluation time while preserving observer and tracker policy.
// Capturing an existing native View is idempotent. Evaluators without native snapshot
// support return ErrUnsupported; a vendor SDK scope is never presented as frozen state.
// The returned facade borrows the same observer and tracking sink as its parent.
func (f *Features) Snapshot() (*Features, error) {
	var captured Evaluator
	switch evaluator := f.evaluator.(type) {
	case *View:
		captured = evaluator
	case nativeSnapshotter:
		captured = evaluator.Snapshot()
	default:
		return nil, ErrUnsupported
	}
	return &Features{evaluator: captured, observer: f.observer, tracker: f.tracker}, nil
}
