# Shunter Project Memory Migration

New projects created by `yard init` use Shunter project memory by default:

```yaml
memory:
  backend: shunter
brain:
  backend: shunter
```

In this mode, `.yard/shunter/project-memory` is the canonical project-memory store. `.brain/` and `.yard/yard.db` are legacy/transitional state and are not used by the normal live runtime path.

## Import Legacy Brain Documents

For an existing vault-backed project, first keep a copy of the current `.brain/` and `.yard/yard.db` directories. Then import Markdown brain documents:

```bash
yard memory migrate --from-vault .brain --to .yard/shunter/project-memory
yard memory verify --from-vault .brain --to .yard/shunter/project-memory
yard brain index
```

`yard memory migrate` currently imports brain documents from `.brain/` into Shunter and preserves vault-relative paths and document content. It does not modify `.brain/`.

## Configure Shunter Mode

After verifying the import, update `yard.yaml`:

```yaml
memory:
  backend: shunter
  shunter_data_dir: .yard/shunter/project-memory
  durable_ack: true
  rpc:
    transport: unix
    path: .yard/run/memory.sock

brain:
  enabled: true
  backend: shunter
  vault_path: .brain
```

`brain.vault_path` remains useful for explicit migration/export commands, but it is not the live write target in Shunter mode.

## Still Planned

The current migration command does not yet import legacy SQLite conversations, messages, chains, launches, context reports, tool executions, or provider subcalls from `.yard/yard.db`. Keep the old `.yard/yard.db` until those imports are implemented or you no longer need the legacy history.
