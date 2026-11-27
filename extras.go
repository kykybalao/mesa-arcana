package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	xdraw "golang.org/x/image/draw"
)

/* ====================== cartas em português ====================== */

// dados/pt.json.gz: texto oficial em português (Scryfall), frases oficiais reaproveitadas e traduções próprias.
// "o": 0 oficial · 1 montada com frases oficiais · 2 tradução própria · 3 só nome/tipo (texto segue em inglês)
//
//go:embed dados/pt.json.gz
var ptGz []byte

type PTFace struct {
	N string `json:"n"`
	T string `json:"t"`
	X string `json:"x"`
}
type PTCard struct {
	PTFace
	F []PTFace `json:"f"`
	O int      `json:"o"`
}

var (
	ptOnce   sync.Once
	ptDB     map[string]*PTCard
	ptByName map[string]string // nome em português (minúsculo) -> nome em inglês
)

func loadPT() {
	ptOnce.Do(func() {
		raw := ptGz
		if b, err := os.ReadFile(filepath.Join(baseDir, "dados", "pt.json.gz")); err == nil {
			raw = b
		}
		ptDB = map[string]*PTCard{}
		ptByName = map[string]string{}
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return
		}
		json.NewDecoder(zr).Decode(&ptDB)
		// traduções do próprio usuário: dados/traducoes.json, no mesmo formato
		if b, err := os.ReadFile(filepath.Join(baseDir, "dados", "traducoes.json")); err == nil {
			var mine map[string]*PTCard
			if json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &mine) == nil {
				for k, v := range mine {
					if v != nil {
						v.O = 2
						if old := ptDB[k]; old != nil {
							if v.N == "" {
								v.N = old.N
							}
							if v.T == "" {
								v.T = old.T
							}
						}
						ptDB[k] = v
					}
				}
			}
		}
		for en, p := range ptDB {
			if p.N != "" && !strings.HasPrefix(en, "#") {
				ptByName[strings.ToLower(p.N)] = en
			}
		}
	})
}

func ptOf(c *Card) *PTCard {
	if c == nil || c.Custom {
		return nil
	}
	loadPT()
	if c.TK != 0 {
		return ptDB["#"+c.ID]
	}
	return ptDB[c.N]
}

// loc devolve a face no idioma escolhido e a origem do texto (-1 = sem tradução)
func loc(c *Card, fi int) (Face, int) {
	f := c.FaceAt(fi)
	if cfg.Lang == "en" {
		return f, 0
	}
	p := ptOf(c)
	if p == nil {
		return f, -1
	}
	pf := p.PTFace
	if len(c.F) > 0 {
		if fi < 0 || fi >= len(p.F) {
			return f, -1
		}
		pf = p.F[fi]
	}
	if pf.N != "" {
		f.N = pf.N
	}
	if pf.T != "" {
		f.T = pf.T
	}
	if pf.X != "" {
		f.X = pf.X
	} else if f.X != "" {
		return f, -1
	}
	return f, p.O
}

// locNote explica de onde veio o texto mostrado
func locNote(c *Card, fi int) string {
	if cfg.Lang == "en" || c == nil || c.Custom {
		return ""
	}
	f, o := loc(c, fi)
	en := c.FaceAt(fi)
	s := ""
	if f.N != en.N {
		s = "Nome em inglês: " + en.N
	}
	switch {
	case o == 1 || o == 2:
		s += "\nTradução não oficial."
	case (o == -1 || o == 3) && en.X != "":
		s += "\nAinda sem tradução: texto em inglês."
	}
	return strings.TrimSpace(s)
}

// findPT acha a carta pelo nome em português
func findPT(name string) *Card {
	loadPT()
	if en := ptByName[strings.ToLower(strings.TrimSpace(name))]; en != "" {
		return cardByName[strings.ToLower(en)]
	}
	return nil
}

func langLabel() string {
	if cfg.Lang == "en" {
		return "Cartas em: Inglês"
	}
	return "Cartas em: Português"
}
func (g *Game) toggleLang() {
	if cfg.Lang == "en" {
		cfg.Lang = "pt"
	} else {
		cfg.Lang = "en"
	}
	g.proxies = map[string]*ebiten.Image{}
	saveCfg()
}

