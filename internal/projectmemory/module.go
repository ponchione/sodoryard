package projectmemory

import (
	"github.com/ponchione/shunter"
	"github.com/ponchione/shunter/schema"
)

const ModuleName = "yard_project_memory"

const schemaVersion = 1

const (
	tableProjectState schema.TableID = iota
	tableDocuments
	tableDocumentChunks
	tableDocumentRevisions
	tableMemoryOperations
	tableBrainIndexState
)

const (
	indexProjectStatePrimary schema.IndexID = iota
)

const (
	indexDocumentsPrimary schema.IndexID = iota
	indexDocumentsKind
	indexDocumentsUpdated
)

const (
	indexDocumentChunksPrimary schema.IndexID = iota
	indexDocumentChunksPath
)

const (
	indexDocumentRevisionsPrimary schema.IndexID = iota
	indexDocumentRevisionsPath
)

const (
	indexMemoryOperationsPrimary schema.IndexID = iota
	indexMemoryOperationsPath
)

const (
	indexBrainIndexStatePrimary schema.IndexID = iota
)

func NewModule() *shunter.Module {
	mod := shunter.NewModule(ModuleName).SchemaVersion(schemaVersion)
	declareProjectState(mod)
	declareDocuments(mod)
	declareDocumentChunks(mod)
	declareDocumentRevisions(mod)
	declareMemoryOperations(mod)
	declareBrainIndexState(mod)
	mod.Reducer("write_document", writeDocumentReducer)
	mod.Reducer("patch_document", patchDocumentReducer)
	mod.Reducer("delete_document", deleteDocumentReducer)
	mod.Reducer("import_documents_batch", importDocumentsBatchReducer)
	mod.Reducer("mark_brain_index_dirty", markBrainIndexDirtyReducer)
	mod.Reducer("mark_brain_index_clean", markBrainIndexCleanReducer)
	return mod
}

func declareProjectState(mod *shunter.Module) {
	mod.TableDef(schema.TableDefinition{
		Name: "project_state",
		Columns: []schema.ColumnDefinition{
			{Name: "project_id", Type: schema.KindString, PrimaryKey: true},
			{Name: "schema_version", Type: schema.KindUint32},
			{Name: "created_at_us", Type: schema.KindUint64},
			{Name: "updated_at_us", Type: schema.KindUint64},
			{Name: "metadata_json", Type: schema.KindString},
		},
	})
}

func declareDocuments(mod *shunter.Module) {
	mod.TableDef(schema.TableDefinition{
		Name: "documents",
		Columns: []schema.ColumnDefinition{
			{Name: "path", Type: schema.KindString, PrimaryKey: true},
			{Name: "kind", Type: schema.KindString},
			{Name: "title", Type: schema.KindString},
			{Name: "content_hash", Type: schema.KindString},
			{Name: "content_size", Type: schema.KindUint64},
			{Name: "chunk_count", Type: schema.KindUint32},
			{Name: "created_at_us", Type: schema.KindUint64},
			{Name: "updated_at_us", Type: schema.KindUint64},
			{Name: "deleted", Type: schema.KindBool},
			{Name: "tags_json", Type: schema.KindString},
			{Name: "metadata_json", Type: schema.KindString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "documents_kind", Columns: []string{"kind"}},
			{Name: "documents_updated", Columns: []string{"updated_at_us"}},
		},
	})
}

func declareDocumentChunks(mod *shunter.Module) {
	mod.TableDef(schema.TableDefinition{
		Name: "document_chunks",
		Columns: []schema.ColumnDefinition{
			{Name: "chunk_id", Type: schema.KindString, PrimaryKey: true},
			{Name: "path", Type: schema.KindString},
			{Name: "chunk_index", Type: schema.KindUint32},
			{Name: "body", Type: schema.KindString},
			{Name: "body_hash", Type: schema.KindString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "document_chunks_path", Columns: []string{"path"}},
		},
	})
}

func declareDocumentRevisions(mod *shunter.Module) {
	mod.TableDef(schema.TableDefinition{
		Name: "document_revisions",
		Columns: []schema.ColumnDefinition{
			{Name: "revision_id", Type: schema.KindString, PrimaryKey: true},
			{Name: "path", Type: schema.KindString},
			{Name: "revision", Type: schema.KindUint32},
			{Name: "content_hash", Type: schema.KindString},
			{Name: "operation_id", Type: schema.KindString},
			{Name: "created_at_us", Type: schema.KindUint64},
			{Name: "summary", Type: schema.KindString},
			{Name: "actor", Type: schema.KindString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "document_revisions_path", Columns: []string{"path"}},
		},
	})
}

func declareMemoryOperations(mod *shunter.Module) {
	mod.TableDef(schema.TableDefinition{
		Name: "memory_operations",
		Columns: []schema.ColumnDefinition{
			{Name: "operation_id", Type: schema.KindString, PrimaryKey: true},
			{Name: "operation_type", Type: schema.KindString},
			{Name: "path", Type: schema.KindString},
			{Name: "actor", Type: schema.KindString},
			{Name: "created_at_us", Type: schema.KindUint64},
			{Name: "before_hash", Type: schema.KindString},
			{Name: "after_hash", Type: schema.KindString},
			{Name: "payload_json", Type: schema.KindString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "memory_operations_path", Columns: []string{"path"}},
		},
	})
}

func declareBrainIndexState(mod *shunter.Module) {
	mod.TableDef(schema.TableDefinition{
		Name: "brain_index_state",
		Columns: []schema.ColumnDefinition{
			{Name: "project_id", Type: schema.KindString, PrimaryKey: true},
			{Name: "last_indexed_at_us", Type: schema.KindUint64},
			{Name: "dirty", Type: schema.KindBool},
			{Name: "dirty_since_us", Type: schema.KindUint64},
			{Name: "dirty_reason", Type: schema.KindString},
			{Name: "metadata_json", Type: schema.KindString},
		},
	})
}
