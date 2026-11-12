package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"math"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	xdraw "golang.org/x/image/draw"
)

// Cartas desenhadas no formato de carta de Magic (barra de nome, arte, linha de tipo, caixa de texto e P/R).
// Tudo aqui é desenhado pelo programa: moldura, texturas e símbolos de mana são próprios.
// Estilos: "moldura" (clássica), "estendida" (arte até a borda dos lados) e "cena" (arte na carta inteira, texto por cima).

const cdW, cdH = 744.0, 1039.0 // 63 x 88 mm a 300 dpi

//go:embed fontes/Caladea-Regular.ttf
var ttfSerif []byte

//go:embed fontes/Caladea-Bold.ttf
var ttfSerifB []byte

//go:embed fontes/Caladea-Italic.ttf
var ttfSerifI []byte

var fSerif, fSerifB, fSerifI *text.GoTextFaceSource

func initSerif() {
	if fSerif == nil {
		fSerif, _ = text.NewGoTextFaceSource(bytes.NewReader(ttfSerif))
		fSerifB, _ = text.NewGoTextFaceSource(bytes.NewReader(ttfSerifB))
		fSerifI, _ = text.NewGoTextFaceSource(bytes.NewReader(ttfSerifI))
	}
}

var styleNames = [][2]string{{"", "Playtest"}, {"moldura", "Moldura"}, {"estendida", "Arte estendida"}, {"cena", "Arte inteira"}}

func styleLabel(k string) string {
	for _, s := range styleNames {
		if s[0] == k {
			return s[1]
		}
	}
	return "Playtest"
}
func nextStyle(k string) string {
	for i, s := range styleNames {
		if s[0] == k {
			return styleNames[(i+1)%len(styleNames)][0]
		}
	}
	return ""
}

var frameNames = [][2]string{{"", "Automática"}, {"W", "Branca"}, {"U", "Azul"}, {"B", "Preta"}, {"R", "Vermelha"}, {"G", "Verde"}, {"M", "Dourada"}, {"A", "Artefato"}, {"L", "Terreno"}}

func frameLabel(k string) string {
	for _, s := range frameNames {
		if s[0] == k {
			return s[1]
		}
	}
	return "Automática"
}
func nextFrame(k string) string {
	for i, s := range frameNames {
		if s[0] == k {
			return frameNames[(i+1)%len(frameNames)][0]
		}
	}
	return ""
}

type pal struct{ frame, bar, box color.RGBA }

var pals = map[byte]pal{
	'W': {color.RGBA{0xdb, 0xd4, 0xbd, 0xff}, color.RGBA{0xf5, 0xf1, 0xe4, 0xff}, color.RGBA{0xf8, 0xf5, 0xeb, 0xff}},
	'U': {color.RGBA{0x2c, 0x7b, 0xb6, 0xff}, color.RGBA{0xc3, 0xdb, 0xec, 0xff}, color.RGBA{0xdc, 0xe9, 0xf2, 0xff}},
	'B': {color.RGBA{0x3d, 0x39, 0x3a, 0xff}, color.RGBA{0xc9, 0xc3, 0xbf, 0xff}, color.RGBA{0xdd, 0xd8, 0xd3, 0xff}},
	'R': {color.RGBA{0xc6, 0x48, 0x30, 0xff}, color.RGBA{0xf0, 0xc8, 0xb4, 0xff}, color.RGBA{0xf4, 0xdf, 0xd3, 0xff}},
	'G': {color.RGBA{0x3b, 0x7e, 0x4f, 0xff}, color.RGBA{0xc6, 0xdc, 0xc5, 0xff}, color.RGBA{0xde, 0xe9, 0xd9, 0xff}},
	'M': {color.RGBA{0xc8, 0xa7, 0x4b, 0xff}, color.RGBA{0xf1, 0xe3, 0xad, 0xff}, color.RGBA{0xf6, 0xed, 0xcd, 0xff}},
	'A': {color.RGBA{0x98, 0xa5, 0xae, 0xff}, color.RGBA{0xdf, 0xe5, 0xe9, 0xff}, color.RGBA{0xe9, 0xed, 0xf0, 0xff}},
	'L': {color.RGBA{0xa4, 0x8d, 0x77, 0xff}, color.RGBA{0xe4, 0xd7, 0xc9, 0xff}, color.RGBA{0xed, 0xe4, 0xd9, 0xff}},
}