/* ====================== playmats e sleeves ====================== */

type look struct {
	key, name string
	tex       *ebiten.Image
	gen       func(u, v float64) [3]float64 // playmat desenhado pelo programa: só é pintado quando é usado
	thumb     *ebiten.Image
	raw       chan *image.RGBA
}

var (
	matsBuilt, sleevesBuilt []*look
	matsFile, sleevesFile   []*look
	lookTex                 = map[string]*ebiten.Image{}
	looksScanned            bool
)

func hash2(x, y int, seed uint32) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263 + seed*2246822519
	h = (h ^ (h >> 13)) * 1274126177
	return float64((h^(h>>16))&0xffff) / 65535
}
func noise(x, y float64, seed uint32) float64 {
	x0, y0 := math.Floor(x), math.Floor(y)
	fx, fy := x-x0, y-y0
	fx, fy = fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	a, b := hash2(int(x0), int(y0), seed), hash2(int(x0)+1, int(y0), seed)
	c, d := hash2(int(x0), int(y0)+1, seed), hash2(int(x0)+1, int(y0)+1, seed)
	return (a*(1-fx)+b*fx)*(1-fy) + (c*(1-fx)+d*fx)*fy
}
func fbm(x, y float64, seed uint32) float64 {
	v, amp, tot := 0.0, 0.5, 0.0
	for o := 0; o < 5; o++ {
		v += amp * noise(x, y, seed+uint32(o)*17)
		tot += amp
		x, y, amp = x*2.03, y*2.03, amp*0.5
	}
	return v / tot
}
func mix(a, b [3]float64, t float64) [3]float64 {
	t = max(0, min(1, t))
	return [3]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

const matW, matH = 1280, 720

// genMat pinta um playmat por pixel; f recebe coordenadas de 0 a 1 e devolve a cor
func genMat(f func(u, v float64) [3]float64, matW, matH int) *ebiten.Image {
	return ebiten.NewImageFromImage(genRaw(f, matW, matH))
}

func genRaw(f func(u, v float64) [3]float64, matW, matH int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, matW, matH))
	for y := 0; y < matH; y++ {
		for x := 0; x < matW; x++ {
			u, v := (float64(x)+0.5)/float64(matW), (float64(y)+0.5)/float64(matH)
			c := f(u, v)
			vg := 1 - 0.55*math.Pow(math.Hypot(u-0.5, (v-0.5)*0.8)*1.5, 2.2) // escurece as bordas
			o := im.PixOffset(x, y)
			for k := 0; k < 3; k++ {
				im.Pix[o+k] = uint8(max(0, min(255, c[k]*vg)))
			}
			im.Pix[o+3] = 255
		}
	}
	return im
}

