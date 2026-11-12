//go:build !windows

package main

import "os"

// fora do Windows não há janela de escolher arquivo (arraste a imagem); MESA_PICK serve para testes.
var canPick = os.Getenv("MESA_PICK") != ""

func pickImage() string { return os.Getenv("MESA_PICK") }
