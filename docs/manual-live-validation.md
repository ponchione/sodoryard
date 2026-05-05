# Manual live validation

Use this checklist for a real human browser/runtime validation before treating the repo as daily-driver ready.

## 0. Preconditions

Choose the exact config you intend to use daily.

Confirm before starting:
- provider credentials for the model path you actually want to use are already working
- local embedding service is available if you want indexing/retrieval
- the configured brain backend is ready for the project

## 1. Clean build and startup

From repo root:

1. `make test`
2. `make build`
3. `./bin/yard index --config <your-config>.yaml`
4. `./bin/yard serve --config <your-config>.yaml`
5. Open the served app URL in the browser

Expected:
- app loads without a blank page
- no immediate backend crash
- browser console has no startup errors

## 2. First conversation flow

1. Create a new conversation
2. Send a simple first message like: `Give me a one-sentence summary of this project.`
3. Wait for the turn to finish

Expected:
- first turn succeeds
- new conversation appears in the sidebar without manual reload
- transcript renders cleanly
- no websocket disconnect loop
- no JS console errors

## 3. Reload and historical navigation

1. Reload the conversation page directly
2. Confirm the same conversation history is still visible
3. Navigate away to another page and back
4. If inspector is open, verify historical turn navigation still works

Expected:
- messages persist across reload
- conversation route loads directly
- latest turn usage still appears under the latest assistant message
- historical inspector data loads without requiring a new live turn

## 4. Settings and model routing

1. Open Settings
2. Verify the current default provider/model shown there matches your intended runtime defaults
3. Change the default model/provider to another valid option if you have one
4. Save
5. Start a fresh conversation and send a message

Expected:
- save succeeds cleanly
- selector does not blank out or show impossible options
- the next new turn uses the updated default
- no console errors on Settings or conversation pages

## 5. Code retrieval sanity check

In a new or existing conversation, ask a code-structure question that should require indexed code context, for example:
- `Where is the websocket turn handling path implemented?`
- `Which files define context assembly and metrics reporting?`

Then open the Context Inspector.

Expected:
- assistant answer is grounded in the repo, not generic
- inspector shows non-empty retrieval/context evidence
- included file/chunk paths match the answer
- backend logs do not show repeated retrieval/path warnings

## 6. Brain retrieval sanity check (only if you use brain features daily)

Ask a brain-oriented question that should be answerable from your note/vault content.
Then inspect the same turn.

Expected:
- assistant answers from brain knowledge without obviously guessing
- context report shows brain hits
- `budget_breakdown.brain` is non-zero when proactive brain retrieval was used
- signal flow shows brain-oriented retrieval/query evidence when applicable

Optional strict scripted proof:
- `python3 scripts/validate_brain_retrieval.py --base-url http://localhost:<port> --scenario runtime-proof`
- `python3 scripts/validate_brain_retrieval.py --base-url http://localhost:<port> --scenario rationale-layout`
- `python3 scripts/validate_brain_retrieval.py --base-url http://localhost:<port> --scenario debug-history-vite`

## 7. Search and transcript quality

1. Create at least one conversation containing a unique search token you can recognize
2. Use the app's conversation search or the API-backed search UI behavior
3. Open the matched conversation

Expected:
- search returns one useful result per conversation, not noisy duplicates
- snippets are readable and not raw JSON/tool noise
- titles look sane and not like tombstones or raw payload text

## 8. Cancellation and recovery

1. Start a longer answer or tool-using turn
2. Cancel it mid-flight
3. Send another message afterward

Expected:
- cancel does not wedge the UI
- later turns still work in the same conversation
- transcript remains readable
- no repeated backend errors after cancellation

## 9. Browser-console and backend-log sweep

During the tests above, keep devtools and backend logs open.

Watch for:
- browser JS exceptions
- websocket 502/connection failures
- provider auth failures
- repeated retrieval warnings
- DB migration/startup errors
- unexpected panic/stack traces

## 10. Daily-driver go/no-go

Call it ready only if all of the following are true:
- `make test` passes
- `make build` passes
- index + serve succeed on the exact config you plan to use
- first-turn chat, reload, sidebar, settings, and search all behave correctly
- retrieval evidence looks real in the inspector
- brain flow works if you rely on it
- browser console stays clean
- backend logs show no recurring runtime errors

## Notes

Current known non-blocker:
- frontend build may still warn about large chunks depending on current bundle composition; warning alone is not a release blocker if runtime behavior is good

Current practical rule:
- validate with the exact config, provider, target project, and ports you plan to use daily; passing tests on a different setup is not enough
