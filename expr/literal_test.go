package expr

// mustParse is Parse that panics on an error (for expressions written in Go).
func mustParse(src string) *Expr {
	e, err := Parse(src)
	if err != nil {
		panic(err)
	}
	return e
}