func buildMats() {
	if matsBuilt != nil {
		return
	}
	add := func(name string, f func(u, v float64) [3]float64) {
		matsBuilt = append(matsBuilt, &look{key: "b:" + name, name: name, gen: f})
	}
	matsBuilt = append(matsBuilt, &look{key: "", name: "Mesa Arcana (padrão)", gen: arcanaMat}, &look{key: "b:liso", name: "Feltro liso"})
	manaMats(add)
	add("Nebulosa", func(u, v float64) [3]float64 {
		n := fbm(u*5, v*3, 1)
		m := fbm(u*9+n*2, v*6-n, 7)
		c := mix([3]float64{14, 12, 40}, [3]float64{96, 44, 150}, n*1.5-0.25)
		c = mix(c, [3]float64{40, 150, 170}, math.Pow(m, 3)*1.6)
		if hash2(int(u*matW), int(v*matH), 3) > 0.9985 {
			c = [3]float64{235, 235, 255}
		}
		return c
	})
	add("Floresta", func(u, v float64) [3]float64 {
		n := fbm(u*7, v*4, 21)
		l := fbm(u*26, v*15, 5)
		c := mix([3]float64{8, 30, 20}, [3]float64{38, 104, 58}, n*1.4-0.2)
		return mix(c, [3]float64{130, 160, 70}, math.Pow(l, 4)*1.3)
	})
	add("Brasas", func(u, v float64) [3]float64 {
		n := fbm(u*6, v*4+0.3, 33)
		r := math.Abs(fbm(u*10+n, v*6, 9)-0.5) * 2
		c := mix([3]float64{26, 10, 10}, [3]float64{92, 24, 16}, n*1.4-0.2)
		return mix(c, [3]float64{255, 150, 40}, math.Pow(1-r, 9))
	})
	add("Abismo", func(u, v float64) [3]float64 {
		n := fbm(u*4, v*3, 51)
		w := math.Sin((v*9+fbm(u*3, v*3, 2)*5)*math.Pi)*0.5 + 0.5
		c := mix([3]float64{4, 16, 34}, [3]float64{16, 70, 110}, n*1.3-0.15)
		return mix(c, [3]float64{80, 190, 200}, math.Pow(w, 10)*0.5)
	})
	add("Pergaminho", func(u, v float64) [3]float64 {
		n := fbm(u*5, v*3, 77)
		g := fbm(u*40, v*24, 4)
		c := mix([3]float64{92, 76, 52}, [3]float64{150, 128, 92}, n*1.3-0.15)
		return mix(c, [3]float64{60, 46, 30}, math.Pow(g, 3)*0.5)
	})
	add("Grade arcana", func(u, v float64) [3]float64 {
		c := mix([3]float64{16, 22, 30}, [3]float64{30, 44, 56}, fbm(u*3, v*2, 90))
		gx, gy := math.Abs(math.Mod(u*16, 1)-0.5), math.Abs(math.Mod(v*9, 1)-0.5)
		if gx > 0.485 || gy > 0.485 {
			c = mix(c, [3]float64{70, 200, 170}, 0.35)
		}
		d := math.Hypot(u-0.5, (v-0.5)*9/16)
		for _, r := range []float64{0.12, 0.2, 0.205} {
			if math.Abs(d-r) < 0.0016 {
				c = mix(c, [3]float64{232, 185, 74}, 0.8)
			}
		}
		return c
	})
	add("Mármore", func(u, v float64) [3]float64 {
		n := fbm(u*4, v*3, 110)
		vn := math.Abs(math.Sin((u*4 + n*6) * math.Pi))
		c := mix([3]float64{38, 40, 46}, [3]float64{84, 88, 98}, n)
		return mix(c, [3]float64{190, 194, 204}, math.Pow(1-vn, 14)*0.8)
	})
}

type sleeveSpec struct {
	name                 string
	edge, bg, line, a, b color.RGBA
	motif                int // 0 losango · 1 anéis · 2 estrela · 3 escamas
}

func rgb(h uint32) color.RGBA { return color.RGBA{uint8(h >> 16), uint8(h >> 8), uint8(h), 0xff} }