func costSyms(cost string) []string {
	cost, _, _ = strings.Cut(cost, " // ")
	var out []string
	for _, m := range symRe.FindAllStringSubmatch(strings.ToUpper(cost), -1) {
		out = append(out, m[1])
	}
	return out
}

// frameKind: uma letra (cor única, M dourada, A artefato/incolor, L terreno) ou duas (moldura em degradê)
func frameKind(cost, typ, force string) string {
	if force != "" {
		return force
	}
	seen := ""
	for _, s := range costSyms(cost) {
		for _, r := range s {
			if strings.ContainsRune("WUBRG", r) && !strings.ContainsRune(seen, r) {
				seen += string(r)
			}
		}
	}
	switch {
	case len(seen) == 1 || len(seen) == 2:
		return seen
	case len(seen) > 2:
		return "M"
	}
	t := strings.ToLower(typ)
	if strings.Contains(t, "land") || strings.Contains(t, "terreno") {
		return "L"
	}
	return "A"
}

func lerpC(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 0xff}
}

var frameTexs = map[string]*ebiten.Image{}

// frameTex: textura manchada da moldura (meia resolução, esticada na hora de desenhar)
func frameTex(kind string) *ebiten.Image {
	if t := frameTexs[kind]; t != nil {
		return t
	}
	const w, h = 372, 520
	a := pals[kind[0]].frame
	b := a
	if len(kind) > 1 {
		b = pals[kind[1]].frame
	}
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(kind[0]) * 31
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := (float64(x)/w - 0.3) / 0.4
			t = max(0, min(1, t))
			t = t * t * (3 - 2*t)
			c := lerpC(a, b, t)
			n := fbm(float64(x)/34, float64(y)/34, seed)
			sh := 0.80 + 0.40*n + (hash2(x, y, seed+9)-0.5)*0.06
			sh *= 1 - 0.10*float64(y)/h
			o := y*im.Stride + x*4
			im.Pix[o] = uint8(min(255, float64(c.R)*sh))
			im.Pix[o+1] = uint8(min(255, float64(c.G)*sh))
			im.Pix[o+2] = uint8(min(255, float64(c.B)*sh))
			im.Pix[o+3] = 255
		}
	}
	t := ebiten.NewImageFromImage(im)
	frameTexs[kind] = t
	return t
}

func kindPal(kind string) pal {
	if len(kind) > 1 {
		return pal{pals['M'].frame, pals['M'].bar, pals['M'].box}
	}
	return pals[kind[0]]
}

/* ---------------- arte ---------------- */

type artSlot struct {
	mod int64
	tex *ebiten.Image
}

var artCache = map[string]*artSlot{}

// artTex abre a arte de uma carta (reduzida) e guarda; devolve nil se o arquivo não existe
func artTex(path string) *ebiten.Image {
	st, err := os.Stat(path)
	if err != nil || st.Size() < 100 {
		return nil
	}
	if s := artCache[path]; s != nil && s.mod == st.ModTime().UnixNano() {
		return s.tex
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	b := src.Bounds()
	if b.Dx() < 2 || b.Dy() < 2 {
		return nil
	}
	k := min(1, 1500/float64(max(b.Dx(), b.Dy())))
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*k)), max(1, int(float64(b.Dy())*k))))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	if len(artCache) > 60 {
		for k, s := range artCache {
			s.tex.Deallocate()
			delete(artCache, k)
		}
	}
	s := &artSlot{st.ModTime().UnixNano(), ebiten.NewImageFromImage(dst)}
	artCache[path] = s
	return s.tex
}

// cover desenha a imagem preenchendo o retângulo inteiro (corta o que sobra)
// adj = zoom (0 ou 1 = justo), e posição horizontal e vertical de -1 a 1 (0 = centro)
func cover(dst, tex *ebiten.Image, x, y, w, h float64, adj [3]float64) {
	x, y, w, h = x*uiS, y*uiS, w*uiS, h*uiS
	b := tex.Bounds()
	s := max(w/float64(b.Dx()), h/float64(b.Dy()))
	if adj[0] > 0 {
		s *= adj[0]
	}
	sub := dst.SubImage(image.Rect(int(x), int(y), int(x+w), int(y+h))).(*ebiten.Image)
	sub.Fill(color.RGBA{0x0c, 0x0b, 0x0a, 0xff})
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(x+(w-float64(b.Dx())*s)*(0.5+adj[1]/2), y+(h-float64(b.Dy())*s)*(0.5+adj[2]/2))
	sub.DrawImage(tex, op)
}

