package evolution

import (
	"fmt"
	"sort"
	"strings"
)

func ComputePlanDelta(original, final []PlanStep) PlanDelta {
	before := make(map[string]PlanStep, len(original))
	after := make(map[string]PlanStep, len(final))
	for _, step := range original {
		before[planKey(step)] = step
	}
	for _, step := range final {
		after[planKey(step)] = step
	}

	delta := PlanDelta{}
	for key, step := range after {
		old, ok := before[key]
		if !ok {
			delta.Added = append(delta.Added, step)
			continue
		}
		if old.Action != step.Action || old.Outcome != step.Outcome || old.Validation != step.Validation {
			b, a := old, step
			delta.Changed = append(delta.Changed, PlanChange{Name: step.Name, Before: &b, After: &a})
		}
	}
	for key, step := range before {
		if _, ok := after[key]; !ok {
			delta.Removed = append(delta.Removed, step)
		}
	}
	sort.Slice(delta.Added, func(i, j int) bool { return planKey(delta.Added[i]) < planKey(delta.Added[j]) })
	sort.Slice(delta.Removed, func(i, j int) bool { return planKey(delta.Removed[i]) < planKey(delta.Removed[j]) })
	sort.Slice(delta.Changed, func(i, j int) bool { return delta.Changed[i].Name < delta.Changed[j].Name })
	delta.Summary = fmt.Sprintf("successful path added %d, removed %d, changed %d step(s)", len(delta.Added), len(delta.Removed), len(delta.Changed))
	return delta
}

func planKey(step PlanStep) string {
	key := strings.ToLower(strings.TrimSpace(step.Name))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(step.Action))
	}
	return key
}

func hasPlanDelta(original, final []PlanStep) bool {
	d := ComputePlanDelta(original, final)
	return len(d.Added)+len(d.Removed)+len(d.Changed) > 0
}
