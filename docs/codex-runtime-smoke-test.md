# Codex Runtime Smoke Test

Use this runbook to launch Sodoryard and verify the Codex auth/reasoning path against a real account.

Run commands from the Sodoryard repo root unless a step explicitly says to switch projects.

## 0. Preconditions

- Use `rtk` for commands in this repo.
- Use the operator CLI, `yard`. Do not use `codex auth`.
- Codex auth should end up in `~/.sirtopham/auth.json`.
- `CODEX_HOME/auth.json` or `~/.codex/auth.json` may be imported once if the private store is empty, but they should not be needed after that.
- If you want code retrieval or brain retrieval in the smoke test, the local embedding service must be available.

## 1. Build The Current Harness

```bash
rtk make yard
rtk make build
```

Expected:
- `bin/yard` exists.
- `bin/tidmouth` exists.
- The build embeds `web/dist` into `webfs/dist`.

For a full local regression pass:

```bash
rtk make test
```

## 2. Confirm Runtime Config

```bash
rtk ./bin/yard config
```

Expected lines:

```text
config: valid
default_provider: codex
default_model: gpt-5.5
server_address: localhost:8090
```

If you are testing a different project, build Sodoryard first, then switch to that project and run the same command with its config:

```bash
cd /path/to/project
rtk /home/gernsback/source/sodoryard/bin/yard config --config yard.yaml
```

## 3. Login To Codex

First inspect state without mutating it:

```bash
rtk ./bin/yard auth status
```

Then start the Yard-owned Codex device-code login:

```bash
rtk ./bin/yard auth login codex
```

Expected:
- The CLI prints `https://auth.openai.com/codex/device`.
- The CLI prints a user code.
- You approve the code in the browser.
- The login completes and writes only the private store at `~/.sirtopham/auth.json`.

Recheck state:

```bash
rtk ./bin/yard auth status
rtk ./bin/yard doctor
```

Expected:
- Codex reports available credentials.
- The effective source is the private Yard compatibility store once login/import has happened.
- Any remediation text says `yard auth login codex`.

Optional file check:

```bash
rtk ls -l "$HOME/.sirtopham/auth.json"
```

## 4. Build Retrieval Indexes

For runtime validation with repository context, build the code index before `serve`.

If local services are not already running:

```bash
rtk ./bin/yard llm status
rtk ./bin/yard llm up
```

Then build indexes:

```bash
rtk ./bin/yard index --full
rtk ./bin/yard brain index
```

Expected:
- `yard index --full` completes without embedding or LanceDB errors.
- `yard brain index` completes from the configured brain backend. In Shunter mode it reads Shunter project-memory documents; `.brain/` is only needed for explicit legacy vault import/export workflows.

If indexing fails with an embedding connection error, run:

```bash
rtk ./bin/yard llm status
rtk ./bin/yard llm logs
```

Then start/fix the local stack and rerun the index command.

## 5. Launch The Web Harness

Single-binary path:

```bash
rtk ./bin/yard serve --host 127.0.0.1 --port 8090
```

Open:

```text
http://localhost:8090
```

If port `8090` is busy:

```bash
rtk ./bin/yard serve --host 127.0.0.1 --port 8091
```

Development path, two terminals:

```bash
rtk make dev-backend
```

```bash
rtk make dev-frontend
```

Open the Vite URL printed by the frontend terminal. Vite proxies `/api/*` to the Go backend.

## 6. Browser Smoke Test

Open browser devtools before sending messages.

1. Open Settings.
2. Confirm the effective default provider/model is `codex / gpt-5.5`.
3. Create a new conversation.
4. Send:

```text
Give me a one-sentence summary of this project. Do not modify files.
```

Expected:
- The websocket connects and stays connected.
- The turn completes without auth errors.
- The UI does not expose encrypted reasoning blobs.
- The assistant response is saved in the conversation.

Now send a prompt that should use repository context:

```text
Use the repository context to answer: which binary serves the web UI and API server? Keep the answer short and do not modify files.
```

Expected:
- The answer names `yard serve` as the operator command and may mention `tidmouth` as the retained internal engine binary.
- Tool calls, if any, complete cleanly.
- There are no `tool_call_id` mismatch errors in backend logs.

Finally send a Codex reasoning/replay-oriented prompt:

```text
Inspect the Codex provider path and explain in two bullets how encrypted Codex reasoning is preserved. Do not modify files.
```

Expected:
- The turn completes.
- Visible UI thinking deltas, if any, are display-only.
- No encrypted `codex_reasoning` payload is shown in the UI transcript.
- A follow-up message in the same conversation still works, proving replay did not break the next request.

## 7. TUI And Chain Smoke

This verifies the daily-driver terminal path and the one-step chain contract.

```bash
rtk ./bin/yard chain start \
  --role coder \
  --max-steps 1 \
  --max-duration 2m \
  --task "In one sentence, name the default Codex model configured by this project. Do not edit files."
```

Expected:
- The command prints a chain ID.
- The one-step chain completes or leaves a readable failure receipt.
- The answer is `gpt-5.5` or explicitly says the default Codex model is `gpt-5.5`.
- No provider auth or tool-result mismatch error appears.

Then open the terminal console:

```bash
rtk ./bin/yard
```

Expected:
- The dashboard shows the configured provider/model, auth status, code index status, brain index status, local service mode, and active-chain count.
- The Chains screen can follow, pause, resume, cancel where valid, and open receipts.
- The Metrics browser route is optional; the TUI remains the daily-driver control surface.

## 8. API Sanity Checks While Server Is Running

```bash
rtk curl -s http://localhost:8090/api/config
rtk curl -s http://localhost:8090/api/runtime/status
rtk curl -s http://localhost:8090/api/chains
```

Expected:
- `default_provider` is `codex`.
- `default_model` is `gpt-5.5`.
- `/api/runtime/status` reports provider/model/auth/index readiness.
- `/api/chains` returns JSON, even if it is an empty list.

If you launched on a different port, replace `8090`.

## 9. Logs To Watch

During the smoke test, watch the backend terminal for:

- `Codex authentication failed`
- remediation that says `codex auth`
- websocket disconnect loops
- `failed to parse stream event`
- `tool_call_id` or tool-result mismatch errors
- raw encrypted reasoning appearing in log lines that are meant for UI/display

Any of those should block calling the harness ready.

## 10. Cleanup

Stop the server with `Ctrl-C`.

If you started local LLM services only for this smoke test:

```bash
rtk ./bin/yard llm down
```

Leave `~/.sirtopham/auth.json` in place if the test passed; that is the expected runtime Codex auth store.

## Go/No-Go

Good to keep testing daily-driver use when all are true:

- `rtk make test` passes or the narrower test set you care about passes.
- `rtk make build` passes.
- `rtk ./bin/yard config` reports `codex / gpt-5.5`.
- `rtk ./bin/yard auth login codex` succeeds or `yard auth status` already reports usable private Codex auth.
- `rtk ./bin/yard index --full` succeeds if retrieval is part of the test.
- `rtk ./bin/yard serve` launches the UI.
- Browser chat completes at least one normal turn and one repository-context turn.
- `rtk ./bin/yard chain start --role coder --max-steps 1 ...` completes or produces an inspectable receipt.
- Bare `rtk ./bin/yard` opens the TUI and shows actionable readiness.
- A follow-up turn in the same conversation works.