func stretch(dst, tex *ebiten.Image, x, y, w, h float64, dim float32) {
	x, y, w, h = x*uiS, y*uiS, w*uiS, h*uiS
	b := tex.Bounds()
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(w/float64(b.Dx()), h/float64(b.Dy()))
	op.GeoM.Translate(x, y)
	op.ColorScale.Scale(dim, dim, dim, 1)
	dst.DrawImage(tex, op)
}

// vgrad: degradê vertical de preto transparente (a0 em cima, a1 embaixo)
func vgrad(dst *ebiten.Image, x, y, w, h, a0, a1 float64) {
	y0, y1 := int(math.Round(y*uiS)), int(math.Round((y+h)*uiS))
	for py := y0; py < y1; py++ {
		a := a0 + (a1-a0)*float64(py-y0)/float64(max(1, y1-y0-1))
		vector.FillRect(dst, float32(x*uiS), float32(py), float32(w*uiS), 1, color.NRGBA{0, 0, 0, uint8(255 * a)}, false)
	}
}

/* ---------------- símbolos de mana (desenho próprio) ---------------- */

var symBg = map[byte]color.RGBA{
	'W': {0xf6, 0xf0, 0xcf, 0xff}, 'U': {0xa8, 0xd6, 0xf0, 0xff}, 'B': {0xbd, 0xb0, 0xab, 0xff},
	'R': {0xf2, 0xa5, 0x8a, 0xff}, 'G': {0x9e, 0xd2, 0xa9, 0xff}, 'N': {0xcf, 0xc9, 0xc3, 0xff},
}
var symInk = color.RGBA{0x17, 0x14, 0x11, 0xff}

func fillP(dst *ebiten.Image, p *vector.Path, col color.Color) {
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(col)
	vector.FillPath(dst, p, nil, op)
}
func strokeP(dst *ebiten.Image, p *vector.Path, w float32, col color.Color) {
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(col)
	vector.StrokePath(dst, p, &vector.StrokeOptions{Width: w, LineCap: vector.LineCapRound, LineJoin: vector.LineJoinRound}, op)
}
func circ(dst *ebiten.Image, cx, cy, r float64, col color.Color) {
	vector.FillCircle(dst, float32(cx*uiS), float32(cy*uiS), float32(r*uiS), col, true)
}

