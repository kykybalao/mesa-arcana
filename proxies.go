package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Fazedor de proxies: desenha qualquer carta no formato de carta de Magic e grava em PNG (63 x 88 mm a 300 dpi)
// ou em folhas A4 com 9 cartas. A arte é só o recorte da ilustração (dados/artes), não a carta inteira.

func artPath(key string) string {
	id, f, _ := strings.Cut(key, ":")
	if f == "1" {
		id += "_b"
	}
	return filepath.Join(baseDir, "dados", "artes", id+".jpg")
}
func artURL(key string) string { return strings.Replace(imgURL(key), "/normal/", "/art_crop/", 1) }
func hasArt(key string) bool {
	st, err := os.Stat(artPath(key))
	return err == nil && st.Size() > 1000
}

// styleOf: estilo em que a carta é desenhada quando não há imagem da carta inteira
func styleOf(c *Card) string {
	if c.Mine {
		return c.Est
	}
	if c.Custom {
		return ""
	}
	return cfg.Proxy
}

func specOf(c *Card, fi int, style string) cardSpec {
	f, _ := loc(c, fi)
	cost := f.C
	if cost == "" {
		cost = c.FaceAt(0).C
	}
	sp := cardSpec{f: f, style: style, sub: c.Sub, kind: frameKind(cost, f.T+" "+c.FaceAt(fi).T, c.Cor)}
	key := c.ID + ":" + string(rune('0'+fi))
	if c.Mine {
		sp.art = artTex(imgPath(key))
	} else {
		sp.art = artTex(artPath(key))
		if a := looks.Alias[c.ID]; a != "" && fi == 0 {
			sp.sub, sp.f.N = f.N, a
		}
	}
	sp.adj = looks.Adj[key]
	sp.mark, sp.hint = markOf(c, fi)
	if sp.kind == "A" && c.Cor == "" {
		if m := meldOf(c); m != nil && m.result == c { // a carta fundida não tem custo: usa as cores das metades
			cs := ""
			for _, p := range m.parts {
				cs += p.C
			}
			sp.kind = frameKind(cs, f.T, "")
		}
	}
	return sp
}

type meld struct {
	result *Card
	parts  []*Card
}

var (
	meldRe   = regexp.MustCompile(`meld them into ([^.]+)\.`)
	meldByID map[string]*meld
)

// meldOf: fusão de que a carta participa (como metade ou como resultado); descoberta pelo texto das cartas
func meldOf(c *Card) *meld {
	if meldByID == nil {
		meldByID = map[string]*meld{}
		byRes := map[string]*meld{}
		for _, x := range allCards {
			if m := meldRe.FindStringSubmatch(x.X); m != nil {
				r := findCard(m[1])
				if r == nil {
					continue
				}
				if byRes[r.ID] == nil {
					byRes[r.ID] = &meld{result: r}
					meldByID[r.ID] = byRes[r.ID]
				}
				byRes[r.ID].parts = append(byRes[r.ID].parts, x)
				meldByID[x.ID] = byRes[r.ID]
			}
		}
	}
	return meldByID[c.ID]
}

// markOf escolhe o símbolo e a linha de aviso da carta
func markOf(c *Card, fi int) (string, string) {
	if c.Mine {
		return c.Mar, c.Dica
	}
	name := func(k int) string { f, _ := loc(c, k); return f.N }
	if len(c.F) == 2 {
		o := c.F[1-fi]
		back := c.F[1]
		modal := back.C != "" || strings.Contains(back.T, "Land")
		pt := ""
		if o.P != "" || o.O != "" {
			pt = " (" + o.P + "/" + o.O + ")"
		}
		side := "Verso: "
		if fi == 1 {
			side = "Frente: "
		}
		switch {
		case modal && fi == 0:
			return "modal1", side + name(1)
		case modal:
			return "modal2", side + name(0)
		case fi == 0:
			return "sol", side + name(1) + pt
		}
		return "lua", side + name(0) + pt
	}
	if m := meldOf(c); m != nil {
		rf, _ := loc(m.result, 0)
		if m.result == c {
			return "fusao", ""
		}
		for _, p := range m.parts {
			if p != c {
				pf, _ := loc(p, 0)
				return "fusao", "Funde-se com " + pf.N + " em " + rf.N
			}
		}
		return "fusao", "Funde-se em " + rf.N
	}
	t := strings.ToLower(c.T)
	if strings.Contains(t, "lesson") {
		return "licao", ""
	}
	return "", ""
}

