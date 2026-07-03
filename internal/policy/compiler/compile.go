package compiler

import (
	"slices"
	"sort"
)

// Compile validates a structured policy and deterministically renders it into a
// Rego module compatible with the rbitr policy contract. The input is never
// mutated. The returned module is stable across repeated calls for equal input,
// which keeps golden-file tests meaningful.
func Compile(p *StructuredPolicy) (string, error) {
	if err := Validate(p); err != nil {
		return "", err
	}
	return render(p, sortRules(p.Rules)), nil
}

// sortRules returns a new slice ordered by descending priority. In an else-chain
// the textual order decides the winner, so emitting in priority order makes the
// numeric priority and the actual evaluation order agree. A stable sort preserves
// original order for equal priorities. The input slice is not modified.
func sortRules(rules []Rule) []Rule {
	sorted := slices.Clone(rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})
	return sorted
}
