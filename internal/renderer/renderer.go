package renderer

// Output is rendered configure content for apply.
type Output struct {
	Target            string
	UUID              string
	Text              string
	PhysicalPortRoles map[string]string
}

// IsZero reports whether Output is the zero value.
func (o Output) IsZero() bool {
	return o.Target == "" && o.UUID == "" && o.Text == "" && len(o.PhysicalPortRoles) == 0
}
