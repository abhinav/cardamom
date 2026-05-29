package store

import (
	"strings"
	"testing"
)

func TestAncestorContextOrdering(t *testing.T) {
	s := newTestStore(t)
	// z ← a ← b ← target  (b needs a needs z; target needs b)
	z, _ := s.Create(ctx, "z", "task", 2, nil)
	a, _ := s.Create(ctx, "a", "task", 2, nil)
	b, _ := s.Create(ctx, "b", "task", 2, nil)
	target, _ := s.Create(ctx, "target", "task", 2, nil)
	must(t, s.AddDep(ctx, a.ID, z.ID))
	must(t, s.AddDep(ctx, b.ID, a.ID))
	must(t, s.AddDep(ctx, target.ID, b.ID))

	got, err := s.AncestorContext(ctx, target.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{z.ID, a.ID, b.ID} // most-upstream first
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}

	// Depth cap: only direct parents (depth 1).
	d1, _ := s.AncestorContext(ctx, target.ID, 1)
	if len(d1) != 1 || d1[0] != b.ID {
		t.Fatalf("depth 1 = %v, want [%s]", d1, b.ID)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