// isTall: carta que é impressa com o dobro da altura (resultado de fusão)
func isTall(c *Card) bool {
	if c.Mine {
		return c.Alta
	}
	m := meldOf(c)
	return m != nil && m.result == c
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '-'
		}
		return r
	}, s)
	s = strings.TrimSpace(strings.Trim(s, ". "))
	if s == "" {
		s = "carta"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func proxyDir() string { return filepath.Join(baseDir, "proxies") }

// grab copia a textura para a memória (precisa rodar dentro do laço do jogo)
func grab(im *ebiten.Image) *image.RGBA {
	b := im.Bounds()
	out := image.NewRGBA(b)
	im.ReadPixels(out.Pix)
	return out
}

func writePNG(path string, im image.Image) error {
	os.MkdirAll(filepath.Dir(path), 0o755)
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, im); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

type pxJob struct {
	c    *Card
	fi   int
	path string // "" = só entra na folha
	n    int    // cópias na folha
}

// pxExport: fila de cartas a desenhar (uma por quadro, para a janela não travar)
type pxExport struct {
	jobs   []pxJob
	total  int
	style  string
	sheets bool
	dir    string
	page   []*image.RGBA
	pages  int
	wg     sync.WaitGroup
	fail   atomic.Int32
	saved  int
}

const a4W, a4H = 2480, 3508

func (e *pxExport) flush() {
	if len(e.page) == 0 {
		return
	}
	e.pages++
	cards, n := e.page, e.pages
	e.page = nil
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		sh := image.NewRGBA(image.Rect(0, 0, a4W, a4H))
		draw.Draw(sh, sh.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		w, h := int(cdW), int(cdH)
		x0, y0 := (a4W-3*w)/2, (a4H-3*h)/2
		for i, c := range cards {
			x, y := x0+(i%3)*w, y0+(i/3)*h
			draw.Draw(sh, image.Rect(x, y, x+w, y+h), c, image.Point{}, draw.Over)
		}
		// marcas de corte nas margens
		gray := image.NewUniform(color.RGBA{0x70, 0x70, 0x70, 0xff})
		for i := 0; i <= 3; i++ {
			x, y := x0+i*w, y0+i*h
			draw.Draw(sh, image.Rect(x, y0-70, x+1, y0-14), gray, image.Point{}, draw.Src)
			draw.Draw(sh, image.Rect(x, y0+3*h+14, x+1, y0+3*h+70), gray, image.Point{}, draw.Src)
			draw.Draw(sh, image.Rect(x0-70, y, x0-14, y+1), gray, image.Point{}, draw.Src)
			draw.Draw(sh, image.Rect(x0+3*w+14, y, x0+3*w+70, y+1), gray, image.Point{}, draw.Src)
		}
		if writePNG(filepath.Join(e.dir, fmt.Sprintf("folha-%02d.png", n)), sh) != nil {
			e.fail.Add(1)
		}
	}()
}

// step desenha a próxima carta; devolve false quando terminou
func (e *pxExport) step(g *Game) bool {
	if len(e.jobs) == 0 {
		return false
	}
	j := e.jobs[0]
	e.jobs = e.jobs[1:]
	im := g.printable(j.c, j.fi, e.style)
	raw := grab(im)
	im.Deallocate()
	parts, names := []*image.RGBA{raw}, []string{j.path}
	if raw.Bounds().Dy() > int(cdH)+2 { // carta alta: metade de cima e metade de baixo
		h := int(cdH)
		parts = nil
		for k := 0; k < 2; k++ {
			half := image.NewRGBA(image.Rect(0, 0, int(cdW), h))
			draw.Draw(half, half.Bounds(), raw, image.Point{0, k * h}, draw.Src)
			parts = append(parts, half)
		}
		base := strings.TrimSuffix(j.path, ".png")
		names = []string{base + " (cima).png", base + " (baixo).png"}
	}
	for k, part := range parts {
		if j.path != "" {
			if writePNG(names[k], part) != nil {
				e.fail.Add(1)
			} else {
				e.saved++
			}
		}
		if e.sheets {
			for q := 0; q < j.n; q++ {
				e.page = append(e.page, part)
				if len(e.page) == 9 {
					e.flush()
				}
			}
		}
	}
	if len(e.jobs) == 0 {
		e.flush()
	}
	return true
}

// printable desenha a carta no tamanho de impressão; no estilo playtest amplia o proxy bege
func (g *Game) printable(c *Card, fi int, style string) *ebiten.Image {
	if style != "" {
		sp := specOf(c, fi, style)
		sp.tall = isTall(c)
		return renderCard(sp, 1)
	}
	saveProxy, saveEst := cfg.Proxy, c.Est
	cfg.Proxy, c.Est = "", ""
	key := c.ID + ":" + string(rune('0'+fi)) + cfg.Lang
	delete(g.proxies, key)
	src := g.proxy(c, fi)
	delete(g.proxies, key)
	cfg.Proxy, c.Est = saveProxy, saveEst
	out := ebiten.NewImage(int(cdW), int(cdH))
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(cdW/pw, cdH/ph)
	out.DrawImage(src, op)
	src.Deallocate()
	return out
}

func isBasicLand(c *Card) bool {
	return strings.HasPrefix(c.T, "Basic Land") || strings.HasPrefix(c.T, "Basic Snow Land")
}

func deckJobs(d *Deck, basics bool) (cards []pxJob) {
	for _, e := range d.Entries {
		c := e.Card
		if c == nil || (!basics && isBasicLand(c)) {
			continue
		}
		for fi := range c.Faces() {
			if fi > 0 && len(c.F) < 2 {
				break
			}
			cards = append(cards, pxJob{c: c, fi: fi, n: max(1, e.Qty)})
		}
	}
	return
}

/* ---------------- baixar só a arte ---------------- */

type artDL struct {
	running atomic.Bool
	msg     atomic.Value
}

var adl artDL

func (a *artDL) start(keys []string, done func()) {
	if a.running.Load() {
		return
	}
	var need []string
	for _, k := range keys {
		if !hasArt(k) {
			need = append(need, k)
		}
	}
	if len(need) == 0 {
		a.msg.Store("As artes já estão guardadas em dados/artes.")
		return
	}
	a.running.Store(true)
	a.msg.Store(fmt.Sprintf("Baixando %d arte(s)…", len(need)))
	go func() {
		defer a.running.Store(false)
		cl := &http.Client{Timeout: 25 * time.Second}
		ok, fail, streak := 0, 0, 0
		last := ""
		os.MkdirAll(filepath.Dir(artPath("x:0")), 0o755)
		for i, k := range need {
			if err := fetchArt(cl, k); err != nil {
				fail++
				streak++
				last = shortErr(err)
				if streak >= 4 {
					a.msg.Store("Sem acesso ao Scryfall daqui (" + last + "). Tente em uma rede sem bloqueio.")
					return
				}
			} else {
				ok++
				streak = 0
			}
			a.msg.Store(fmt.Sprintf("Baixando artes: %d de %d…", i+1, len(need)))
			time.Sleep(100 * time.Millisecond)
		}
		if fail > 0 {
			a.msg.Store(fmt.Sprintf("Baixei %d arte(s); %d falharam (%s).", ok, fail, last))
		} else {
			a.msg.Store(fmt.Sprintf("Pronto: %d arte(s) guardadas em dados/artes.", ok))
		}
		if done != nil {
			done()
		}
	}()
}

func fetchArt(cl *http.Client, key string) error {
	req, _ := http.NewRequest("GET", artURL(key), nil)
	req.Header.Set("User-Agent", "MesaArcana/1.0")
	req.Header.Set("Accept", "image/jpeg,*/*")
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(b)); err != nil {
		return fmt.Errorf("resposta não é imagem")
	}
	tmp := artPath(key) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, artPath(key))
}