// picto desenha o desenho de uma cor (sol, gota, caveira, chama, árvore) centrado em (cx, cy) com tamanho d
func picto(dst *ebiten.Image, k byte, cx, cy, d float64, bg color.Color) {
	S := uiS
	P := func(dx, dy float64) (float32, float32) { return float32((cx + dx*d) * S), float32((cy + dy*d) * S) }
	p := &vector.Path{}
	mv := func(dx, dy float64) { x, y := P(dx, dy); p.MoveTo(x, y) }
	cu := func(a, b, c, e, f, g float64) {
		x1, y1 := P(a, b)
		x2, y2 := P(c, e)
		x3, y3 := P(f, g)
		p.CubicTo(x1, y1, x2, y2, x3, y3)
	}
	switch k {
	case 'W':
		circ(dst, cx, cy, d*0.17, symInk)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			q := &vector.Path{}
			x1, y1 := P(math.Cos(a-0.2)*0.23, math.Sin(a-0.2)*0.23)
			x2, y2 := P(math.Cos(a)*0.40, math.Sin(a)*0.40)
			x3, y3 := P(math.Cos(a+0.2)*0.23, math.Sin(a+0.2)*0.23)
			q.MoveTo(x1, y1)
			q.LineTo(x2, y2)
			q.LineTo(x3, y3)
			q.Close()
			fillP(dst, q, symInk)
		}
	case 'U':
		mv(0, -0.38)
		cu(0.08, -0.18, 0.25, -0.02, 0.25, 0.13)
		cu(0.25, 0.44, -0.25, 0.44, -0.25, 0.13)
		cu(-0.25, -0.02, -0.08, -0.18, 0, -0.38)
		p.Close()
		fillP(dst, p, symInk)
	case 'B':
		circ(dst, cx, cy-d*0.07, d*0.26, symInk)
		x, y := P(-0.14, 0.05)
		vector.FillRect(dst, x, y, float32(0.28*d*S), float32(0.27*d*S), symInk, true)
		circ(dst, cx-d*0.11, cy-d*0.05, d*0.075, bg)
		circ(dst, cx+d*0.11, cy-d*0.05, d*0.075, bg)
		for _, dx := range []float64{-0.05, 0.05} {
			x, y := P(dx-0.012, 0.2)
			vector.FillRect(dst, x, y, float32(0.024*d*S), float32(0.13*d*S), bg, true)
		}
	case 'R':
		mv(0.02, -0.40)
		cu(0.06, -0.16, 0.30, -0.06, 0.25, 0.18)
		cu(0.21, 0.40, -0.21, 0.40, -0.25, 0.18)
		cu(-0.29, 0.0, -0.13, -0.02, -0.15, -0.22)
		cu(-0.06, -0.12, -0.01, -0.24, 0.02, -0.40)
		p.Close()
		fillP(dst, p, symInk)
		q := &vector.Path{}
		x0, y0 := P(0, 0.02)
		q.MoveTo(x0, y0)
		x1, y1 := P(0.12, 0.14)
		x2, y2 := P(0.10, 0.30)
		x3, y3 := P(0, 0.30)
		q.CubicTo(x1, y1, x2, y2, x3, y3)
		x1, y1 = P(-0.10, 0.30)
		x2, y2 = P(-0.12, 0.14)
		q.CubicTo(x1, y1, x2, y2, x0, y0)
		q.Close()
		fillP(dst, q, bg)
	case 'G':
		circ(dst, cx, cy-d*0.16, d*0.19, symInk)
		circ(dst, cx-d*0.16, cy-d*0.01, d*0.16, symInk)
		circ(dst, cx+d*0.16, cy-d*0.01, d*0.16, symInk)
		x, y := P(-0.05, 0)
		vector.FillRect(dst, x, y, float32(0.10*d*S), float32(0.36*d*S), symInk, true)
		x, y = P(-0.14, 0.31)
		vector.FillRect(dst, x, y, float32(0.28*d*S), float32(0.06*d*S), symInk, true)
	case 'C':
		mv(0, -0.34)
		x, y := P(0.22, 0)
		p.LineTo(x, y)
		x, y = P(0, 0.34)
		p.LineTo(x, y)
		x, y = P(-0.22, 0)
		p.LineTo(x, y)
		p.Close()
		strokeP(dst, p, float32(0.07*d*S), symInk)
	case 'T':
		x, y := P(0, 0.02)
		p.Arc(x, y, float32(0.24*d*S), float32(-0.2*math.Pi), float32(1.25*math.Pi), vector.Clockwise)
		strokeP(dst, p, float32(0.10*d*S), symInk)
		q := &vector.Path{}
		x1, y1 := P(0.06, -0.30)
		x2, y2 := P(0.36, -0.22)
		x3, y3 := P(0.14, 0.0)
		q.MoveTo(x1, y1)
		q.LineTo(x2, y2)
		q.LineTo(x3, y3)
		q.Close()
		fillP(dst, q, symInk)
	}
}

// drawSym desenha um símbolo {…} com o canto em (x, y) e diâmetro d
func drawSym(dst *ebiten.Image, s string, x, y, d float64, shadow bool) {
	initSerif()
	cx, cy, r := x+d/2, y+d/2, d/2
	if shadow {
		circ(dst, cx-d*0.03, cy+d*0.07, r, color.RGBA{0x10, 0x0e, 0x0c, 0xff})
	}
	isCol := func(b byte) bool { return strings.IndexByte("WUBRG", b) >= 0 }
	a, b2, hyb := s, "", false
	if i := strings.IndexByte(s, '/'); i > 0 {
		a, b2, hyb = s[:i], s[i+1:], true
	}
	bgOf := func(t string) color.RGBA {
		if len(t) == 1 && isCol(t[0]) {
			return symBg[t[0]]
		}
		return symBg['N']
	}
	label := func(t string, cx, cy, d float64) {
		size := d * 0.74
		if len(t) > 1 {
			size = d * 0.56
		}
		txtC(dst, t, cx, cy-size*0.62, fSerifB, size, symInk)
	}
	if hyb && b2 != "P" {
		circ(dst, cx, cy, r, bgOf(a))
		p := &vector.Path{}
		p.Arc(float32(cx*uiS), float32(cy*uiS), float32(r*uiS), float32(-math.Pi/4), float32(3*math.Pi/4), vector.Clockwise)
		p.Close()
		fillP(dst, p, bgOf(b2))
		for i, t := range []string{a, b2} {
			o := -0.17
			if i == 1 {
				o = 0.17
			}
			if len(t) == 1 && isCol(t[0]) {
				picto(dst, t[0], cx+o*d, cy+o*d, d*0.52, bgOf(t))
			} else {
				label(t, cx+o*d, cy+o*d, d*0.52)
			}
		}
		return
	}
	circ(dst, cx, cy, r, bgOf(a))
	switch {
	case hyb: // mana phyrexiana
		label("Φ", cx, cy, d)
	case len(a) == 1 && (isCol(a[0]) || a[0] == 'C' || a[0] == 'T'):
		picto(dst, a[0], cx, cy, d, bgOf(a))
	case a == "Q":
		label("↶", cx, cy, d)
	default:
		label(a, cx, cy, d)
	}
}

