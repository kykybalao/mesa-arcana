package main

// psp.go: exporta um deck para o Mesa Arcana de PSP. O PC faz o trabalho pesado: grava o texto das
// cartas (no idioma escolhido) e uma imagem pequena e uma grande de cada face, já no estilo da mesa.
// Tudo vai para PSP/GAME/MesaArcana/decks/<deck>, ao lado do programa; é só copiar a pasta PSP
// para a raiz do Memory Stick.

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const (
	pspSmW, pspSmH = 64, 89   // carta na mesa do PSP
	pspBgW, pspBgH = 186, 260 // carta ampliada
)

func pspDir() string { return filepath.Join(baseDir, "PSP", "GAME", "MesaArcana") }

func pspField(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.ReplaceAll(s, "\n", `\n`)
}

// faceRGBA: a carta como aparece na mesa do PC (imagem oficial, moldura do programa ou proxy)
func (g *Game) faceRGBA(c *Card, fi int) image.Image {
	key := c.ID + ":" + string(rune('0'+fi))
	if (!c.Custom && cfg.Table == "") || (c.Mine && c.Est == "") {
		if im := decodeRounded(imgPath(key)); im != nil {
			return im
		}
	}
	tex := g.proxy(c, fi)
	b := tex.Bounds()
	im := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	tex.ReadPixels(im.Pix)
	return im
}

func pspJPEG(src image.Image, w, h int, path string) error {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.Black, image.Point{}, draw.Src) // cantos transparentes ficam pretos
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 88}); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// exportPSP grava o deck no formato do PSP e devolve a pasta
func (g *Game) exportPSP(d *Deck) (string, int, error) {
	dir := filepath.Join(pspDir(), "decks", safeName(d.Name))
	os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o755); err != nil {
		return "", 0, err
	}
	cmd := map[string]bool{}
	for _, id := range d.Cmd {
		cmd[id] = true
	}
	var sb strings.Builder
	sb.WriteString("MESA1\nNOME\t" + pspField(d.Name) + "\n")
	idx, imgs := 0, 0
	put := func(c *Card, qty int, isCmd, token bool) error {
		faces := 1
		if len(c.F) > 1 {
			faces = len(c.F)
		}
		b2i := func(b bool) int {
			if b {
				return 1
			}
			return 0
		}
		fmt.Fprintf(&sb, "CARTA\t%d\t%d\t%d\t%d\t%d\n", idx, qty, b2i(isCmd), b2i(token), faces)
		for fi := 0; fi < faces; fi++ {
			f, _ := loc(c, fi)
			fmt.Fprintf(&sb, "FACE\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", pspField(f.N), pspField(f.C), pspField(f.T), pspField(f.X), pspField(f.P), pspField(f.O), pspField(f.L))
			im := g.faceRGBA(c, fi)
			for _, sz := range []struct {
				w, h int
				suf  string
			}{{pspSmW, pspSmH, ""}, {pspBgW, pspBgH, "g"}} {
				if err := pspJPEG(im, sz.w, sz.h, filepath.Join(dir, "img", fmt.Sprintf("%d_%d%s.jpg", idx, fi, sz.suf))); err != nil {
					return err
				}
			}
			imgs++
		}
		idx++
		return nil
	}
	for _, e := range d.Entries {
		if err := put(e.Card, e.Qty, cmd[e.Card.ID], false); err != nil {
			return "", 0, err
		}
	}
	for _, id := range d.Tokens {
		if c := cardByID[id]; c != nil {
			if err := put(c, 0, false, true); err != nil {
				return "", 0, err
			}
		}
	}
	g.clearProxies()
	return dir, imgs, os.WriteFile(filepath.Join(dir, "deck.txt"), []byte(sb.String()), 0o644)
}

func (g *Game) pspModal(d *Deck) {
	dir, n, err := g.exportPSP(d)
	msg := ""
	if err != nil {
		msg = "Não consegui gravar o pacote: " + err.Error()
	} else {
		msg = fmt.Sprintf("Pronto: %d cartas (%d imagens) gravadas em\n%s\n\nCopie a pasta PSP (ao lado do programa) para a raiz do Memory Stick, junto com o MesaArcana do PSP. As cartas saem no estilo escolhido em \"Cartas na mesa\" e no idioma atual.", strings.Count(mustRead(filepath.Join(dir, "deck.txt")), "\nCARTA\t"), n*2, dir)
	}
	g.openModal(&Modal{title: "Exportar para PSP", w: 640, h: 300, draw: func(x, y, w, h float64) {
		ls := wrap(msg, fReg, 13.5, w)
		txt(g.dst, strings.Join(ls, "\n"), x, y, fReg, 13.5, cTxt)
		if err == nil {
			g.button(x, y+h-32, 0, 30, "Abrir a pasta", 0, func() { openFolder(pspDir()) })
		}
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}})
}

func mustRead(p string) string {
	b, _ := os.ReadFile(p)
	return "\n" + string(b)
}