func (g *Game) clearProxies() {
	for k, im := range g.proxies {
		im.Deallocate()
		delete(g.proxies, k)
	}
}

/* ---------------- nome novo e ajuste da arte ---------------- */

// cardLooks guarda, por carta, o nome novo (o original vai embaixo) e o ajuste da arte (zoom e posição)
type cardLooks struct {
	Alias map[string]string     `json:"nomes"`
	Adj   map[string][3]float64 `json:"ajustes"`
}

var looks = cardLooks{Alias: map[string]string{}, Adj: map[string][3]float64{}}

func looksPath() string { return filepath.Join(baseDir, "dados", "proxies.json") }
func loadLooks() {
	if b, err := os.ReadFile(looksPath()); err == nil {
		json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &looks)
	}
	if looks.Alias == nil {
		looks.Alias = map[string]string{}
	}
	if looks.Adj == nil {
		looks.Adj = map[string][3]float64{}
	}
}
func saveLooks() {
	if b, err := json.MarshalIndent(looks, "", " "); err == nil {
		os.MkdirAll(filepath.Dir(looksPath()), 0o755)
		os.WriteFile(looksPath(), b, 0o644)
	}
}

func setAdj(key string, a [3]float64) {
	if a[0] == 0 {
		a[0] = 1
	}
	a[0] = max(0.3, min(6, a[0]))
	a[1], a[2] = max(-1, min(1, a[1])), max(-1, min(1, a[2]))
	if a == [3]float64{1, 0, 0} {
		delete(looks.Adj, key)
	} else {
		looks.Adj[key] = a
	}
}
func getAdj(key string) [3]float64 {
	a := looks.Adj[key]
	if a[0] == 0 {
		a[0] = 1
	}
	return a
}

