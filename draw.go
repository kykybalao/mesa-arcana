package main

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

const ratio = 1.395

// uiS é a escala da tela (monitores com zoom de 125%, 150%...); as primitivas multiplicam tudo por ela
var uiS = 1.0

var (
	cBg     = color.RGBA{0x12, 0x18, 0x1c, 0xff}
	cPanel  = color.RGBA{0x1b, 0x24, 0x2a, 0xff}
	cPanel2 = color.RGBA{0x23, 0x2f, 0x36, 0xff}
	cLine   = color.RGBA{0x34, 0x44, 0x4d, 0xff}
	cTxt    = color.RGBA{0xe7, 0xec, 0xee, 0xff}
	cMut    = color.RGBA{0x93, 0xa4, 0xad, 0xff}
	cAcc    = color.RGBA{0xe0, 0xb3, 0x4a, 0xff}
	cAcc2   = color.RGBA{0x5c, 0xc2, 0xa6, 0xff}
	cBad    = color.RGBA{0xe2, 0x68, 0x5f, 0xff}
	cFelt   = color.RGBA{0x19, 0x3b, 0x33, 0xff}
	cDark   = color.RGBA{0x0e, 0x13, 0x16, 0xff}
	cInk    = color.RGBA{0x1d, 0x1b, 0x18, 0xff}
	cPaper  = color.RGBA{0xe8, 0xe1, 0xd3, 0xff}
	cPaperT = color.RGBA{0xdf, 0xe6, 0xdc, 0xff}
	cRule   = color.RGBA{0x9c, 0x93, 0x82, 0xff}
)

var fReg, fBold, fMono *text.GoTextFaceSource

func initFonts() {
	fReg, _ = text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	fBold, _ = text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF))
	fMono, _ = text.NewGoTextFaceSource(bytes.NewReader(gomono.TTF))
}

func tw(s string, src *text.GoTextFaceSource, size float64) float64 {
	return text.Advance(s, &text.GoTextFace{Source: src, Size: size})
}

// txt desenha texto com o canto superior esquerdo em (x, y)
func txt(dst *ebiten.Image, s string, x, y float64, src *text.GoTextFaceSource, size float64, col color.Color) {
	if s == "" {
		return
	}
	op := &text.DrawOptions{}
	op.GeoM.Translate(math.Round(x*uiS), math.Round(y*uiS))
	op.ColorScale.ScaleWithColor(col)
	op.LineSpacing = size * 1.25 * uiS
	text.Draw(dst, s, &text.GoTextFace{Source: src, Size: size * uiS}, op)
}
func txtC(dst *ebiten.Image, s string, cx, y float64, src *text.GoTextFaceSource, size float64, col color.Color) {
	txt(dst, s, cx-tw(s, src, size)/2, y, src, size, col)
}

type wrapKey struct {
	s    string
	src  *text.GoTextFaceSource
	size float64
	w    float64
}

var wrapCache = map[wrapKey][]string{}

func wrap(s string, src *text.GoTextFaceSource, size, width float64) []string {
	k := wrapKey{s, src, size, width}
	if v, ok := wrapCache[k]; ok {
		return v
	}
	if len(wrapCache) > 4000 {
		wrapCache = map[wrapKey][]string{}
	}
	var out []string
	f := &text.GoTextFace{Source: src, Size: size}
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, w := range strings.Fields(para) {
			try := w
			if line != "" {
				try = line + " " + w
			}
			if line != "" && text.Advance(try, f) > width {
				out = append(out, line)
				line = w
			} else {
				line = try
			}
		}
		out = append(out, line)
	}
	wrapCache[k] = out
	return out
}

