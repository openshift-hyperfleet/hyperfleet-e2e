package pointer

import "testing"

func TestToPtr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		val  int
	}{
		{name: "zero", val: 0},
		{name: "positive", val: 42},
		{name: "negative", val: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := ToPtr(tt.val)
			if p == nil {
				t.Fatal("ToPtr returned nil")
			}
			if *p != tt.val {
				t.Errorf("ToPtr(%d) = %d, want %d", tt.val, *p, tt.val)
			}
		})
	}
}

func TestFromPtr(t *testing.T) {
	t.Parallel()
	t.Run("non-nil int", func(t *testing.T) {
		t.Parallel()
		v := 42
		if got := FromPtr(&v); got != 42 {
			t.Errorf("FromPtr(&42) = %d, want 42", got)
		}
	})
	t.Run("nil int returns zero", func(t *testing.T) {
		t.Parallel()
		if got := FromPtr[int](nil); got != 0 {
			t.Errorf("FromPtr[int](nil) = %d, want 0", got)
		}
	})
	t.Run("nil string returns empty", func(t *testing.T) {
		t.Parallel()
		if got := FromPtr[string](nil); got != "" {
			t.Errorf("FromPtr[string](nil) = %q, want empty", got)
		}
	})
}