var (
	picking atomic.Bool
	picked  atomic.Value // caminho escolhido na janela do Windows
	artGrab struct {
		on     bool
		lx, ly int
	}
)

// artTools: botões de escolher imagem do PC e de ajustar a arte; dest é o arquivo onde a arte da carta fica.
// A prévia em (px, py, pw) aceita arrastar (mover) e a roda do mouse (zoom). Devolve a altura usada e um aviso.
func (g *Game) artTools(key, dest string, x, y, w, px, py, pw float64, changed func()) (float64, string) {
	note := ""
	if v, _ := picked.Load().(string); v != "" {
		picked.Store("")
		b, err := os.ReadFile(v)
		if err != nil {
			note = "Não consegui abrir esse arquivo."
		} else if _, _, err := image.Decode(bytes.NewReader(b)); err != nil {
			note = "Esse arquivo não é uma imagem PNG ou JPG."
		} else {
			os.MkdirAll(filepath.Dir(dest), 0o755)
			os.WriteFile(dest, b, 0o644)
			delete(looks.Adj, key)
			saveLooks()
			changed()
			note = "Imagem colocada. Arraste a prévia para mover e use a roda do mouse para o zoom."
		}
	}
	a := getAdj(key)
	set := func(n [3]float64) {
		setAdj(key, n)
		saveLooks()
		changed()
	}
	// arrastar e roda do mouse em cima da prévia
	ph := pw * ratio
	in := float64(g.mx) >= px && float64(g.mx) < px+pw && float64(g.my) >= py && float64(g.my) < py+ph
	down := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if artGrab.on && down {
		dx, dy := float64(g.mx-artGrab.lx), float64(g.my-artGrab.ly)
		if dx != 0 || dy != 0 {
			k := 1 / (pw * 0.35 * max(0.6, a[0]))
			set([3]float64{a[0], a[1] - dx*k, a[2] - dy*k})
			artGrab.lx, artGrab.ly = g.mx, g.my
		}
	} else if down && in && !artGrab.on && g.ctx == nil {
		artGrab.on, artGrab.lx, artGrab.ly = true, g.mx, g.my
	} else if !down {
		artGrab.on = false
	}
	g.add(region{x: px, y: py, w: pw, h: ph, wheel: func(dy float64) { set([3]float64{a[0] * (1 + 0.08*dy), a[1], a[2]}) }})
	y0 := y
	if canPick {
		st := 0
		if picking.Load() {
			st = 3
		}
		g.button(x, y, w, 28, "Escolher imagem do PC…", st, func() {
			if picking.CompareAndSwap(false, true) {
				go func() {
					defer picking.Store(false)
					picked.Store(pickImage())
				}()
			}
		})
		y += 32
	}
	bw := (w - 4*4) / 5
	bx := x
	for _, b := range []struct {
		lab string
		fn  func()
	}{
		{"−", func() { set([3]float64{a[0] / 1.1, a[1], a[2]}) }},
		{"+", func() { set([3]float64{a[0] * 1.1, a[1], a[2]}) }},
		{"←", func() { set([3]float64{a[0], a[1] + 0.1, a[2]}) }},
		{"→", func() { set([3]float64{a[0], a[1] - 0.1, a[2]}) }},
		{"↑", func() { set([3]float64{a[0], a[1], a[2] + 0.1}) }},
	} {
		g.button(bx, y, bw, 26, b.lab, 0, b.fn)
		bx += bw + 4
	}
	y += 30
	g.button(x, y, bw, 26, "↓", 0, func() { set([3]float64{a[0], a[1], a[2] - 0.1}) })
	g.button(x+bw+4, y, w-bw-4, 26, fmt.Sprintf("Zoom %d%% · centralizar", int(a[0]*100+0.5)), 0, func() { set([3]float64{1, 0, 0}) })
	y += 30
	return y - y0, note
}