// para caber em uma linha só
func fit(s string, src *text.GoTextFaceSource, size, width float64) string {
	if tw(s, src, size) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 1 && tw(string(r)+"…", src, size) > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func rpath(x, y, w, h, r float32) *vector.Path {
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	p := &vector.Path{}
	p.MoveTo(x+r, y)
	p.LineTo(x+w-r, y)
	p.ArcTo(x+w, y, x+w, y+r, r)
	p.LineTo(x+w, y+h-r)
	p.ArcTo(x+w, y+h, x+w-r, y+h, r)
	p.LineTo(x+r, y+h)
	p.ArcTo(x, y+h, x, y+h-r, r)
	p.LineTo(x, y+r)
	p.ArcTo(x, y, x+r, y, r)
	p.Close()
	return p
}
func rrect(dst *ebiten.Image, x, y, w, h, r float64, col color.Color) {
	x, y, w, h, r = x*uiS, y*uiS, w*uiS, h*uiS, r*uiS
	if r <= 0 {
		vector.FillRect(dst, float32(x), float32(y), float32(w), float32(h), col, false)
		return
	}
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(col)
	vector.FillPath(dst, rpath(float32(x), float32(y), float32(w), float32(h), float32(r)), nil, op)
}
func rstroke(dst *ebiten.Image, x, y, w, h, r, sw float64, col color.Color) {
	x, y, w, h, r, sw = x*uiS, y*uiS, w*uiS, h*uiS, r*uiS, sw*uiS
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(col)
	vector.StrokePath(dst, rpath(float32(x), float32(y), float32(w), float32(h), float32(r)), &vector.StrokeOptions{Width: float32(sw)}, op)
}
func rect(dst *ebiten.Image, x, y, w, h float64, col color.Color) {
	x, y, w, h = x*uiS, y*uiS, w*uiS, h*uiS
	vector.FillRect(dst, float32(x), float32(y), float32(w), float32(h), col, false)
}

const pw, ph = 330.0, 460.0 // tamanho em que os proxies são desenhados

// proxy desenha a carta no estilo de proxy de playtest: nome, custo, tipo, texto e P/R
func (g *Game) proxy(c *Card, fi int) *ebiten.Image {
	key := c.ID + ":" + string(rune('0'+fi)) + cfg.Lang
	if im := g.proxies[key]; im != nil {
		return im
	}
	f, _ := loc(c, fi)
	old := uiS
	uiS = 1 // o proxy é sempre desenhado no tamanho fixo e depois redimensionado
	defer func() { uiS = old }()
	im := ebiten.NewImage(int(pw), int(ph))
	bg := cPaper
	if (c.Custom && !c.Mine) || c.TK != 0 || strings.Contains(f.T, "Token") {
		bg = cPaperT
	}
	rrect(im, 0, 0, pw, ph, 16, color.RGBA{0x7d, 0x75, 0x66, 0xff})
	rrect(im, 1.5, 1.5, pw-3, ph-3, 15, bg)
	const m = 20.0
	y := 18.0
	costW := 0.0
	if f.C != "" {
		costW = tw(f.C, fMono, 17) + 10
		if costW > pw*0.45 {
			costW = 0 // custo comprido demais vai para a linha de baixo
		}
	}
	for _, l := range wrap(f.N, fBold, 25, pw-2*m-costW) {
		txt(im, l, m, y, fBold, 25, cInk)
		y += 29
	}
	if f.C != "" {
		if costW > 0 {
			txt(im, f.C, pw-m-costW+10, 22, fMono, 17, color.RGBA{0x3a, 0x36, 0x30, 0xff})
		} else {
			txt(im, f.C, m, y, fMono, 15, color.RGBA{0x3a, 0x36, 0x30, 0xff})
			y += 22
		}
	}
	y += 12
	rect(im, m, y, pw-2*m, 1.5, cRule)
	y += 12
	for _, l := range wrap(f.T, fBold, 16.5, pw-2*m) {
		txt(im, l, m, y, fBold, 16.5, cInk)
		y += 20
	}
	y += 10
	rect(im, m, y, pw-2*m, 1.5, cRule)
	y += 16
	pt := ""
	if f.P != "" || f.O != "" {
		pt = f.P + "/" + f.O
	} else if f.L != "" {
		pt = f.L
	}
	bottom := ph - 18.0
	if pt != "" {
		bottom = ph - 62
	}
	size := 19.0
	var lines []string
	for ; size >= 9.5; size -= 1 {
		lines = wrap(f.X, fReg, size, pw-2*m)
		if float64(len(lines))*size*1.24 <= bottom-y {
			break
		}
	}
	for _, l := range lines {
		if y+size > bottom+4 {
			break
		}
		txt(im, l, m, y, fReg, size, cInk)
		y += size * 1.24
	}
	if pt != "" {
		w := tw(pt, fBold, 23) + 30
		rrect(im, pw-m-w, ph-56, w, 38, 12, color.RGBA{0x6f, 0x68, 0x5c, 0xff})
		rrect(im, pw-m-w+1.5, ph-54.5, w-3, 35, 11, color.RGBA{0xef, 0xe9, 0xdd, 0xff})
		txtC(im, pt, pw-m-w/2, ph-51, fBold, 23, cInk)
	}
	g.proxies[key] = im
	return im
}

func (g *Game) backTex() *ebiten.Image {
	if g.back == nil {
		g.back = g.sleeveTex()
	}
	if g.back != nil {
		return g.back
	}
	old := uiS
	uiS = 1
	defer func() { uiS = old }()
	im := ebiten.NewImage(int(pw), int(ph))
	rrect(im, 0, 0, pw, ph, 16, color.RGBA{0x0b, 0x0f, 0x12, 0xff})
	rrect(im, 10, 10, pw-20, ph-20, 10, color.RGBA{0x22, 0x30, 0x38, 0xff})
	rstroke(im, 22, 22, pw-44, ph-44, 8, 2, color.RGBA{0x34, 0x48, 0x52, 0xff})
	cx, cy, d := float32(pw/2), float32(ph/2), float32(78)
	p := &vector.Path{}
	p.MoveTo(cx, cy-d)
	p.LineTo(cx+d, cy)
	p.LineTo(cx, cy+d)
	p.LineTo(cx-d, cy)
	p.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(cDark)
	vector.FillPath(im, p, nil, op)
	op2 := &vector.DrawPathOptions{AntiAlias: true}
	op2.ColorScale.ScaleWithColor(cAcc)
	vector.StrokePath(im, p, &vector.StrokeOptions{Width: 6, LineJoin: vector.LineJoinRound}, op2)
	vector.FillCircle(im, cx, cy, 30, cAcc2, true)
	g.back = im
	return im
}

// cardTex escolhe a imagem real (se já estiver no pendrive) ou o proxy de texto
func (g *Game) cardTex(c *Card, fi int) *ebiten.Image {
	if !c.Custom || c.Mine {
		if im := photo(c.ID + ":" + string(rune('0'+fi))); im != nil {
			return im
		}
	}
	return g.proxy(c, fi)
}

func drawTex(dst, tex *ebiten.Image, x, y, w float64, tapped bool, alpha float32) {
	x, y, w = x*uiS, y*uiS, w*uiS
	b := tex.Bounds()
	s := w / float64(b.Dx())
	h := w * ratio
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Translate(-float64(b.Dx())/2, -float64(b.Dy())/2)
	op.GeoM.Scale(s, h/float64(b.Dy()))
	if tapped {
		op.GeoM.Rotate(math.Pi / 2)
	}
	op.GeoM.Translate(x+w/2, y+h/2)
	if alpha < 1 {
		op.ColorScale.ScaleAlpha(alpha)
	}
	dst.DrawImage(tex, op)
}

// clip devolve a parte da tela onde se pode desenhar (em coordenadas lógicas)
func clip(dst *ebiten.Image, x, y, w, h float64) *ebiten.Image {
	return dst.SubImage(image.Rect(int(x*uiS), int(y*uiS), int((x+w)*uiS), int((y+h)*uiS))).(*ebiten.Image)
}
