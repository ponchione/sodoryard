# Shunter Project Memory Migration

New projects created by `yard init` use Shunter project memory by default:

```yaml
memory:
  backend: shunter
brain:
  backend: shunter
```

In this mode, `.yard/shunter/project-memory` is the canonical project-memory store. `.brain/` and `.yard/yard.db` are legacy/transitional state and are not used by the normal live runtime path.

## Import Legacy Brain Documents And SQLite State

For an existing vault-backed project, first keep a copy of the current `.brain/` and `.yard/yard.db` paths. Then import Markdown brain documents:

```bash
yard memory migrate --from-vault .brain --to .yard/shunter/project-memory
yard memory verify --from-vault .brain --to .yard/shunter/project-memory
yard brain index
```

`yard memory migrate` imports brain documents from `.brain/` into Shunter and preserves vault-relative paths and document content. It does not modify `.brain/`.

To import the highest-value legacy runtime history from SQLite, run:

```bash
yard memory migrate --from-sqlite .yard/yard.db --to .yard/shunter/project-memory
```

The SQLite import is explicit and one-way. It reads the source database during migration only, then stores conversations, messages, chains, steps, events, launch drafts, launch presets, context reports, tool executions, and provider subcalls in Shunter. It does not make `.yard/yard.db` a live runtime dependency in Shunter mode.

If you want both Markdown documents and SQLite runtime state in one pass, provide both sources:

```bash
yard memory migrate \
  --from-vault .brain \
  --from-sqlite .yard/yard.db \
  --to .yard/shunter/project-memory
```

## Export Markdown Documents

Shunter brain documents can be exported back to a Markdown vault for backup or round-trip verification:

```bash
yard memory export --from .yard/shunter/project-memory --to-vault ./backup/brain-markdown
yard memory verify --from-vault ./backup/brain-markdown --to .yard/shunter/project-memory
```

Export writes Markdown documents to the requested vault path. It does not make that path the live write target unless the project is separately configured for the legacy vault backend.

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

## Remaining Gaps

The SQLite import covers the legacy runtime history listed above. It does not import derived index rows such as legacy `index_state`, `brain_documents`, or `brain_links`; those should be rebuilt from Shunter documents with `yard brain index` and `yard index` as needed. Keep old `.brain/` and `.yard/yard.db` backups until you have verified the migration and no longer need legacy-only derived metadata.
