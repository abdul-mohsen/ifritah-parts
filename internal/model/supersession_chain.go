package model

// SupersessionChain is the transitive replacement chain for a part,
// built by recursively walking replacedbyarticles / replacesarticles.
//
// Current is the queried article. ReplacedBy is the ordered forward chain
// (newer parts that replace Current); Replaces is the ordered backward
// chain (older parts Current supersedes). Depth is the deepest hop count
// discovered by the recursive CTE, capped at MaxSupersessionDepth to
// prevent runaway traversals.
//
// Truncated is true when the walker hit the depth cap; consumers must
// surface a warning rather than claim a complete chain per BUGS.md
// ("Only cautious legacy source-backed links").
type SupersessionChain struct {
	Current    SupersessionLink   `json:"current"`
	ReplacedBy []SupersessionLink `json:"replacedBy,omitempty"`
	Replaces   []SupersessionLink `json:"replaces,omitempty"`
	Depth      int                `json:"depth"`
	Truncated  bool               `json:"truncated,omitempty"`
}

// MaxSupersessionDepth caps the recursive CTE walk. TecDoc chains rarely
// exceed 5 hops; anything deeper is almost certainly a data cycle or a
// misuse of the supersession table.
const MaxSupersessionDepth = 10
