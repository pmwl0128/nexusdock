package agentdock

import "testing"

func TestPublishedToolContractPersistsDescriptorAndSource(t *testing.T) {
	store, db := newTestStore(t)
	descriptor := ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"cmd": map[string]any{"type": "string"}},
		},
	}
	if err := store.SavePublishedToolContract(t.Context(), PublishedToolContract{
		ToolName: "exec_command", Descriptor: descriptor, SourceNodeID: "node_source", SourceVersion: "1.9.0",
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := reopened.ListPublishedToolContracts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 1 || contracts[0].ToolName != "exec_command" || contracts[0].SourceVersion != "1.9.0" {
		t.Fatalf("contracts = %#v", contracts)
	}
	properties := contracts[0].Descriptor.InputSchema["properties"].(map[string]any)
	if properties["cmd"].(map[string]any)["type"] != "string" {
		t.Fatalf("descriptor = %#v", contracts[0].Descriptor)
	}
}
