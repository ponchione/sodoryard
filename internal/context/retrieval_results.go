package context

// BuildRetrievalResults converts the source-specific context report result
// slices into a common shape for inspection and deterministic evals.
func BuildRetrievalResults(rag []RAGHit, brain []BrainHit, graph []GraphHit, files []FileResult) []RetrievalResult {
	out := make([]RetrievalResult, 0, len(rag)+len(brain)+len(graph)+len(files))
	for _, hit := range rag {
		metadata := map[string]any{
			"chunk_id": hit.ChunkID,
		}
		if hit.Signature != "" {
			metadata["signature"] = hit.Signature
		}
		if hit.Language != "" {
			metadata["language"] = hit.Language
		}
		if hit.ChunkType != "" {
			metadata["chunk_type"] = string(hit.ChunkType)
		}
		if hit.LineStart > 0 {
			metadata["line_start"] = hit.LineStart
		}
		if hit.LineEnd > 0 {
			metadata["line_end"] = hit.LineEnd
		}
		if hit.MatchedBy != "" {
			metadata["matched_by"] = hit.MatchedBy
		}
		if len(hit.Sources) > 0 {
			metadata["sources"] = append([]string(nil), hit.Sources...)
		}
		out = append(out, RetrievalResult{
			Source:          "code",
			Kind:            "code_chunk",
			Path:            hit.FilePath,
			Symbol:          hit.Name,
			Score:           hit.SimilarityScore,
			Content:         hit.Body,
			Included:        hit.Included,
			ExclusionReason: hit.ExclusionReason,
			Metadata:        metadata,
		})
	}
	for _, hit := range brain {
		metadata := map[string]any{}
		if hit.Title != "" {
			metadata["title"] = hit.Title
		}
		if hit.SectionHeading != "" {
			metadata["section_heading"] = hit.SectionHeading
		}
		if hit.MatchMode != "" {
			metadata["match_mode"] = hit.MatchMode
		}
		if hit.LexicalScore != 0 {
			metadata["lexical_score"] = hit.LexicalScore
		}
		if hit.SemanticScore != 0 {
			metadata["semantic_score"] = hit.SemanticScore
		}
		if hit.GraphSourcePath != "" {
			metadata["graph_source_path"] = hit.GraphSourcePath
		}
		if hit.GraphHopDepth > 0 {
			metadata["graph_hop_depth"] = hit.GraphHopDepth
		}
		if len(hit.MatchSources) > 0 {
			metadata["match_sources"] = append([]string(nil), hit.MatchSources...)
		}
		if len(hit.Tags) > 0 {
			metadata["tags"] = append([]string(nil), hit.Tags...)
		}
		out = append(out, RetrievalResult{
			Source:          "brain",
			Kind:            "brain_doc",
			Path:            hit.DocumentPath,
			Symbol:          hit.SectionHeading,
			Score:           hit.MatchScore,
			Content:         hit.Snippet,
			Included:        hit.Included,
			ExclusionReason: hit.ExclusionReason,
			Metadata:        metadataOrNil(metadata),
		})
	}
	for _, hit := range graph {
		metadata := map[string]any{}
		if hit.ChunkID != "" {
			metadata["chunk_id"] = hit.ChunkID
		}
		if hit.RelationshipType != "" {
			metadata["relationship_type"] = hit.RelationshipType
		}
		if hit.Depth > 0 {
			metadata["depth"] = hit.Depth
		}
		if hit.LineStart > 0 {
			metadata["line_start"] = hit.LineStart
		}
		if hit.LineEnd > 0 {
			metadata["line_end"] = hit.LineEnd
		}
		out = append(out, RetrievalResult{
			Source:          "graph",
			Kind:            "symbol",
			Path:            hit.FilePath,
			Symbol:          hit.SymbolName,
			Included:        hit.Included,
			ExclusionReason: hit.ExclusionReason,
			Metadata:        metadataOrNil(metadata),
		})
	}
	for _, file := range files {
		metadata := map[string]any{}
		if file.Truncated {
			metadata["truncated"] = true
		}
		out = append(out, RetrievalResult{
			Source:          "explicit_file",
			Kind:            "explicit_file",
			Path:            file.FilePath,
			Content:         file.Content,
			TokenEstimate:   file.TokenCount,
			Included:        file.Included,
			ExclusionReason: file.ExclusionReason,
			Metadata:        metadataOrNil(metadata),
		})
	}
	return out
}

func metadataOrNil(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func ensureUnifiedRetrievalResults(report *ContextAssemblyReport) {
	if report == nil || len(report.UnifiedResults) > 0 {
		return
	}
	report.UnifiedResults = BuildRetrievalResults(report.RAGResults, report.BrainResults, report.GraphResults, report.ExplicitFileResults)
}
