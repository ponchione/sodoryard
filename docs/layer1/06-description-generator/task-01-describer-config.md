# Task 01: Describer Config Struct

**Epic:** 06 — Description Generator
**Status:** ⬚ Not started
**Dependencies:** L0-E03 (config loading)

---

## Description

Add a `Describer` config section to the root `Config` struct in `internal/config/`. This provides all tunables the description generator needs: the LLM container URL, model name, temperature, max completion tokens, request timeout, and the file content truncation limit. Every field has a sensible default so the describer works out of the box with a standard Docker setup.

## Config Shape

The YAML section under the root config:

```yaml
describer:
  base_url: "http://localhost:8080"
  model: "qwen2.5-coder-7b"
  temperature: 0.1
  max_tokens: 4096
  timeout_seconds: 60
  max_file_content_chars: 6000
```

## Struct Definition

```go
// DescriberConfig holds settings for the local LLM description generator.
type DescriberConfig struct {
    BaseURL             string  `yaml:"base_url"`
    Model               string  `yaml:"model"`
    Temperature         float64 `yaml:"temperature"`
    MaxTokens           int     `yaml:"max_tokens"`
    TimeoutSeconds      int     `yaml:"timeout_seconds"`
    MaxFileContentChars int     `yaml:"max_file_content_chars"`
}
```

## Default Values

| Field                 | Default                    | Rationale                                                                 |
|----------------------|----------------------------|---------------------------------------------------------------------------|
| `BaseURL`            | `http://localhost:8080`    | Standard local LLM container port per spec 02                             |
| `Model`              | `qwen2.5-coder-7b`        | Default model per spec 02; configurable for DeepSeek, Mercury, etc.       |
| `Temperature`        | `0.1`                      | Low temperature for consistent, deterministic descriptions                |
| `MaxTokens`          | `4096`                     | Sufficient for JSON array of ~30 function descriptions                    |
| `TimeoutSeconds`     | `60`                       | Local LLM inference can be slow for large files                           |
| `MaxFileContentChars`| `6000`                     | Pragmatic limit for local models with smaller context windows (spec 04)   |

## Acceptance Criteria

- [ ] `DescriberConfig` struct defined in `internal/config/` with YAML tags matching the shape above
- [ ] Wired into the root `Config` struct as `Describer DescriberConfig \`yaml:"describer"\``
- [ ] Default values populated by the config loading logic (the `NewDefaultConfig` function or equivalent)
- [ ] Omitting the entire `describer:` section from YAML produces a valid config with all defaults
- [ ] Partial override works: specifying only `describer.model` keeps all other fields at defaults
- [ ] No validation beyond Go type checking required (all fields have safe defaults; an unreachable URL fails gracefully at call time, not at config load time)

## Sizing Note

Estimated ~30-40 minutes. The config struct is small, but partial-override testing and default-value wiring provide enough substance for a standalone task.
