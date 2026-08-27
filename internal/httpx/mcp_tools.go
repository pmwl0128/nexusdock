package httpx

import mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

const (
	agentDockContextUIResourceURI = "ui://agentdock/context"
	recallUIResourceURI           = "ui://agentdock/recall"
	workflowUIResourceURI         = "ui://agentdock/workflow"
)

func nexusToolDefinitions() []*mcpsdk.Tool {
	return []*mcpsdk.Tool{
		{
			Name: "agentdock_context", Title: "AgentDock fleet context",
			Description: "Return one combined context for all enabled AgentDock nodes, including node-local capabilities and Nexus-owned shared Workflow and Recall context.",
			InputSchema: objectSchema(map[string]any{}), OutputSchema: fleetAgentDockContextOutputSchema(),
			Annotations: centralReadOnlyAnnotations(false),
			Meta:        centralToolUIResourceMeta(agentDockContextUIResourceURI),
		},
		{Name: "recall_bootstrap", Title: "Bootstrap NexusDock Recall context", Description: "Load high-priority Recall context once from NexusDock.", InputSchema: objectSchema(map[string]any{
			"max_bytes":   integerProperty("Maximum combined Recall pack bytes."),
			"include_raw": booleanProperty("Include raw Markdown content."), "include_body": booleanProperty("Include full section bodies."),
		}), OutputSchema: centralToolOutputSchema("recall_bootstrap"), Annotations: centralReadOnlyAnnotations(false)},
		{Name: "recall_search", Title: "Search NexusDock Recall", Description: "Search Markdown documents and cards in the central Recall store.", InputSchema: requiredObjectSchema(map[string]any{
			"query": stringProperty("Text query."), "kind": enumProperty("all", "markdown", "card"), "max_results": integerProperty("Maximum results."),
		}, "query"), OutputSchema: centralToolOutputSchema("recall_search"), Annotations: centralReadOnlyAnnotations(false), Meta: centralToolUIResourceMeta(recallUIResourceURI)},
		{Name: "recall_read", Title: "Read NexusDock Recall entry", Description: "Read one central Recall entry by path.", InputSchema: requiredObjectSchema(map[string]any{
			"path": stringProperty("Recall-relative path."), "include_raw": booleanProperty("Include raw Markdown."),
		}, "path"), OutputSchema: centralToolOutputSchema("recall_read"), Annotations: centralReadOnlyAnnotations(false)},
		{Name: "recall_write", Title: "Write NexusDock Recall entry", Description: "Plan, create, replace, append, patch, update facts, diff, or delete central Recall content. The model must choose target and action explicitly.", InputSchema: requiredObjectSchema(map[string]any{
			"target": enumProperty("card", "markdown"), "action": enumProperty("plan", "create", "replace", "append", "patch", "update_fact", "diff", "delete"),
			"confirmed": booleanProperty("Required for true writes/deletes. card create with confirmed=false returns a review plan."), "path": stringProperty("Recall-relative path."),
			"content": stringProperty("Card or Markdown content, or proposed replacement content."), "title": stringProperty("Short title for a card or Markdown entry."),
			"summary": stringProperty("Short summary for a card."), "overwrite": booleanProperty("Replace an existing entry when supported."),
			"allow_warnings": booleanProperty("Card only: after reviewing warnings, allow writing a warned card. Do not use by default."),
			"old":            stringProperty("Patch only: literal text to replace."), "new": stringProperty("Patch only: replacement text for old."),
			"append": stringProperty("Append/patch only: text to append to the Recall document."), "section": stringProperty("Patch/update_fact only: Markdown heading title whose section should be updated."),
			"section_content": stringProperty("Patch only: new body for the selected Markdown section."), "key": stringProperty("Update_fact only: fact key to update."),
			"value": stringProperty("Update_fact only: new fact value."), "facts": mapStringProperty("Update_fact only: multiple key/value facts to update."),
			"append_if_missing": booleanProperty("Update_fact only: append missing keys to the selected section or document instead of failing."), "max_bytes": integerProperty("Maximum diff/output bytes."),
		}, "target", "action"), OutputSchema: centralToolOutputSchema("recall_write"), Annotations: centralMutatingAnnotations(true, false), Meta: centralToolUIResourceMeta(recallUIResourceURI)},
		{Name: "recall_maintain", Title: "Maintain NexusDock Recall", Description: "Inspect sync/index state or rebuild the central Recall index.", InputSchema: objectSchema(map[string]any{
			"action": enumProperty("list", "lint", "embedding_status", "reindex", "reindex_cards"), "prefix": stringProperty("Optional prefix."), "max_entries": integerProperty("Maximum entries."),
			"terms": arrayStringProperty("Terms or regular expressions to find."), "regex": booleanProperty("Treat terms as regular expressions."), "max_findings": integerProperty("Maximum lint findings."),
			"max_results": integerProperty("Maximum results where supported."),
		}), OutputSchema: centralToolOutputSchema("recall_maintain"), Annotations: centralMutatingAnnotations(true, false)},
		{Name: "private_note_manage", Title: "Manage private notes", Description: "Explicit low-frequency entrypoint for sensitive private notes.", InputSchema: requiredObjectSchema(map[string]any{
			"action": enumProperty("search", "read", "write", "delete", "status", "maintain"), "query": stringProperty("Metadata query."),
			"max_results": boundedIntegerProperty("Maximum results.", 1, 100), "path": stringProperty("Private-note path."), "category": stringProperty("Category."),
			"title": stringProperty("Title."), "summary": stringProperty("Safe summary."), "tags": arrayStringProperty("Safe tags."),
			"content": stringProperty("Plaintext content."), "confirmed": booleanProperty("Required for mutations."), "overwrite": booleanProperty("Allow replacement."),
			"max_bytes": boundedIntegerProperty("Maximum read bytes.", 1, 1048576), "status_action": enumProperty("check", "list"),
			"maintenance_action": enumProperty("init", "init-encryption", "sync-encrypted", "encrypt-all"),
		}, "action"), OutputSchema: centralToolOutputSchema("private_note_manage"), Annotations: centralMutatingAnnotations(true, false)},
		{
			Name: "workflow_template_manage", Title: "Manage workflow templates",
			Description: "List, get, get multiple, publish, retire, or match NexusDock workflow templates. get_many requires the model to compose the returned templates before task creation.",
			InputSchema: workflowTemplateManageInputSchema(), OutputSchema: centralToolOutputSchema("workflow_template_manage"),
			Annotations: centralMutatingAnnotations(true, false),
		},
	}
}

func centralToolUIResourceMeta(uri string) mcpsdk.Meta {
	return mcpsdk.Meta{"ui": map[string]any{"resourceUri": uri}}
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

func boundedIntegerProperty(description string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum, "maximum": maximum}
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