func drawSleeve(sp sleeveSpec) *ebiten.Image {
	old := uiS
	uiS = 1
	defer func() { uiS = old }()
	im := ebiten.NewImage(int(pw), int(ph))
	rrect(im, 0, 0, pw, ph, 16, sp.edge)
	rrect(im, 10, 10, pw-20, ph-20, 10, sp.bg)
	rstroke(im, 22, 22, pw-44, ph-44, 8, 2, sp.line)
	cx, cy := float32(pw/2), float32(ph/2)
	fill := func(p *vector.Path, c color.Color) {
		op := &vector.DrawPathOptions{AntiAlias: true}
		op.ColorScale.ScaleWithColor(c)
		vector.FillPath(im, p, nil, op)
	}
	stroke := func(p *vector.Path, w float32, c color.Color) {
		op := &vector.DrawPathOptions{AntiAlias: true}
		op.ColorScale.ScaleWithColor(c)
		vector.StrokePath(im, p, &vector.StrokeOptions{Width: w, LineJoin: vector.LineJoinRound}, op)
	}
	poly := func(n int, r1, r2 float32, rot float64) *vector.Path {
		p := &vector.Path{}
		for k := 0; k < n*2; k++ {
			r := r1
			if k%2 == 1 {
				r = r2
			}
			a := rot + float64(k)*math.Pi/float64(n)
			x, y := cx+r*float32(math.Cos(a)), cy+r*float32(math.Sin(a))
			if k == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		p.Close()
		return p
	}
	switch sp.motif {
	case 1:
		for _, r := range []float32{118, 92, 66} {
			vector.StrokeCircle(im, cx, cy, r, 4, sp.a, true)
		}
		vector.FillCircle(im, cx, cy, 34, sp.b, true)
	case 2:
		fill(poly(8, 120, 52, -math.Pi/2), sp.edge)
		stroke(poly(8, 120, 52, -math.Pi/2), 5, sp.a)
		vector.FillCircle(im, cx, cy, 26, sp.b, true)
	case 3:
		for row := 0; row < 9; row++ {
			for col := -1; col < 7; col++ {
				x := float32(34 + col*46)
				if row%2 == 1 {
					x += 23
				}
				y := float32(60 + row*42)
				if x < 40 || x > float32(pw)-40 {
					continue
				}
				vector.StrokeCircle(im, x, y, 21, 2.5, sp.line, true)
			}
		}
		vector.FillCircle(im, cx, cy, 54, sp.edge, true)
		vector.StrokeCircle(im, cx, cy, 54, 5, sp.a, true)
		vector.FillCircle(im, cx, cy, 22, sp.b, true)
	default:
		d := poly(2, 78, 78, -math.Pi/2)
		fill(d, sp.edge)
		stroke(d, 6, sp.a)
		vector.FillCircle(im, cx, cy, 30, sp.b, true)
	}
	return im
}

func buildSleeves() {
	if sleevesBuilt != nil {
		return
	}
	for _, sp := range []sleeveSpec{
		{"Arcana", rgb(0x0b0f12), rgb(0x223038), rgb(0x344852), rgb(0xe8b94a), rgb(0x4fc3a1), 0},
		{"Rubi", rgb(0x16070a), rgb(0x5c1420), rgb(0x8a2a38), rgb(0xf0c9a0), rgb(0xff5a6a), 2},
		{"Esmeralda", rgb(0x06140e), rgb(0x12482f), rgb(0x1f6e49), rgb(0xd9c27a), rgb(0x6fe3a5), 3},
		{"Safira", rgb(0x070c1c), rgb(0x16306e), rgb(0x2a4c9a), rgb(0xcfd8e8), rgb(0x66b2ff), 1},
		{"Ametista", rgb(0x110a1c), rgb(0x3e2166), rgb(0x5f3a94), rgb(0xe8b94a), rgb(0xc59bff), 2},
		{"Ouro velho", rgb(0x171208), rgb(0x5a4516), rgb(0x86692a), rgb(0xf3e3b0), rgb(0xfff1c2), 1},
		{"Noite", rgb(0x000000), rgb(0x121417), rgb(0x2a2e33), rgb(0x8a929a), rgb(0xe6e9ec), 0},
		{"Coral", rgb(0x1c0d0a), rgb(0x8a3b2a), rgb(0xb85c44), rgb(0xffe2c4), rgb(0x4fc3a1), 3},
	} {
		sleevesBuilt = append(sleevesBuilt, &look{key: "b:" + sp.name, name: sp.name, tex: drawSleeve(sp)})
	}
}

// loadPicture abre uma imagem do usuário e reduz para um tamanho que a placa de vídeo aguenta bem
func loadPicture(path string, maxW int, card bool) *ebiten.Image {
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
	if card { // sleeve: recorta no formato da carta e arredonda os cantos
		w, h := int(pw), int(ph)
		crop := b
		if float64(b.Dx())/float64(b.Dy()) > pw/ph {
			nw := int(float64(b.Dy()) * pw / ph)
			crop = image.Rect(b.Min.X+(b.Dx()-nw)/2, b.Min.Y, b.Min.X+(b.Dx()-nw)/2+nw, b.Max.Y)
		} else {
			nh := int(float64(b.Dx()) * ph / pw)
			crop = image.Rect(b.Min.X, b.Min.Y+(b.Dy()-nh)/2, b.Max.X, b.Min.Y+(b.Dy()-nh)/2+nh)
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, xdraw.Src, nil)
		pic := ebiten.NewImageFromImage(dst)
		old := uiS
		uiS = 1
		mask := ebiten.NewImage(w, h)
		rrect(mask, 0, 0, pw, ph, 16, color.White)
		uiS = old
		out := ebiten.NewImage(w, h)
		out.DrawImage(mask, nil)
		op := &ebiten.DrawImageOptions{}
		op.Blend = ebiten.BlendSourceIn
		out.DrawImage(pic, op)
		return out
	}
	w, h := b.Dx(), b.Dy()
	if w > maxW {
		h = h * maxW / w
		w = maxW
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return ebiten.NewImageFromImage(dst)
}

func lookDir(kind string) string { return filepath.Join(baseDir, kind) }

// scanLooks lê as pastas playmats/ e sleeves/ ao lado do programa
func scanLooks() {
	read := func(kind string) []*look {
		os.MkdirAll(lookDir(kind), 0o755)
		var out []*look
		es, _ := os.ReadDir(lookDir(kind))
		for _, e := range es {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if e.IsDir() || (ext != ".png" && ext != ".jpg" && ext != ".jpeg") {
				continue
			}
			out = append(out, &look{key: "f:" + e.Name(), name: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))})
		}
		sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a].name) < strings.ToLower(out[b].name) })
		return out
	}
	matsFile, sleevesFile = read("playmats"), read("sleeves")
	looksScanned = true
	for k := range lookTex {
		delete(lookTex, k)
	}
}

