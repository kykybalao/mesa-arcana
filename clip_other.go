//go:build !windows

package main

import "os"

// fora do Windows não há leitura da área de transferência; MESA_CLIP serve para testes.
func clipboardText() (string, bool) {
	if p := os.Getenv("MESA_CLIP"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			return string(b), true
		}
	}
	return "", false
}
