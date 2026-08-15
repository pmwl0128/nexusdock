package recall

import "testing"

func TestRecallScopeSetMatchesContract(t *testing.T) {
	valid := []Scope{ScopeProfile, ScopeGlobal, ScopeProject, ScopeDevice, ScopeAgent, ScopeOps, ScopeInbox}
	for _, scope := range valid {
		if !scope.Valid() {
			t.Fatalf("scope %q should be valid", scope)
		}
	}
	if Scope("domain").Valid() {
		t.Fatalf("domain scope should not be valid unless it is documented in the public contract")
	}
}
