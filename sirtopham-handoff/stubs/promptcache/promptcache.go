package promptcache

// BlockKind distinguishes stable and dynamic prompt regions.
type BlockKind string

const (
	BlockKindStableSession BlockKind = "stable_session"
	BlockKindStableTurn    BlockKind = "stable_turn"
	BlockKindDynamic       BlockKind = "dynamic"
)

// PromptBlock is a logical component of the final rendered prompt.
type PromptBlock struct {
	Name    string
	Kind    BlockKind
	Content string
}

// RenderedPrompt exposes byte-stable sections explicitly.
type RenderedPrompt struct {
	StableSession string
	StableTurn    string
	DynamicTail   string
	Full          string
}

// LatchState captures request-shape properties that should not drift accidentally.
type LatchState struct {
	ModelID           string
	ToolSchemaDigest  string
	StablePromptHash  string
	OutputStyle       string
	ReasoningMode     string
}

// Renderer builds the block-wise prompt representation.
type Renderer interface {
	Render(blocks []PromptBlock) (RenderedPrompt, error)
}

// Latcher stores and validates cache-relevant state over session/turn lifetime.
type Latcher interface {
	LatchSession(state LatchState) error
	Current() (LatchState, bool)
	Validate(next LatchState) error
}

// Suggested rule:
// any field that meaningfully perturbs prompt-cache identity should become visible in LatchState,
// not left as an implicit side effect of request assembly.
