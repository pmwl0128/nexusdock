package httpx

import mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

func centralReadOnlyAnnotations(openWorld bool) *mcpsdk.ToolAnnotations {
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: boolPointer(false),
		OpenWorldHint:   boolPointer(openWorld),
	}
}

func centralMutatingAnnotations(destructive, openWorld bool) *mcpsdk.ToolAnnotations {
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: boolPointer(destructive),
		OpenWorldHint:   boolPointer(openWorld),
	}
}

func boolPointer(value bool) *bool { return &value }

func centralToolOutputSchema(name string) map[string]any {
	properties := map[string]any{}
	object := func(description string) map[string]any {
		return map[string]any{"type": "object", "description": description, "additionalProperties": true}
	}
	arrayObjects := func(description string) map[string]any {
		return map[string]any{"type": "array", "description": description, "items": map[string]any{"type": "object", "additionalProperties": true}}
	}

	switch name {
	case "workflow_template_manage":
		properties["action"] = stringProperty("Completed workflow template action.")
		properties["template"] = object("Full workflow template returned by get.")
		properties["templates"] = arrayObjects("Compact summaries from list or full active templates from get_many.")
		properties["composition_required"] = booleanProperty("Whether the returned templates must be combined by the model before task creation.")
		properties["next_required_action"] = stringProperty("Required model action after get_many.")
		properties["template_id"] = stringProperty("Workflow template id returned by publish or retire.")
		properties["template_summary"] = object("Compact workflow template summary returned by publish, retire, and list items.")
		properties["count"] = integerProperty("Returned item count.")
		properties["workflow_dir"] = stringProperty("NexusDock workflow template registry directory.")
		properties["candidates"] = arrayObjects("Matched workflow template candidates with scores and reasons.")
		properties["vector_search_enabled"] = booleanProperty("Whether optional embedding-backed template vector search is enabled for match.")
		properties["vector_index_status"] = stringProperty("Template vector index status: disabled, ready, or degraded.")
		properties["vector_index_items"] = integerProperty("Number of persisted template vectors for the current embedding model.")
		properties["vector_index_available"] = booleanProperty("Whether workflow vector index content is available for export.")
		properties["content"] = stringProperty("Raw workflow vector index JSON returned by vector_index.")
		properties["embedding_model"] = stringProperty("Embedding model configured for template vector search.")
		properties["recommended"] = stringProperty("Template recommendation: use_template, consider_template, or plain_task.")
		properties["recommendation_reason"] = stringProperty("Reason for recommendation.")
		properties["best_candidate_score"] = integerProperty("Highest template match score.")
		properties["score_thresholds"] = object("Template match score thresholds.")
	case "recall_bootstrap":
		properties["recall_endpoint"] = stringProperty("NexusDock Recall endpoint.")
		properties["project"] = stringProperty("Backend-selected Recall context; not an input selector for the model.")
		properties["sections"] = arrayObjects("Packed Recall sections. Raw Markdown is returned only when include_raw=true.")
		properties["count"] = integerProperty("Section count.")
		properties["bytes"] = integerProperty("Combined bytes.")
	case "recall_search":
		properties["recall_endpoint"] = stringProperty("NexusDock Recall endpoint.")
		properties["recall_kind"] = stringProperty("Search kind used.")
		properties["query"] = stringProperty("Search query.")
		properties["recall_store"] = stringProperty("Recall store name.")
		properties["results"] = map[string]any{
			"type": "array", "description": "Recall search results with source identity fields.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"frontmatter":    object("Recall frontmatter metadata."),
					"matched_fields": arrayStringProperty("Matched fields."),
					"matched_terms":  arrayStringProperty("Matched terms."),
					"path":           stringProperty("Recall-relative document path."),
					"snippet":        stringProperty("Matched content snippet."),
					"id":             stringProperty("Stable Recall document id."),
					"title":          stringProperty("Human-readable document title."),
					"url":            stringProperty("Absolute source URL."),
				},
				"required": []string{"id", "title", "url"}, "additionalProperties": true,
			},
		}
		properties["count"] = integerProperty("Search result count.")
	case "recall_read":
		properties["recall_endpoint"] = stringProperty("NexusDock Recall endpoint.")
		properties["recall"] = object("NexusDock Recall document. Raw Markdown is returned only when include_raw=true.")
	case "recall_write":
		properties["recall_endpoint"] = stringProperty("NexusDock Recall endpoint.")
		properties["recall_target"] = stringProperty("Recall target used.")
		properties["recall_action"] = stringProperty("Recall action used.")
		properties["recall"] = object("NexusDock Recall document returned when a write occurs.")
		properties["card"] = object("Normalized card candidate or written card when target=card.")
		properties["warnings"] = arrayObjects("Review warnings before writing.")
		properties["capture_plan"] = object("Reviewable write plan for card captures.")
		properties["similar_results"] = arrayObjects("Similar existing card search results.")
		properties["path"] = stringProperty("NexusDock Recall-relative path.")
		properties["changed"] = booleanProperty("Whether the proposed edit changes content.")
		properties["dry_run"] = booleanProperty("Whether the operation only previewed changes.")
		properties["confirmed"] = booleanProperty("Whether write confirmation was supplied.")
		properties["written"] = booleanProperty("Whether the entry was written.")
		properties["diff"] = stringProperty("Unified diff preview.")
		properties["updates"] = arrayObjects("Fact update results.")
	case "recall_maintain":
		properties["recall_endpoint"] = stringProperty("NexusDock Recall endpoint.")
		properties["recall_action"] = stringProperty("Maintenance action performed.")
		properties["entries"] = arrayObjects("NexusDock Recall entries for action=list.")
		properties["count"] = integerProperty("Entry count where applicable.")
		properties["terms"] = arrayStringProperty("Terms used for action=lint.")
		properties["files_scanned"] = integerProperty("Files scanned for action=lint.")
		properties["finding_count"] = integerProperty("Finding count for action=lint.")
		properties["findings"] = arrayObjects("Lint findings.")
	case "private_note_manage":
		properties["root"] = stringProperty("NexusDock private notes root path.")
		properties["private_note_store"] = stringProperty("Private note store name, fixed to NexusDock Private Notes.")
		properties["action"] = stringProperty("Action performed.")
		properties["query"] = stringProperty("Metadata-only query for action=search.")
		properties["results"] = arrayObjects("Metadata-only private-note search results; never plaintext snippets.")
		properties["metadata_only"] = booleanProperty("Whether search was restricted to safe metadata.")
		properties["path"] = stringProperty("Plain note path for read/write/delete results.")
		properties["encrypted_path"] = stringProperty("Age encrypted backup path.")
		properties["content"] = stringProperty("Plaintext content returned only by explicit action=read.")
		properties["truncated"] = booleanProperty("Whether returned content was truncated.")
		properties["contains_secret"] = booleanProperty("Whether the note content is marked as containing secrets.")
		properties["written"] = booleanProperty("Whether plaintext was written.")
		properties["encrypted"] = booleanProperty("Whether encrypted backup was written.")
		properties["deleted_plaintext"] = booleanProperty("Whether plaintext was deleted.")
		properties["deleted_encrypted"] = booleanProperty("Whether encrypted backup was deleted.")
		properties["notes"] = arrayObjects("Metadata-only private note summaries for status/list.")
		properties["count"] = integerProperty("Result or note count.")
		properties["notes_count"] = integerProperty("Private note count for status checks.")
		properties["encrypted_count"] = integerProperty("Encrypted backup count for maintenance actions.")
		properties["recipient"] = stringProperty("Age public recipient generated or used.")
		properties["identity_created"] = booleanProperty("Whether a new local age identity was created.")
		properties["algorithm"] = stringProperty("Encryption algorithm.")
		properties["missing_encrypted"] = arrayStringProperty("Missing encrypted backup paths.")
		properties["encrypted_backup_ok"] = booleanProperty("Whether every private note has its required encrypted backup.")
		properties["plaintext_git_ignored"] = booleanProperty("Whether private note plaintext is Git-ignored.")
		properties["keys_git_ignored"] = booleanProperty("Whether private note keys are Git-ignored.")
	}

	return map[string]any{"type": "object", "properties": properties, "required": []string{}, "additionalProperties": true}
}
