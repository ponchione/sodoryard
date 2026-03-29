# Task 07: DeleteByFilePath Operation

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Implement the `DeleteByFilePath` method that removes all chunks for a given file path. This is used during re-indexing when a file is deleted or fully re-parsed: the old chunks are removed before new ones are inserted. The delete is scoped to the store's project name.

## Acceptance Criteria

- [ ] Method signature: `DeleteByFilePath(ctx context.Context, filePath string) error`
- [ ] Deletes all records where `file_path = '<filePath>' AND project_name = '<projectName>'`
- [ ] If no records match the file path, the operation is a no-op (returns nil, no error)
- [ ] Returns a descriptive error on delete failure (includes the file path in the error message)
- [ ] Implements the `DeleteByFilePath` method of the `Store` interface from L1-E01

## Sizing Note

Estimated ~30-45 minutes. Single method with straightforward filter-delete pattern. Standalone task is justified by distinct error handling (project scoping, no-op on miss) and separate testability.
