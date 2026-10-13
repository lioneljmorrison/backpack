package board

import "strconv"

// What a Change names.
const (
	KindStory     = "story"
	KindIdea      = "idea"
	KindPhase     = "phase"
	KindWorkspace = "workspace"
)

// Change names an item a write changed: a story (its log, rank and
// dependencies included), an idea, a phase (its goals included), or a new
// workspace, which has no ID.
type Change struct {
	Workspace string
	Kind      string
	ID        string
}

// OnChange sets the function told about each change, after the write has
// gone through. Set it before the board is shared; it is not safe to change
// while writes run.
func (b *Board) OnChange(f func(Change)) { b.onChange = f }

// changed reports a write once it has succeeded. Deferred against a named
// error result, it stays silent for a refused or failed write.
func (b *Board) changed(err error, c Change) {
	if err == nil && b.onChange != nil {
		b.onChange(c)
	}
}

func phaseID(n int) string { return strconv.Itoa(n) }
