# Task 05: Context Assembly Config Struct

**Epic:** 01 — Context Assembly Types & Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01; Layer 0: Epic 03 (config)

---

## Description

Define the configuration struct for the context assembly layer, loadable from the `context:` section of `sirtopham.yaml`. This struct defines the shape of all tunable parameters — budget limits, quality thresholds, structural graph settings, momentum lookback, compression settings, and debug flags. Default values are set in the config loading path, not hardcoded in the type definition.

## Acceptance Criteria

- [ ] Budget config fields: `MaxAssembledTokens int`, `MaxChunks int`, `MaxExplicitFiles int`, `ConventionBudgetTokens int`, `GitContextBudgetTokens int`
- [ ] Quality config fields: `RelevanceThreshold float64`, `StructuralHopDepth int`, `StructuralHopBudget int`, `MomentumLookbackTurns int`
- [ ] Compression config fields: `CompressionThreshold float64`, `CompressionHeadPreserve int`, `CompressionTailPreserve int`, `CompressionModel string`
- [ ] Debug config fields: `EmitContextDebug bool`, `StoreAssemblyReports bool`
- [ ] All fields have YAML struct tags matching the `context:` section of `sirtopham.yaml`
- [ ] GoDoc comment notes that defaults are set in the config loading path, not in the type definition
- [ ] Package compiles with no errors
