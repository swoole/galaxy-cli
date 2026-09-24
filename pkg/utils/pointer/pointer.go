package pointer

// String returns a pointer to s.
func String(s string) *string {
	return &s
}