// lookThumb: miniatura para a janela de aparência (os playmats do programa são pintados pequenos, na hora)
func lookThumb(kind string, l *look) *ebiten.Image {
	if l.gen == nil {
		return lookImage(kind, l)
	}
	if l.thumb == nil {
		l.thumb = genMat(l.gen, 256, 144)
	}
	return l.thumb
}

func lookImage(kind string, l *look) *ebiten.Image {
	if l.gen != nil && l.tex == nil { // pinta em segundo plano; até ficar pronto, vale a miniatura ampliada
		if l.raw == nil {
			l.raw = make(chan *image.RGBA, 1)
			go func(f func(u, v float64) [3]float64, ch chan *image.RGBA) { ch <- genRaw(f, matW, matH) }(l.gen, l.raw)
		}
		select {
		case im := <-l.raw:
			l.tex = ebiten.NewImageFromImage(im)
		default:
			return lookThumb(kind, l)
		}
	}
	if l.tex != nil || !strings.HasPrefix(l.key, "f:") {
		return l.tex
	}
	k := kind + "/" + l.key
	if im, ok := lookTex[k]; ok {
		return im
	}
	im := loadPicture(filepath.Join(lookDir(kind), l.key[2:]), 1920, kind == "sleeves")
	lookTex[k] = im
	return im
}

func findLook(kind, key string) *look {
	built, files := matsBuilt, matsFile
	if kind == "sleeves" {
		built, files = sleevesBuilt, sleevesFile
	}
	for _, l := range append(append([]*look{}, built...), files...) {
		if l.key == key {
			return l
		}
	}
	return nil
}

// matTex devolve o playmat escolhido (nil = feltro liso)
func (g *Game) matTex() *ebiten.Image {
	buildMats()
	if !looksScanned {
		scanLooks()
	}
	if l := findLook("playmats", cfg.Mat); l != nil {
		return lookImage("playmats", l)
	}
	return nil
}

// drawCover preenche o retângulo com a imagem, cortando o que sobrar
func drawCover(dst, tex *ebiten.Image, x, y, w, h float64, dim float32) {
	sub := clip(dst, x, y, w, h)
	b := tex.Bounds()
	s := max(w*uiS/float64(b.Dx()), h*uiS/float64(b.Dy()))
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Translate(-float64(b.Dx())/2, -float64(b.Dy())/2)
	op.GeoM.Scale(s, s)
	op.GeoM.Translate((x+w/2)*uiS, (y+h/2)*uiS)
	if dim < 1 {
		op.ColorScale.Scale(dim, dim, dim, 1)
	}
	sub.DrawImage(tex, op)
}