/* ---------------- texto com símbolos ---------------- */

type rpiece struct {
	s   string
	sym bool
	it  bool
	x   float64
}
type rline struct {
	ps  []rpiece
	gap bool // começa um parágrafo novo
}

func richLayout(s string, size, width float64) []rline {
	initSerif()
	var out []rline
	symW := size * 0.96
	sp := tw(" ", fSerif, size)
	for pi, para := range strings.Split(s, "\n") {
		it := false
		line := rline{gap: pi > 0}
		cx := 0.0
		for _, word := range strings.Fields(para) {
			if strings.HasPrefix(word, "(") {
				it = true
			}
			var ps []rpiece
			ww := 0.0
			rest := word
			for rest != "" {
				loc := symRe.FindStringIndex(rest)
				if loc == nil {
					loc = []int{len(rest), len(rest)}
				}
				if loc[0] > 0 {
					src := fSerif
					if it {
						src = fSerifI
					}
					ps = append(ps, rpiece{s: rest[:loc[0]], it: it, x: ww})
					ww += tw(rest[:loc[0]], src, size)
				}
				if loc[1] > loc[0] {
					ps = append(ps, rpiece{s: strings.ToUpper(rest[loc[0]+1 : loc[1]-1]), sym: true, x: ww})
					ww += symW
				}
				rest = rest[loc[1]:]
			}
			if cx > 0 && cx+sp+ww > width {
				out = append(out, line)
				line, cx = rline{}, 0
			}
			if cx > 0 {
				cx += sp
			}
			for _, p := range ps {
				p.x += cx
				line.ps = append(line.ps, p)
			}
			cx += ww
			if strings.Contains(word, ")") {
				it = false
			}
		}
		out = append(out, line)
	}
	return out
}

func richHeight(ls []rline, size float64) float64 {
	h := 0.0
	for _, l := range ls {
		h += size * 1.17
		if l.gap {
			h += size * 0.38
		}
	}
	return h
}

// richFit escolhe o maior tamanho de letra que cabe na altura
func richFit(s string, width, height, maxSize float64) ([]rline, float64) {
	size := maxSize
	for ; size > 13; size -= 1 {
		ls := richLayout(s, size, width)
		if richHeight(ls, size) <= height {
			return ls, size
		}
	}
	return richLayout(s, size, width), size
}

// otxt: texto com contorno escuro, para ficar legível em cima da arte
func otxt(dst *ebiten.Image, s string, x, y float64, src *text.GoTextFaceSource, size float64, col color.Color) {
	o := max(1.6, size*0.07)
	dark := color.RGBA{0, 0, 0, 0xff}
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		txt(dst, s, x+math.Cos(a)*o, y+math.Sin(a)*o, src, size, dark)
	}
	txt(dst, s, x, y+o*1.4, src, size, dark)
	txt(dst, s, x, y, src, size, col)
}

func richDraw(dst *ebiten.Image, ls []rline, size, x, y float64, col color.Color, outline bool) {
	for _, l := range ls {
		if l.gap {
			y += size * 0.38
		}
		for _, p := range l.ps {
			if p.sym {
				if outline {
					circ(dst, x+p.x+size*0.44, y+size*0.66, size*0.44+max(1.6, size*0.07), color.RGBA{0, 0, 0, 0xff})
				}
				drawSym(dst, p.s, x+p.x, y+size*0.22, size*0.88, false)
				continue
			}
			src := fSerif
			if p.it {
				src = fSerifI
			}
			if outline {
				otxt(dst, p.s, x+p.x, y, src, size, col)
			} else {
				txt(dst, p.s, x+p.x, y, src, size, col)
			}
		}
		y += size * 1.17
	}
}

// fitSize: maior tamanho (até maxSize) em que o texto cabe na largura
func fitSize(s string, src *text.GoTextFaceSource, maxSize, width float64) float64 {
	size := maxSize
	for size > 14 && tw(s, src, size) > width {
		size -= 1
	}
	return size
}