/* ---------------- janela ---------------- */

var pxTest func(what string) string // usado pelo roteiro de teste

func (g *Game) proxyModal() {
	var (
		deck    *Deck
		cur     *Card
		fi      int
		style   = "moldura"
		basics  bool
		msg     string
		exp     *pxExport
		pv      *ebiten.Image
		pvKey   string
		scrollB float64
		dirty   atomic.Bool
	)
	if len(g.decks) > 0 {
		deck = g.decks[min(g.sel, len(g.decks)-1)]
	}
	g.fields["pxq"] = ""
	pick := func(c *Card) {
		cur, fi, pvKey = c, 0, ""
		g.fields["pxa"] = looks.Alias[c.ID]
	}
	if deck != nil && len(deck.Entries) > 0 {
		pick(deck.Entries[0].Card)
	}
	one := func() {
		if cur == nil || exp != nil {
			return
		}
		name := safeName(cur.N)
		if fi == 1 {
			name += " (verso)"
		}
		exp = &pxExport{style: style, dir: proxyDir(), jobs: []pxJob{{c: cur, fi: fi, path: filepath.Join(proxyDir(), name+".png")}}}
		exp.total = 1
	}
	all := func(sheets bool) {
		if deck == nil || exp != nil {
			return
		}
		dir := filepath.Join(proxyDir(), safeName(deck.Name))
		jobs := deckJobs(deck, basics)
		if !sheets {
			for i := range jobs {
				name := safeName(jobs[i].c.N)
				if jobs[i].fi == 1 {
					name += " (verso)"
				}
				jobs[i].path = filepath.Join(dir, name+".png")
			}
		}
		if len(jobs) == 0 {
			msg = "Esse deck não tem cartas para gravar."
			return
		}
		exp = &pxExport{style: style, dir: dir, jobs: jobs, total: len(jobs), sheets: sheets}
	}
	pxTest = func(what string) string {
		switch what {
		case "carta":
			one()
		case "deck":
			all(false)
		case "folhas":
			all(true)
		case "estilo":
			style = nextStyle(style)
			pvKey = ""
		}
		return style
	}
	m := &Modal{title: "Fazer proxies", w: 1120, h: 720}
	m.onClose = func() {
		if pv != nil {
			pv.Deallocate()
		}
		pxTest = nil
	}
	m.draw = func(x, y, w, h float64) {
		// exportação em andamento: uma carta por quadro
		if exp != nil {
			if exp.step(g) {
				msg = fmt.Sprintf("Desenhando %d de %d…", exp.total-len(exp.jobs), exp.total)
			} else {
				e := exp
				exp = nil
				e.wg.Wait()
				switch {
				case e.fail.Load() > 0:
					msg = "Não consegui gravar alguns arquivos na pasta proxies."
				case e.sheets:
					msg = fmt.Sprintf("Pronto: %d folha(s) A4 em proxies/%s. Imprima em tamanho real (100%%).", e.pages, filepath.Base(e.dir))
				default:
					msg = fmt.Sprintf("Pronto: %d PNG gravado(s) na pasta %s.", e.saved, strings.TrimPrefix(e.dir, baseDir+string(filepath.Separator)))
				}
			}
		}
		if dirty.Swap(false) {
			pvKey = ""
			g.clearProxies()
		}
		// imagem arrastada vira a arte da carta mostrada
		if fsys := ebiten.DroppedFiles(); fsys != nil && cur != nil {
			if es, err := fs.ReadDir(fsys, "."); err == nil && len(es) > 0 {
				if b, err := fs.ReadFile(fsys, es[0].Name()); err == nil {
					if _, _, err := image.Decode(bytes.NewReader(b)); err != nil {
						msg = "Esse arquivo não é uma imagem PNG ou JPG."
					} else {
						p := artPath(cur.ID + ":" + string(rune('0'+fi)))
						if cur.Mine {
							p = imgPath(cur.ID + ":0")
						}
						os.MkdirAll(filepath.Dir(p), 0o755)
						os.WriteFile(p, b, 0o644)
						g.forgetProxy(cur.ID)
						pvKey = ""
						msg = "Arte colocada em “" + cur.N + "”."
					}
				}
			}
		}
		// coluna A: decks
		aw := 210.0
		txt(g.dst, "Decks", x, y, fBold, 14.5, cAcc)
		listY, listH := y+30, h-30
		old := g.dst
		g.dst = clip(old, x, listY, aw, listH)
		for k, d := range g.decks {
			cy := listY + float64(k)*34
			bg := cPanel2
			if d == deck {
				bg = cLine
			}
			rrect(g.dst, x, cy, aw, 30, 6, bg)
			txt(g.dst, fit(d.Name, fReg, 13.5, aw-20), x+10, cy+7, fReg, 13.5, cTxt)
			if cy+30 <= listY+listH {
				g.add(region{x: x, y: cy, w: aw, h: 30, click: func() {
					deck, scrollB = d, 0
					g.fields["pxq"] = ""
					if len(d.Entries) > 0 {
						pick(d.Entries[0].Card)
					}
				}})
			}
		}
		g.dst = old
		// coluna B: cartas do deck ou busca
		bx, bw := x+aw+20, 280.0
		g.field("pxq", bx, y-4, bw, "Procurar qualquer carta…")
		var list []*Card
		if q := strings.TrimSpace(g.fields["pxq"]); q != "" {
			list = searchCards(q, 40)
			if c := findPT(q); c != nil {
				list = append([]*Card{c}, list...)
			}
		} else if deck != nil {
			for _, e := range deck.Entries {
				if e.Card != nil {
					list = append(list, e.Card)
				}
			}
		}
		listY, listH = y+36, h-36
		scrollB = max(0, min(scrollB, float64(len(list))*30-listH))
		g.add(region{x: bx, y: listY, w: bw, h: listH, wheel: func(dy float64) { scrollB -= dy * 30 }})
		g.dst = clip(old, bx, listY, bw, listH)
		for k, c := range list {
			cy := listY + float64(k)*30 - scrollB
			if cy+30 < listY || cy > listY+listH {
				continue
			}
			bg := cPanel
			if c == cur {
				bg = cLine
			}
			rrect(g.dst, bx, cy, bw, 27, 5, bg)
			f, _ := loc(c, 0)
			txt(g.dst, fit(f.N, fReg, 13, bw-18), bx+9, cy+6, fReg, 13, cTxt)
			if cy >= listY && cy+27 <= listY+listH {
				g.add(region{x: bx, y: cy, w: bw, h: 27, click: func() { pick(c) }})
			}
		}
		g.dst = old
		// coluna C: prévia
		px, pvW := bx+bw+20, 300.0
		if cur != nil {
			ckey := cur.ID + ":" + string(rune('0'+fi))
			if !cur.Mine {
				if a := strings.TrimSpace(g.fields["pxa"]); a != looks.Alias[cur.ID] {
					if a == "" {
						delete(looks.Alias, cur.ID)
					} else {
						looks.Alias[cur.ID] = a
					}
					saveLooks()
					g.forgetProxy(cur.ID)
				}
			}
			key := cur.ID + fmt.Sprint(fi, looks.Alias[cur.ID], looks.Adj[ckey]) + style + cfg.Lang
			ty := y + pvW*ratio + 12
			if !cur.Mine {
				g.field("pxa", px, ty, pvW, "Nome novo (o original fica embaixo)")
				ty += 38
			}
			if style != "" {
				dest := artPath(ckey)
				if cur.Mine {
					dest = imgPath(cur.ID + ":0")
				}
				c := cur
				_, n := g.artTools(ckey, dest, px, ty, pvW, px, y, pvW, func() { g.forgetProxy(c.ID); pvKey = "" })
				if n != "" {
					msg = n
				}
			}
			if key != pvKey {
				if pv != nil {
					pv.Deallocate()
				}
				pv, pvKey = g.printable(cur, fi, style), key
			}
			drawFitted(g.dst, pv, px, y, pvW)
		} else {
			txt(g.dst, "Escolha uma carta.", px, y, fReg, 13.5, cMut)
		}
		// coluna D: opções
		dx := px + pvW + 20
		dw := x + w - dx
		dy := y
		txt(g.dst, "Estilo", dx, dy, fBold, 14.5, cAcc)
		dy += 26
		for _, s := range styleNames[1:] {
			st := 0
			if s[0] == style {
				st = 1
			}
			g.button(dx, dy, dw, 30, s[1], st, func() { style, pvKey = s[0], "" })
			dy += 34
		}
		st := 0
		if style == "" {
			st = 1
		}
		g.button(dx, dy, dw, 30, "Playtest (bege)", st, func() { style, pvKey = "", "" })
		dy += 44
		if cur != nil && len(cur.F) > 1 {
			g.button(dx, dy, dw, 28, fmt.Sprintf("Face %d de %d (trocar)", fi+1, len(cur.F)), 0, func() { fi, pvKey = (fi+1)%len(cur.F), "" })
			dy += 36
		}
		txt(g.dst, "Arte", dx, dy, fBold, 14.5, cAcc)
		dy += 24
		busy := 0
		if adl.running.Load() || exp != nil {
			busy = 3
		}
		refresh := func() { dirty.Store(true) }
		if cur != nil && !cur.Mine {
			g.button(dx, dy, dw, 28, "Baixar a arte desta carta", busy, func() {
				adl.start([]string{cur.ID + ":" + string(rune('0'+fi))}, refresh)
			})
			dy += 32
		}
		if deck != nil {
			g.button(dx, dy, dw, 28, "Baixar as artes do deck", busy, func() {
				var keys []string
				for _, j := range deckJobs(deck, true) {
					if !j.c.Custom {
						keys = append(keys, j.c.ID+":"+string(rune('0'+j.fi)))
					}
				}
				adl.start(keys, refresh)
			})
			dy += 32
		}
		for _, l := range wrap("Baixa só a ilustração. Para usar uma imagem sua, use o botão embaixo da prévia ou arraste o arquivo para a janela.", fReg, 12, dw) {
			txt(g.dst, l, dx, dy, fReg, 12, cMut)
			dy += 15
		}
		dy += 12
		txt(g.dst, "Gravar", dx, dy, fBold, 14.5, cAcc)
		dy += 24
		g.button(dx, dy, dw, 28, "Esta carta em PNG", busy, one)
		dy += 32
		g.button(dx, dy, dw, 28, "Deck inteiro em PNGs", busy, func() { all(false) })
		dy += 32
		g.button(dx, dy, dw, 28, "Folhas A4 (9 por folha)", busy, func() { all(true) })
		dy += 32
		lab := "Terrenos básicos: não"
		if basics {
			lab = "Terrenos básicos: sim"
		}
		g.button(dx, dy, dw, 28, lab, 0, func() { basics = !basics })
		dy += 32
		g.button(dx, dy, dw, 28, "Abrir a pasta proxies", 0, func() { openFolder(proxyDir()) })
		dy += 44
		txt(g.dst, "Na mesa", dx, dy, fBold, 14.5, cAcc)
		dy += 24
		g.button(dx, dy, dw, 28, "Sem imagem: "+styleLabel(cfg.Proxy), 0, func() {
			cfg.Proxy = nextStyle(cfg.Proxy)
			saveCfg()
			g.clearProxies()
		})
		dy += 34
		note := msg
		if v, _ := adl.msg.Load().(string); v != "" && (adl.running.Load() || msg == "") {
			note = v
		}
		for _, l := range wrap(note, fBold, 12.5, dw) {
			txt(g.dst, l, dx, dy, fBold, 12.5, cAcc2)
			dy += 16
		}
	}
	g.openModal(m)
}

// drawFitted desenha a prévia; a carta alta (fusão) aparece inteira, mais estreita, no mesmo espaço
func drawFitted(dst, tex *ebiten.Image, x, y, w float64) {
	b := tex.Bounds()
	if float64(b.Dy())/float64(b.Dx()) < ratio*1.2 {
		drawTex(dst, tex, x, y, w, false, 1)
		return
	}
	h := w * ratio
	s := h * uiS / float64(b.Dy())
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate((x+w/2)*uiS-float64(b.Dx())*s/2, y*uiS)
	dst.DrawImage(tex, op)
}