func openFolder(dir string) {
	os.MkdirAll(dir, 0o755)
	switch runtime.GOOS {
	case "windows":
		exec.Command("explorer", dir).Start()
	case "darwin":
		exec.Command("open", dir).Start()
	default:
		exec.Command("xdg-open", dir).Start()
	}
}

func (g *Game) looksModal() {
	buildMats()
	buildSleeves()
	scanLooks()
	m := &Modal{title: "Aparência: playmat e sleeve", w: 940, h: 790}
	m.draw = func(x, y, w, h float64) {
		section := func(title, kind string, cur *string, tw0, th0 float64, y0 float64) float64 {
			txt(g.dst, title, x, y0, fBold, 15.5, cAcc)
			built, files := matsBuilt, matsFile
			if kind == "sleeves" {
				built, files = sleevesBuilt, sleevesFile
			}
			all := append(append([]*look{}, built...), files...)
			cx, cy := x, y0+28
			for _, l := range all {
				if cx+tw0 > x+w+1 {
					cx, cy = x, cy+th0+30
				}
				if cy+th0+24 > y+h-56 {
					break
				}
				sel := *cur == l.key
				if sel {
					rrect(g.dst, cx-4, cy-4, tw0+8, th0+8, 9, cAcc)
				}
				rrect(g.dst, cx, cy, tw0, th0, 6, cFelt)
				if im := lookThumb(kind, l); im != nil {
					if kind == "sleeves" {
						drawTex(g.dst, im, cx, cy, tw0, false, 1)
					} else {
						drawCover(g.dst, im, cx, cy, tw0, th0, 1)
					}
				} else if strings.HasPrefix(l.key, "f:") {
					txtC(g.dst, "não abriu", cx+tw0/2, cy+th0/2-8, fReg, 11.5, cBad)
				}
				col := cMut
				if sel {
					col = cTxt
				}
				txtC(g.dst, fit(l.name, fReg, 12, tw0+10), cx+tw0/2, cy+th0+6, fReg, 12, col)
				key := l.key
				g.add(region{x: cx, y: cy, w: tw0, h: th0 + 22, click: func() {
					*cur = key
					g.back = nil
					saveCfg()
				}})
				cx += tw0 + 18
			}
			return cy + th0 + 40
		}
		ny := section("Playmat (fundo do campo de batalha)", "playmats", &cfg.Mat, 112, 63, y)
		section("Sleeve (verso das cartas)", "sleeves", &cfg.Sleeve, 78, 78*ratio, ny)
		hint := "Para usar os seus: coloque imagens PNG ou JPG nas pastas playmats e sleeves, ao lado do programa, e clique em Atualizar."
		txt(g.dst, fit(hint, fReg, 12.5, w), x, y+h-58, fReg, 12.5, cMut)
		bx := x
		bx += 8 + g.button(bx, y+h-32, 0, 30, "Abrir pasta de playmats", 0, func() { openFolder(lookDir("playmats")) })
		bx += 8 + g.button(bx, y+h-32, 0, 30, "Abrir pasta de sleeves", 0, func() { openFolder(lookDir("sleeves")) })
		g.button(bx, y+h-32, 0, 30, "Atualizar", 0, func() { scanLooks(); g.back = nil })
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}
	g.openModal(m)
}

// sleeveTex devolve o verso escolhido (nil = o padrão antigo)
func (g *Game) sleeveTex() *ebiten.Image {
	buildSleeves()
	if !looksScanned {
		scanLooks()
	}
	key := cfg.Sleeve
	if key == "" {
		key = "b:Arcana"
	}
	if l := findLook("sleeves", key); l != nil {
		if im := lookImage("sleeves", l); im != nil {
			return im
		}
	}
	return sleevesBuilt[0].tex
}

/* ====================== ícone ====================== */

//go:embed icone.png
var iconPNG []byte

func windowIcons() []image.Image {
	im, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		return nil
	}
	out := []image.Image{im}
	for _, s := range []int{48, 32, 16} {
		d := image.NewRGBA(image.Rect(0, 0, s, s))
		xdraw.CatmullRom.Scale(d, d.Bounds(), im, im.Bounds(), xdraw.Over, nil)
		out = append(out, d)
	}
	return out
}