/* ---------------- a carta ---------------- */

type cardSpec struct {
	f     Face
	style string // moldura, estendida, cena
	sub   string // segunda linha de nome (só no estilo cena)
	kind  string
	art   *ebiten.Image
	adj   [3]float64
	mark  string // símbolo: licao, sol, lua, modal1, modal2, fusao
	hint  string // linha pequena no pé do texto (outro lado, fusão)
	tall  bool
	token bool
}

// drawCost desenha o custo alinhado à direita em xr e devolve a largura usada
func drawCost(dst *ebiten.Image, cost string, xr, y, d float64, shadow bool) float64 {
	ss := costSyms(cost)
	step := d * 1.1
	w := float64(len(ss)) * step
	for i, s := range ss {
		drawSym(dst, s, xr-w+float64(i)*step, y, d, shadow)
	}
	return w
}

func bar(dst *ebiten.Image, x, y, w, h float64, p pal) {
	rrect(dst, x-3, y-3, w+6, h+6, 15, color.RGBA{0x14, 0x12, 0x10, 0xff})
	rrect(dst, x, y, w, h, 12, p.bar)
	rrect(dst, x+4, y+3, w-8, h*0.42, 9, color.NRGBA{255, 255, 255, 70})
}

const footNote = "PROXY · não é carta oficial"

var markNames = [][2]string{{"", "Nenhum"}, {"licao", "Lição"}, {"sol", "Frente (transforma)"}, {"lua", "Verso (transformada)"}, {"modal1", "Frente (dupla)"}, {"modal2", "Verso (dupla)"}, {"fusao", "Fusão"}}

func markLabel(k string) string {
	for _, s := range markNames {
		if s[0] == k {
			return s[1]
		}
	}
	return "Nenhum"
}
func nextMark(k string) string {
	for i, s := range markNames {
		if s[0] == k {
			return markNames[(i+1)%len(markNames)][0]
		}
	}
	return ""
}

// drawMark desenha o símbolo do tipo de carta (desenho próprio) dentro de um disco centrado em (cx, cy)
func drawMark(dst *ebiten.Image, mark string, cx, cy, d float64, bg, ink color.Color) {
	S := uiS
	circ(dst, cx, cy, d/2+2.5, color.RGBA{0x12, 0x10, 0x0e, 0xff})
	circ(dst, cx, cy, d/2, bg)
	P := func(dx, dy float64) (float32, float32) { return float32((cx + dx*d) * S), float32((cy + dy*d) * S) }
	poly := func(pts ...float64) {
		p := &vector.Path{}
		for i := 0; i < len(pts); i += 2 {
			x, y := P(pts[i], pts[i+1])
			if i == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		p.Close()
		fillP(dst, p, ink)
	}
	switch mark {
	case "sol":
		circ(dst, cx, cy, d*0.15, ink)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			poly(math.Cos(a-0.22)*0.21, math.Sin(a-0.22)*0.21, math.Cos(a)*0.38, math.Sin(a)*0.38, math.Cos(a+0.22)*0.21, math.Sin(a+0.22)*0.21)
		}
	case "lua":
		circ(dst, cx-d*0.03, cy, d*0.30, ink)
		circ(dst, cx+d*0.10, cy-d*0.07, d*0.25, bg)
	case "modal1":
		poly(0, -0.28, 0.27, 0.2, -0.27, 0.2)
	case "modal2":
		poly(-0.17, -0.22, 0.03, 0.2, -0.37, 0.2)
		poly(0.17, -0.22, 0.37, 0.2, -0.03, 0.2)
	case "fusao":
		for _, o := range []float64{-0.11, 0.11} {
			p := &vector.Path{}
			x, y := P(o, 0)
			p.Arc(x, y, float32(0.17*d*S), 0, 2*math.Pi, vector.Clockwise)
			p.Close()
			strokeP(dst, p, float32(0.07*d*S), ink)
		}
	case "licao": // livro aberto
		poly(-0.36, -0.20, -0.03, -0.11, -0.03, 0.27, -0.36, 0.18)
		poly(0.36, -0.20, 0.03, -0.11, 0.03, 0.27, 0.36, 0.18)
		for _, yy := range []float64{-0.04, 0.07} {
			for _, sx := range []float64{-1, 1} {
				p := &vector.Path{}
				x1, y1 := P(sx*0.29, yy)
				x2, y2 := P(sx*0.10, yy+0.05)
				p.MoveTo(x1, y1)
				p.LineTo(x2, y2)
				strokeP(dst, p, float32(0.035*d*S), bg)
			}
		}
	}
}

