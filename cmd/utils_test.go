package cmd

// ptrBool returns a pointer to a boolean value
func ptrBool(b bool) *bool {
	return &b
}

// ptrString returns a pointer to a string value
func ptrString(s string) *string {
	return &s
}
