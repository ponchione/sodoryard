// Package context owns Layer 3 context-assembly contracts and implementations.
//
// It defines the per-turn context assembly surface used by the future agent
// loop: turn analysis, retrieval orchestration, budget fitting, serialization,
// report generation, and compression support. Layer 5 consumes the assembled
// context output but is intentionally not imported here; Layer 3 exposes narrow
// boundary types instead.
//
// Project-brain fields remain in the Layer 3 contracts for report/schema
// continuity, and proactive Shunter-backed brain retrieval is live.
package context
