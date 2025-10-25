package main

import (
	"bytes"
	"testing"
)

func TestCompletionCommandSupportsShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var output bytes.Buffer
		rootCmd.SetOut(&output)
		rootCmd.SetArgs([]string{"completion", shell})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
		if output.Len() == 0 {
			t.Fatalf("completion %s produced no output", shell)
		}
	}
}
