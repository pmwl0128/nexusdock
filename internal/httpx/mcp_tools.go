package httpx

import mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

func nexusToolDefinitions() []*mcpsdk.Tool {
	return []*mcpsdk.Tool{
		{Name: "node_list", Title: "List AgentDock nodes", Description: "List paired AgentDock nodes and their stable node_id, online state, platform, version, and capabilities.", InputSchema: objectSchema(map[string]any{})},
		{Name: "recall_bootstrap", Title: "Bootstrap NexusDock Recall context", Description: "Load high-priority Recall context once from NexusDock.", InputSchema: objectSchema(map[string]any{
			"max_bytes":   integerProperty("Maximum combined Recall pack bytes."),
			"include_raw": booleanProperty("Include raw Markdown content."), "include_body": booleanProperty("Include full section bodies."),
		})},
		{Name: "recall_search", Title: "Search NexusDock Recall", Description: "Search Markdown documents and cards in the central Recall store.", InputSchema: requiredObjectSchema(map[string]any{
			"query": stringProperty("Text query."), "kind": enumProperty("all", "markdown", "card"), "max_results": integerProperty("Maximum results."),
		}, "query")},
		{Name: "recall_read", Title: "Read NexusDock Recall entry", Description: "Read one central Recall entry by path.", InputSchema: requiredObjectSchema(map[string]any{
			"path": stringProperty("Recall-relative path."), "include_raw": booleanProperty("Include raw Markdown."),
		}, "path")},
		{Name: "recall_write", Title: "Write NexusDock Recall entry", Description: "Plan or mutate a central Recall card or Markdown document.", InputSchema: requiredObjectSchema(map[string]any{
			"target": enumProperty("card", "markdown"), "action": enumProperty("plan", "create", "replace", "append", "patch", "update_fact", "diff", "delete"),
			"confirmed": booleanProperty("Required for persistent mutations."), "path": stringProperty("Recall-relative path."),
			"content": stringProperty("Content."), "title": stringProperty("Title."), "summary": stringProperty("Summary."),
			"type": stringProperty("Card or Markdown type."), "scope": stringProperty("Recall scope."), "project": stringProperty("Project."),
			"source": stringProperty("Source."), "confidence": stringProperty("Confidence."), "tags": arrayStringProperty("Tags."),
			"overwrite": booleanProperty("Allow replacement."), "allow_warnings": booleanProperty("Allow reviewed card warnings."),
			"old": stringProperty("Patch text to replace."), "new": stringProperty("Patch replacement."), "append": stringProperty("Text to append."),
			"section": stringProperty("Optional Markdown heading containing facts."), "key": stringProperty("Fact key."), "value": stringProperty("Fact value."),
			"facts": mapStringProperty("Multiple fact key/value replacements."), "append_if_missing": booleanProperty("Append missing facts."),
			"dry_run": booleanProperty("Preview without writing."), "max_bytes": integerProperty("Maximum diff bytes."),
		}, "target", "action")},
		{Name: "recall_maintain", Title: "Maintain NexusDock Recall", Description: "Inspect sync/index state or rebuild the central Recall index.", InputSchema: objectSchema(map[string]any{
			"action": enumProperty("sync_status", "list", "lint", "embedding_status", "reindex", "reindex_cards"), "prefix": stringProperty("Optional prefix."), "max_entries": integerProperty("Maximum entries."),
			"terms": arrayStringProperty("Terms or regular expressions to find."), "regex": booleanProperty("Treat terms as regular expressions."), "max_findings": integerProperty("Maximum lint findings."),
		})},
		{Name: "private_note_manage", Title: "Manage private notes", Description: "Explicit low-frequency entrypoint for sensitive private notes.", InputSchema: requiredObjectSchema(map[string]any{
			"action": enumProperty("search", "read", "write", "delete", "status", "maintain"), "query": stringProperty("Metadata query."),
			"max_results": integerProperty("Maximum results."), "path": stringProperty("Private-note path."), "category": stringProperty("Category."),
			"title": stringProperty("Title."), "summary": stringProperty("Safe summary."), "tags": arrayStringProperty("Safe tags."),
			"content": stringProperty("Plaintext content."), "confirmed": booleanProperty("Required for mutations."), "overwrite": booleanProperty("Allow replacement."),
			"max_bytes": integerProperty("Maximum read bytes."), "status_action": enumProperty("check", "list"),
			"maintenance_action": enumProperty("init", "init-encryption", "sync-encrypted", "encrypt-all"),
		}, "action")},
	}
}

func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}

func requiredObjectSchema(properties map[string]any, required ...string) map[string]any {
	schema := objectSchema(properties)
	schema["required"] = required
	return schema
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerProperty(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func booleanProperty(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func enumProperty(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func arrayStringProperty(description string) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": map[string]any{"type": "string"}}
}

func mapStringProperty(description string) map[string]any {
	return map[string]any{"type": "object", "description": description, "additionalProperties": map[string]any{"type": "string"}}
}
