package context

// SeenFileLookup is the minimal Layer 3 boundary required for
// previously-viewed annotations during serialization.
//
// Layer 3 depends only on this lookup contract rather than importing a concrete
// Layer 5 session or conversation runtime type.
type SeenFileLookup interface {
	Contains(path string) (bool, int)
}

// AssemblyScope carries the small amount of per-turn runtime state Layer 3
// needs at assembly time.
type AssemblyScope struct {
	ConversationID string         `json:"conversation_id"`
	TurnNumber     int            `json:"turn_number"`
	SeenFiles      SeenFileLookup `json:"-"`
}
