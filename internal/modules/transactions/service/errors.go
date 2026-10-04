package service

// ErrAmountMustBePositive exposes the existing validation value to delivery
// tests that previously shared this package; its identity and text are intact.
var ErrAmountMustBePositive = errAmountMustBePositive

// InvalidInputError retains the existing validation constructor across the
// package boundary introduced by the structural refactoring.
func InvalidInputError(message string) error { return invalidInputError(message) }

// FallbackText exposes the unchanged agent fallback across delivery boundaries.
const FallbackText = fallbackText
