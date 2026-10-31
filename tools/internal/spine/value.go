package spine

import "github.com/23min/verifier-kit/tools/internal/yamldoc"

// isText reports whether v is text.
func isText(v *yamldoc.Value) bool { return v != nil && v.Kind == yamldoc.KStr }

// isBool reports whether v is true or false.
func isBool(v *yamldoc.Value) bool { return v != nil && v.Kind == yamldoc.KBool }

// isTrue reports whether v is true.
func isTrue(v *yamldoc.Value) bool { return isBool(v) && v.Bool }
