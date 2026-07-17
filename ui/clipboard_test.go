package ui

import (
	"errors"
	"syscall"
	"testing"
)

func TestClipboardCallErrorUsesFallbackOnlyForEmptyLastError(t *testing.T) {
	if got := clipboardCallError(syscall.Errno(0)); !errors.Is(got, syscall.EINVAL) {
		t.Fatalf("zero last-error fallback = %v, want EINVAL", got)
	}
	want := syscall.Errno(5)
	if got := clipboardCallError(want); !errors.Is(got, want) {
		t.Fatalf("clipboard error = %v, want %v", got, want)
	}
}