// renderCard desenha a carta inteira; scale 1 = 744x1039 (tamanho de impressão).
// Com sp.tall a carta tem o dobro da altura (carta de fusão, para cortar em duas).
func renderCard(sp cardSpec, scale float64) *ebiten.Image {
	initSerif()
	old := uiS
	uiS = scale
	defer func() { uiS = old }()
	off := 0.0
	if sp.tall {
		off = cdH
	}
	fullH := cdH + off
	W, H := int(math.Round(cdW*scale)), int(math.Round(fullH*scale))
	im := ebiten.NewImage(W, H)
	f := sp.f
	p := kindPal(sp.kind)
	ink := color.RGBA{0x15, 0x13, 0x11, 0xff}
	black := color.RGBA{0x12, 0x10, 0x0e, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	pt := ""
	if f.P != "" || f.O != "" {
		pt = f.P + "/" + f.O
	} else if f.L != "" {
		pt = f.L
	}
	topMark, lesson := sp.mark != "" && sp.mark != "licao", sp.mark == "licao"
	nameX := 0.0
	if topMark {
		nameX = 48
	}
	typeW := 0.0
	if lesson {
		typeW = 50
	}
	rect(im, 0, 0, cdW, fullH, black)
	tex := frameTex(sp.kind)
	if sp.style == "cena" {
		if sp.art != nil {
			cover(im, sp.art, 0, 0, cdW, fullH, sp.adj)
		} else {
			stretch(im, tex, 0, 0, cdW, fullH, 0.55)
		}
		vgrad(im, 0, 0, cdW, 190, 0.62, 0)
		// bloco de texto ancorado embaixo: tipo em cima, regras depois
		bottom := 938.0 + off
		if pt != "" || sp.hint != "" {
			bottom = 906 + off
		}
		ls, size := richFit(f.X, 656, 330, 31)
		th := richHeight(ls, size)
		if strings.TrimSpace(f.X) == "" {
			th = 0
		}
		ty := bottom - th
		typeY := ty - 54
		vgrad(im, 0, typeY-200, cdW, 200, 0, 0.56)
		vgrad(im, 0, typeY, cdW, fullH-typeY, 0.56, 0.74)
		cw := drawCost(im, f.C, 704, 44, 40, true)
		if topMark {
			drawMark(im, sp.mark, 64, 62, 42, black, white)
		}
		ns := fitSize(f.N, fSerifB, 42, 656-cw-14-nameX)
		otxt(im, f.N, 44+nameX, 36+(42-ns)*0.5, fSerifB, ns, white)
		if sp.sub != "" {
			ss := fitSize(sp.sub, fSerifI, 27, 656-nameX)
			otxt(im, sp.sub, 46+nameX, 88, fSerifI, ss, white)
		}
		ts := fitSize(f.T, fSerifB, 33, 656-typeW)
		otxt(im, f.T, 44, typeY, fSerifB, ts, white)
		if lesson {
			drawMark(im, sp.mark, 678, typeY+22, 42, black, white)
		}
		if th > 0 {
			richDraw(im, ls, size, 44, ty, white, true)
		}
		ptW := 0.0
		if pt != "" {
			ptW = tw(pt, fSerifB, 48) + 20
			otxt(im, pt, 700-tw(pt, fSerifB, 48), 922+off, fSerifB, 48, white)
		}
		if sp.hint != "" {
			otxt(im, fit(sp.hint, fSerifI, 22, 656-ptW), 44, 936+off, fSerifI, 22, color.RGBA{0xe6, 0xe6, 0xe6, 0xff})
		}
		txt(im, footNote, 44, 992+off, fReg, 15, color.RGBA{0xd8, 0xd8, 0xd8, 0xff})
		txt(im, "Mesa Arcana", 700-tw("Mesa Arcana", fReg, 15), 992+off, fReg, 15, color.RGBA{0xd8, 0xd8, 0xd8, 0xff})
	} else {
		ext := sp.style == "estendida"
		ax, ay, aw, ah := 54.0, 114.0, 636.0, 466.0+off
		if ext {
			// a arte vai até a borda dos lados; a moldura fica só em cima e embaixo
			stretch(im, tex, 28, 28, 688, 84, 1)
			ax, ay, aw, ah = 0, 108, cdW, 482+off
			subY := 590.0 + off
			b := tex.Bounds()
			lower := tex.SubImage(image.Rect(0, b.Dy()*57/100, b.Dx(), b.Dy()*93/100)).(*ebiten.Image)
			stretch(im, lower, 28, subY, 688, 968+off-subY, 1)
		} else {
			stretch(im, tex, 28, 28, 688, 940+off, 1)
		}
		// arte
		if !ext {
			rect(im, ax-3, ay-3, aw+6, ah+6, black)
		}
		if sp.art != nil {
			cover(im, sp.art, ax, ay, aw, ah, sp.adj)
		} else {
			stretch(im, tex, ax, ay, aw, ah, 0.42)
			txtC(im, "sem arte", ax+aw/2, ay+ah/2-14, fSerifI, 26, color.NRGBA{255, 255, 255, 110})
		}
		if ext {
			rect(im, 0, ay-3, cdW, 3, black)
			rect(im, 0, ay+ah, cdW, 3, black)
		}
		// nome e custo
		bar(im, 42, 44, 660, 60, p)
		cw := drawCost(im, f.C, 692, 56, 36, true)
		if topMark {
			drawMark(im, sp.mark, 74, 74, 42, p.box, ink)
		}
		if sp.sub != "" { // nome novo em cima, nome original embaixo
			ns := fitSize(f.N, fSerifB, 31, 626-cw-10-nameX)
			txt(im, f.N, 58+nameX, 43+(31-ns)*0.5, fSerifB, ns, ink)
			ss := fitSize(sp.sub, fSerifI, 20, 626-cw-10-nameX)
			txt(im, sp.sub, 59+nameX, 76, fSerifI, ss, color.RGBA{0x2c, 0x28, 0x24, 0xff})
		} else {
			ns := fitSize(f.N, fSerifB, 39, 626-cw-10-nameX)
			txt(im, f.N, 58+nameX, 49+(39-ns)*0.6, fSerifB, ns, ink)
		}
		// tipo
		bar(im, 42, 588+off, 660, 54, p)
		ts := fitSize(f.T, fSerifB, 32, 628-typeW)
		txt(im, f.T, 58, 594+off+(32-ts)*0.6, fSerifB, ts, ink)
		if lesson {
			drawMark(im, sp.mark, 674, 615+off, 40, p.box, ink)
		}
		// caixa de texto
		bx, by, bw, bh := 54.0, 650.0+off, 636.0, 304.0
		rect(im, bx-3, by-3, bw+6, bh+6, black)
		rect(im, bx, by, bw, bh, p.box)
		stretch(im, frameTex("W"), bx, by, bw, bh, 1) // leve textura de papel
		rect(im, bx, by, bw, bh, color.NRGBA{p.box.R, p.box.G, p.box.B, 205})
		availH := bh - 30
		if pt != "" {
			availH -= 16
		}
		if sp.hint != "" {
			availH -= 24
		}
		ls, size := richFit(f.X, bw-36, availH, 31)
		th := richHeight(ls, size)
		richDraw(im, ls, size, bx+18, by+14+max(0, (availH-th)/2-6), ink, false)
		// P/R ou lealdade
		ptW := 0.0
		if pt != "" {
			w := max(118, tw(pt, fSerifB, 38)+44)
			ptW = w
			rrect(im, 700-w-3, 915+off, w+6, 62, 16, black)
			rrect(im, 700-w, 918+off, w, 56, 13, p.bar)
			rrect(im, 700-w+4, 921+off, w-8, 22, 9, color.NRGBA{255, 255, 255, 70})
			txtC(im, pt, 700-w/2, 921+off, fSerifB, 38, ink)
		}
		if sp.hint != "" {
			txt(im, fit(sp.hint, fSerifI, 21, bw-30-ptW), bx+16, by+bh-34, fSerifI, 21, color.RGBA{0x55, 0x50, 0x4a, 0xff})
		}
		txt(im, footNote, 44, 986+off, fReg, 15, color.RGBA{0xc8, 0xc8, 0xc8, 0xff})
		txt(im, "Mesa Arcana", 44, 1006+off, fReg, 13, color.RGBA{0x90, 0x90, 0x90, 0xff})
	}
	// cantos arredondados
	mask := ebiten.NewImage(W, H)
	rrect(mask, 0, 0, cdW, fullH, 38, color.White)
	op := &ebiten.DrawImageOptions{}
	op.Blend = ebiten.BlendSourceIn
	mask.DrawImage(im, op)
	im.Deallocate()
	return mask
}
