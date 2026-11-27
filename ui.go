package main

import (
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type region struct {
	x, y, w, h float64
	click      func()
	rclick     func()
	wheel      func(dy float64)
	inst       int
	pv         *Card
	pvFace     int
	pvHidden   bool
	zone       string
	block      bool
}

func (r *region) has(x, y int) bool {
	return float64(x) >= r.x && float64(x) < r.x+r.w && float64(y) >= r.y && float64(y) < r.y+r.h
}

type Modal struct {
	title   string
	w, h    float64
	draw    func(x, y, w, h float64)
	onClose func()
	scroll  float64
}
type item struct {
	hd, label, key string
	fn             func()
	pm             func(d int)
	val            func() int
}
type Ctx struct {
	x, y  float64
	items []item
}

type Game struct {
	S       *State
	undo    [][]byte
	deck    *Deck
	decks   []*Deck
	sel     int
	screen  string
	customs map[string]*Card
	proxies map[string]*ebiten.Image
	back    *ebiten.Image
	cw      float64
	bf      image.Rectangle
	handPos []float64
	regs    []region
	dst     *ebiten.Image
	W, H    int
	mx, my  int
	modal   *Modal
	ctx     *Ctx
	drag    struct {
		on, moved bool
		id        int
		sx, sy    int
		ox, oy    float64
	}
	lastID    int
	lastT     time.Time
	pv        *Card
	pvFace    int
	pvHide    bool
	focus     string
	fields    map[string]string
	onEnter   func()
	msg       string
	imgHave   int
	imgTot    int
	imgTick   int
	artHave   int
	zf        *zoneFilter
	backOver  *ebiten.Image
	listScr   float64
	frame     int
	test      *tester
	inModal   bool
	inCtx     bool
	shot      string
	rebind    string // ação esperando a tecla nova (janela de atalhos)
	started   time.Time
	saved     bool // esta partida já foi registrada no histórico
	banner    string
	bannerT   int
	ed        *Editor
	showHand  bool     // minha mão está revelada para a mesa
	chat      []string // mensagens do chat da mesa
	arrows    []Arrow
	table     *TableTurn // ordem da mesa em rede (quem começa, vez, tranca, tempo)
	unlocked  bool       // pediu para responder na vez de outro
	tableOff  bool       // só para o roteiro de teste
	turnStart time.Time
	zoom      *zoomView
	pvNow     bool // o cursor está em cima de uma carta agora
	viewMode  int  // 1 uma mesa, 2 lado a lado, 3 empilhado, 4 quadrante
	clampBF   bool // conferir se alguma carta ficou fora do campo
	au        arrowUI
	viewPeer  int // 0 = minha mesa; senão, o jogador cuja mesa está na tela
	anKey     string
	anRes     *Analysis
	equip     int // carta esperando o clique no alvo para ser anexada
	drawn     bool
}

func (g *Game) Layout(w, h int) (int, int) {
	if m := ebiten.Monitor(); m != nil {
		if s := m.DeviceScaleFactor(); s >= 1 && s <= 4 {
			uiS = s
		}
	}
	return int(float64(w) * uiS), int(float64(h) * uiS)
}

func (g *Game) add(r region) { g.regs = append(g.regs, r) }

// hit devolve a região mais de cima sob o cursor que satisfaz o filtro
func (g *Game) hit(ok func(r *region) bool) *region {
	for i := len(g.regs) - 1; i >= 0; i-- {
		r := &g.regs[i]
		if !r.has(g.mx, g.my) {
			continue
		}
		if ok(r) {
			return r
		}
		if r.block {
			return nil
		}
	}
	return nil
}

func (g *Game) playing() bool {
	return g.screen == "game" && g.S.Phase == "play" && g.modal == nil && !g.lockedOut()
}

var fotoT0 time.Time
var fotoStep int

func (g *Game) Update() error {
	g.frame++
	if f := os.Getenv("MESA_FOTO"); f != "" { // só para tirar a foto de divulgação: MESA_FOTO=deck|arquivo.png
		dn, out, _ := strings.Cut(f, "|")
		rede := os.Getenv("MESA_FOTO_REDE") // "host:Nome" ou "join:Nome": foto da mesa com vários jogadores
		if rede != "" {
			if g.frame == 1 {
				fotoT0 = time.Now()
			}
			el := time.Since(fotoT0)
			switch {
			case el > 30*time.Second && fotoStep == 0:
				fotoStep = 1
				if strings.HasPrefix(rede, "host") {
					g.setMode(4)
				}
			case el > 33*time.Second && fotoStep == 1:
				fotoStep = 2
				for _, id := range g.S.Z["battlefield"] {
					if g.S.Cards[id].Cmdr {
						g.pv = g.card(g.S.Cards[id])
					}
				}
				if out != "" {
					g.shot = out
				}
			case el > 36*time.Second && fotoStep == 2 && strings.HasPrefix(rede, "host"):
				return ebiten.Termination
			case el > 45*time.Second:
				return ebiten.Termination
			}
		}
		switch g.frame {
		case 2:
			if rede != "" {
				mode, nome, _ := strings.Cut(rede, ":")
				if mode == "host" {
					nw.host(nome)
				} else {
					nw.join(nome, "127.0.0.1")
				}
			}
			g.reloadDecks(dn)
			g.startGame(g.decks[g.sel])
			g.keep()
			g.confirmBottom()
			for _, z := range []string{"hand", "hand", "hand", "library", "library", "library", "library"} {
				for _, id := range g.S.Z[z] {
					c := g.card(g.S.Cards[id])
					land := strings.Contains(c.typeLine(), "Land")
					if (z == "hand") == land || z == "library" {
						g.move(id, "battlefield", moveOpt{})
						break
					}
				}
			}
			if c := g.S.Z["command"]; len(c) > 0 {
				g.move(c[0], "battlefield", moveOpt{})
				g.pv = g.card(g.S.Cards[c[0]])
			}
			g.nextTurn()
			g.nextTurn()
		case 240:
			for _, id := range g.S.Z["battlefield"] {
				if g.S.Cards[id].Cmdr {
					g.pv = g.card(g.S.Cards[id])
				}
			}
		case 245:
			if rede == "" {
				g.shot = out
			}
		case 250:
			if rede != "" {
				break
			}
			return ebiten.Termination
		}
	}
	if os.Getenv("MESA_EXPORTAR") != "" && g.frame == 2 { // só para gerar os decks de exemplo do PSP
		if os.Getenv("MESA_ARTES") != "" { // baixa as ilustrações (como o botão "Baixar só as ilustrações")
			cfg.Table = "moldura"
			saveCfg()
			cl := &http.Client{Timeout: 25 * time.Second}
			os.MkdirAll(filepath.Dir(artPath("x:0")), 0o755)
			for _, d := range g.decks {
				for _, k := range d.imageKeys() {
					if !hasArt(k) {
						if err := fetchArt(cl, k); err != nil {
							fmt.Println("arte falhou:", k, err)
						}
						time.Sleep(100 * time.Millisecond)
					}
				}
			}
		}
		for _, d := range g.decks {
			dir, n, err := g.exportPSP(d)
			fmt.Println("exportado:", dir, n, err)
		}
		return ebiten.Termination
	}
	cx, cy := ebiten.CursorPosition()
	g.mx, g.my = int(float64(cx)/uiS), int(float64(cy)/uiS)
	lp, ld, rp := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft), ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft), inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	if g.test != nil {
		if err := g.test.step(g); err != nil {
			return err
		}
		if g.test.fake { // entrada simulada pelo roteiro de teste
			g.mx, g.my = g.test.mx, g.test.my
			lp, ld, rp = g.test.ld && !g.test.pld, g.test.ld, g.test.rp
			g.test.pld, g.test.rp = g.test.ld, false
		}
	}
	g.importDropped()
	if artDirty.Swap(false) {
		g.clearProxies()
		g.imgTick = 0
	}
	if g.S != nil {
		g.syncAtt()
	}
	if g.bannerT > 0 {
		g.bannerT--
	}
	g.netSync()
	// texto digitado
	if g.focus != "" {
		s := g.fields[g.focus]
		s += string(ebiten.AppendInputChars(nil))
		if d := inpututil.KeyPressDuration(ebiten.KeyBackspace); d == 1 || (d > 25 && d%3 == 0) {
			if r := []rune(s); len(r) > 0 {
				s = string(r[:len(r)-1])
			}
		}
		g.fields[g.focus] = s
		if (inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter)) && g.onEnter != nil {
			g.onEnter()
		}
	}
	if _, wy := ebiten.Wheel(); wy != 0 {
		if r := g.hit(func(r *region) bool { return r.wheel != nil }); r != nil {
			r.wheel(wy)
		}
	}
	if lp {
		if r := g.hit(func(r *region) bool { return r.click != nil || r.inst != 0 }); r != nil {
			if r.inst != 0 && g.denied() {
				// tranca: a vez é de outro jogador
			} else if r.inst != 0 {
				g.drag.on, g.drag.moved, g.drag.id = true, false, r.inst
				g.drag.sx, g.drag.sy = g.mx, g.my
				g.drag.ox = min(g.cw, max(0, float64(g.mx)-r.x))
				g.drag.oy = min(g.cw*ratio, max(0, float64(g.my)-r.y))
			} else {
				r.click()
			}
		} else {
			g.focus = ""
		}
	}
	if g.drag.on {
		dx, dy := g.mx-g.drag.sx, g.my-g.drag.sy
		if !g.drag.moved && dx*dx+dy*dy > 36 && g.S != nil && g.S.Phase == "play" {
			g.drag.moved = true
		}
		if !ld {
			g.drag.on = false
			if g.S != nil {
				if g.drag.moved {
					g.drop()
				} else {
					g.clickCard(g.drag.id)
				}
			}
		}
	}
	if rp {
		hadCtx := g.ctx != nil
		g.ctx = nil
		if r := g.hit(func(r *region) bool { return r.rclick != nil }); r != nil && !(hadCtx && r.block) {
			r.rclick()
		}
	}
	// carta sob o cursor vai para a prévia
	g.pvNow = false
	if !g.drag.moved {
		if r := g.hit(func(r *region) bool { return r.pv != nil }); r != nil {
			g.pvNow = true
			g.pv, g.pvFace, g.pvHide = r.pv, r.pvFace, r.pvHidden
		}
	}
	if g.screen == "edit" && g.modal == nil && (ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)) && inpututil.IsKeyJustPressed(ebiten.KeyV) {
		g.edPaste()
	}
	g.keys()
	return nil
}

func (g *Game) keys() {
	jp := inpututil.IsKeyJustPressed
	if g.rebind != "" { // trocando um atalho: a próxima tecla vira o atalho
		for _, k := range inpututil.AppendJustPressedKeys(nil) {
			if k != ebiten.KeyEscape {
				setKey(g.rebind, k)
			}
			g.rebind = ""
			break
		}
		return
	}
	if jp(ebiten.KeyEscape) {
		if g.zoom != nil {
			g.zoom = nil
		} else if g.au.pick {
			g.au.pick = false
		} else if g.equip != 0 {
			g.equip = 0
		} else if g.ctx != nil {
			g.ctx = nil
		} else if g.modal != nil {
			g.closeModal()
		}
		return
	}
	if g.focus == "" && !ebiten.IsKeyPressed(ebiten.KeyControl) && !ebiten.IsKeyPressed(ebiten.KeyMeta) && (jpA("zoom") || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle)) {
		g.zoomHover()
		return
	}
	if g.focus == "" && jp(ebiten.KeyF1) {
		g.shortcutsModal()
		return
	}
	if g.screen != "game" || g.focus != "" || g.zoom != nil {
		return
	}
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	if ctrl && jp(ebiten.KeyZ) {
		g.undoLast()
		return
	}
	if ctrl || !g.playing() {
		return
	}
	// mouse sobre uma carta + tecla: move sem abrir o menu
	if z, lab := hoverMoveKey(); z != "" {
		if r := g.hit(func(r *region) bool { return r.inst != 0 }); r != nil && g.S.Phase == "play" && !g.denied() {
			if i := g.S.Cards[r.inst]; i != nil && !(i.Zone == z && !strings.HasPrefix(lab, "fundo")) {
				g.snap()
				g.move(r.inst, z, moveOpt{bottom: lab == "fundo"})
			}
		}
		return
	}
	switch {
	case jp(ebiten.KeyDigit1):
		g.setMode(1)
	case jp(ebiten.KeyDigit2):
		g.setMode(2)
	case jp(ebiten.KeyDigit3):
		g.setMode(3)
	case jp(ebiten.KeyDigit4):
		g.setMode(4)
	case jp(ebiten.KeyTab):
		g.cyclePeer()
	case jp(ebiten.KeyBracketLeft), jp(ebiten.KeyBracketRight):
		d := 8.0
		if jp(ebiten.KeyBracketLeft) {
			d = -8
		}
		g.cw = max(70, min(190, g.cw+d))
		cfg.Size = g.cw
		saveCfg()
	case jpA("painel"):
		g.togglePanel()
	case jpA("estilo"):
		g.cycleTable()
	case jpA("comprar"):
		g.drawN(1)
	case jpA("turno"):
		g.nextTurn()
	case jpA("fase"), jp(ebiten.KeySpace):
		g.nextStep()
	case jpA("desvirar"):
		g.untapAll()
	case jpA("embaralhar"):
		g.shuffleLib()
	case jpA("ficha"):
		g.tokenModal()
	}
}

func (g *Game) importDropped() {
	fsys := ebiten.DroppedFiles()
	if fsys == nil || (g.screen != "home" && g.screen != "edit") || g.modal != nil {
		return
	}
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil || len(b) > 300000 {
			return nil
		}
		if g.screen != "edit" { // o arquivo abre no editor para conferir antes de salvar
			g.openEditor(nil, "")
			g.fields["edname"] = baseName(p)
		}
		g.edAddText(string(b))
		return nil
	})
}

func (g *Game) reloadDecks(selName string) {
	g.decks = loadDecks()
	for i, d := range g.decks {
		if d.Name == selName {
			g.sel = i
		}
	}
	if g.sel >= len(g.decks) {
		g.sel = 0
	}
	g.imgTick = 0
}

func (g *Game) clickCard(id int) {
	s := g.S
	i := s.Cards[id]
	if i == nil {
		return
	}
	lim := 400 * time.Millisecond
	if g.test != nil {
		lim = 30 * time.Second // a tela virtual do teste é lenta
	}
	dbl := g.lastID == id && time.Since(g.lastT) < lim
	g.lastID, g.lastT = id, time.Now()
	if s.Phase == "bottom" {
		if i.Zone == "hand" {
			if contains(s.Pick, id) {
				s.Pick = remove(s.Pick, id)
			} else if len(s.Pick) < g.bottomCount() {
				s.Pick = append(s.Pick, id)
			}
		}
		return
	}
	if s.Phase != "play" {
		return
	}
	if g.au.pick { // escolhendo o alvo de uma seta
		if i.Zone == "battlefield" && id != g.au.src {
			g.au.pick = false
			g.addArrow(Arrow{From: g.au.src, To: id}, g.card(i).N)
		}
		return
	}
	if g.equip != 0 { // escolhendo o alvo de "equipar/anexar"
		src := g.equip
		if i.Zone == "battlefield" && id != src {
			g.equip = 0
			g.attach(src, id)
		}
		return
	}
	switch i.Zone {
	case "battlefield":
		g.snap()
		i.Tapped = !i.Tapped
	case "library":
		g.drawN(1)
	case "graveyard", "exile":
		g.viewZone(i.Zone, false)
	case "hand", "command":
		if dbl {
			g.lastID = 0
			g.snap()
			g.move(id, "battlefield", moveOpt{})
		}
	}
}

func (g *Game) drop() {
	r := g.hit(func(r *region) bool { return r.zone != "" })
	i := g.S.Cards[g.drag.id]
	if r == nil || i == nil {
		return
	}
	id, zone := i.ID, r.zone
	if zone == i.Zone && zone != "battlefield" && zone != "hand" {
		return
	}
	g.snap()
	switch {
	case zone == "battlefield":
		x := float64(g.mx) - float64(g.bf.Min.X) - g.drag.ox
		y := float64(g.my) - float64(g.bf.Min.Y) - g.drag.oy
		x = max(0, min(float64(g.bf.Dx())-g.cw*0.5, x))
		y = max(0, min(float64(g.bf.Dy())-g.cw*0.6, y))
		x, y = g.snapZone(x, y)
		g.move(id, zone, moveOpt{pos: true, x: x, y: y})
	case zone == "hand" && i.Zone == "hand":
		h := g.S.Z["hand"]
		idx := len(h)
		for k, hx := range g.handPos {
			if k < len(h) && float64(g.mx) < hx+g.cw/2 {
				idx = k
				break
			}
		}
		var out []int
		for k, x := range h {
			if k == idx {
				out = append(out, id)
			}
			if x != id {
				out = append(out, x)
			}
		}
		if idx >= len(h) {
			out = append(out, id)
		}
		g.S.Z["hand"] = out
	default:
		g.move(id, zone, moveOpt{bottom: ebiten.IsKeyPressed(ebiten.KeyShift)})
	}
}

/* ====================== componentes ====================== */

func (g *Game) hover(x, y, w, h float64) bool {
	return float64(g.mx) >= x && float64(g.mx) < x+w && float64(g.my) >= y && float64(g.my) < y+h
}

// button desenha um botão e devolve a largura usada. style: 0 normal, 1 principal, 2 perigo, 3 desligado
func (g *Game) button(x, y, w, h float64, label string, style int, fn func()) float64 {
	size := 14.0
	if h < 24 {
		size = 12
	}
	if w == 0 {
		w = tw(label, fReg, size) + 22
	}
	bg, border, fg := color.Color(cPanel2), color.Color(cLine), color.Color(cTxt)
	src := fReg
	if style == 1 {
		bg, border, fg, src = cAcc, cAcc, color.RGBA{0x1b, 0x16, 0x08, 0xff}, fBold
	}
	if style == 3 {
		fg = cMut
	} else if g.hover(x, y, w, h) && g.topAt(x, y, w, h) {
		border = cAcc
		if style == 2 {
			border, fg = cBad, cBad
		}
	}
	rrect(g.dst, x, y, w, h, 6, border)
	rrect(g.dst, x+1, y+1, w-2, h-2, 5, bg)
	txtC(g.dst, fit(label, src, size, w-6), x+w/2, y+(h-size*1.25)/2, src, size, fg)
	if style != 3 && fn != nil {
		g.add(region{x: x, y: y, w: w, h: h, click: fn})
	}
	return w
}

// topAt evita destacar botões que estão atrás de uma janela
func (g *Game) topAt(x, y, w, h float64) bool {
	if g.ctx != nil {
		return g.inCtx
	}
	if g.modal != nil {
		return g.inModal
	}
	return true
}

func (g *Game) field(id string, x, y, w float64, placeholder string) {
	h := 30.0
	border := color.Color(cLine)
	if g.focus == id {
		border = cAcc
	}
	rrect(g.dst, x, y, w, h, 6, border)
	rrect(g.dst, x+1, y+1, w-2, h-2, 5, cDark)
	s := g.fields[id]
	if s == "" && g.focus != id {
		txt(g.dst, fit(placeholder, fReg, 13, w-14), x+8, y+7, fReg, 13, cMut)
	} else {
		if g.focus == id && (g.frame/30)%2 == 0 {
			s += "|"
		}
		txt(g.dst, s, x+8, y+6, fReg, 14, cTxt)
	}
	g.add(region{x: x, y: y, w: w, h: h, click: func() { g.focus = id }})
}

// drawInst desenha uma carta em jogo com contadores e marcas
func (g *Game) drawInst(i *Inst, x, y, w float64, forceDown, forceUp bool, alpha float32) {
	c := g.card(i)
	tex := g.cardTex(c, i.Face)
	if forceDown || (i.Down && !forceUp) {
		tex = g.backTex()
	}
	tapped := i.Tapped && i.Zone == "battlefield" && !forceUp
	drawTex(g.dst, tex, x, y, w, tapped, alpha)
	h := w * ratio
	bx, by := x, y
	if tapped {
		bx, by = x+w/2-h/2, y+h/2-w/2
	}
	cy := by + 6
	if i.Cmdr && g.S.Tax[i.Cid] > 0 {
		cy = by + 24
	}
	if !tapped {
		cy = y + h*0.30
	}
	keys := make([]string, 0, len(i.Counters))
	for k, v := range i.Counters {
		if v != 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := fmt.Sprintf("%d %s", i.Counters[k], k)
		if k == "Contador" {
			s = strconv.Itoa(i.Counters[k])
		}
		bw := tw(s, fBold, 12) + 12
		btn := g.au.cnt && i.ID != 0 // no meu campo: − e + para mudar o contador com um clique
		tx := bx + 9
		if btn {
			bw += 36
			tx += 18
		}
		rrect(g.dst, bx+3, cy, bw, 19, 9, cAcc)
		rrect(g.dst, bx+4, cy+1, bw-2, 17, 8, color.RGBA{0, 0, 0, 0xe6})
		txt(g.dst, s, tx, cy+2, fBold, 12, color.White)
		if btn {
			txtC(g.dst, "−", bx+13, cy+1.5, fBold, 13, cAcc)
			txtC(g.dst, "+", bx+bw-7, cy+1.5, fBold, 13, cAcc)
			id := i.ID
			g.au.btns = append(g.au.btns,
				region{x: bx + 23, y: cy, w: bw - 40, h: 19, click: func() { g.bump(id, k, 1) }, rclick: func() { g.bump(id, k, -1) }}, // corpo: clique +1, direito -1
				region{x: bx + 3, y: cy, w: 20, h: 19, click: func() { g.bump(id, k, -1) }},
				region{x: bx + 3 + bw - 20, y: cy, w: 20, h: 19, click: func() { g.bump(id, k, 1) }})
		}
		cy += 21
	}
	if g.au.cnt && i.ID != 0 { // poder/resistência calculado (minhas cartas no campo)
		if s, ok := g.autoPT(i); ok {
			pw2 := tw(s, fBold, 13.5) + 16
			px, py := x+w-pw2-3, y+h-24
			if tapped {
				px, py = bx+h-pw2-3, by+w-24
			}
			rrect(g.dst, px, py, pw2, 21, 8, cAcc2)
			rrect(g.dst, px+1.5, py+1.5, pw2-3, 18, 7, color.RGBA{0x06, 0x12, 0x10, 0xf2})
			txtC(g.dst, s, px+pw2/2, py+2.5, fBold, 13.5, color.White)
		}
	}
	if i.Token && !tapped {
		rrect(g.dst, x+3, y+h-17, 40, 14, 3, color.RGBA{0, 0, 0, 0xcc})
		txt(g.dst, "FICHA", x+7, y+h-16, fBold, 9.5, cAcc2)
	}
	if i.Cmdr && g.S.Tax[i.Cid] > 0 {
		s := fmt.Sprintf("imposto +%d", g.S.Tax[i.Cid]*2)
		bw := tw(s, fBold, 11) + 10
		rrect(g.dst, bx+3, by+3, bw, 17, 4, cAcc)
		rrect(g.dst, bx+4, by+4, bw-2, 15, 3, color.RGBA{0, 0, 0, 0xe6})
		txt(g.dst, s, bx+8, by+4.5, fBold, 11, cAcc)
	}
}

/* ====================== janelas ====================== */

func (g *Game) openModal(m *Modal) { g.modal, g.ctx = m, nil }
func (g *Game) closeModal() {
	m := g.modal
	g.modal, g.focus, g.onEnter = nil, "", nil
	if m != nil && m.onClose != nil {
		m.onClose()
	}
}
func (g *Game) drawModal() {
	m := g.modal
	W, H := float64(g.W), float64(g.H)
	left := 0.0
	if g.screen == "game" && m.w > 600 {
		left = 250 // deixa a prévia da carta visível ao lado das janelas grandes
	}
	rect(g.dst, left, 0, W-left, H, color.RGBA{0, 0, 0, 0xb0})
	g.add(region{x: 0, y: 0, w: W, h: H, block: true, click: func() {}, rclick: func() {}, wheel: func(float64) {}})
	w, h := min(m.w, W-left-24), min(m.h, H-30)
	x, y := left+(W-left-w)/2, (H-h)/2
	rrect(g.dst, x-1, y-1, w+2, h+2, 12, cLine)
	rrect(g.dst, x, y, w, h, 11, cPanel)
	g.add(region{x: x, y: y, w: w, h: h, block: true, click: func() { g.focus = "" }, rclick: func() {}})
	g.inModal = true
	txt(g.dst, fit(m.title, fBold, 15, w-70), x+14, y+11, fBold, 15, cTxt)
	g.button(x+w-38, y+8, 28, 24, "X", 0, g.closeModal)
	rect(g.dst, x, y+40, w, 1, cLine)
	m.draw(x+14, y+52, w-28, h-64)
	g.inModal = false
}

func (g *Game) ask(title, def string, fn func(string)) {
	g.fields["ask"] = def
	g.focus = "ask"
	ok := func() {
		v := strings.TrimSpace(g.fields["ask"])
		g.closeModal()
		if v != "" {
			fn(v)
		}
	}
	g.onEnter = ok
	g.openModal(&Modal{title: title, w: 420, h: 150, draw: func(x, y, w, h float64) {
		g.field("ask", x, y, w, "")
		g.button(x+w-90, y+44, 90, 30, "OK", 1, ok)
		g.button(x+w-190, y+44, 92, 30, "Cancelar", 0, g.closeModal)
	}})
}
func (g *Game) askN(title, def string, fn func(int)) {
	g.ask(title, def, func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			fn(min(n, 200))
		}
	})
}
func (g *Game) confirm(q string, fn func()) {
	g.openModal(&Modal{title: "Confirmar", w: 440, h: 150, draw: func(x, y, w, h float64) {
		txt(g.dst, strings.Join(wrap(q, fReg, 14, w), "\n"), x, y, fReg, 14, cTxt)
		g.button(x+w-90, y+48, 90, 30, "Sim", 1, func() { g.closeModal(); fn() })
		g.button(x+w-190, y+48, 92, 30, "Cancelar", 0, g.closeModal)
	}})
}

type cellBtn struct {
	label string
	fn    func()
}

// grid desenha cartas em grade rolável, cada uma com botões embaixo
func (g *Game) grid(m *Modal, x, y, w, h float64, n int, rows int, cell func(k int, cx, cy, cw float64) []cellBtn) {
	cw := min(g.cw, 130)
	ch := cw * ratio
	cellW, cellH := cw+34, ch+10+float64(rows)*22
	per := max(1, int(w/cellW))
	total := float64((n+per-1)/per) * cellH
	m.scroll = max(0, min(m.scroll, total-h))
	g.add(region{x: x, y: y, w: w, h: h, wheel: func(dy float64) { m.scroll -= dy * 70 }})
	sub := clip(g.dst, x, y, w, h)
	old := g.dst
	for k := 0; k < n; k++ {
		cx := x + float64(k%per)*cellW
		cy := y + float64(k/per)*cellH - m.scroll
		if cy+cellH < y || cy > y+h {
			continue
		}
		g.dst = sub
		btns := cell(k, cx+17, cy, cw)
		bx, by := cx, cy+ch+5
		for _, b := range btns {
			bw := tw(b.label, fReg, 12) + 12
			if bx+bw > cx+cellW-2 {
				bx, by = cx, by+22
			}
			if by >= y && by+20 <= y+h {
				g.button(bx, by, bw, 20, b.label, 0, b.fn)
			}
			bx += bw + 2
		}
		g.dst = old
	}
	if total > h { // barra de rolagem
		bh := h * h / total
		rrect(g.dst, x+w+4, y+(h-bh)*m.scroll/(total-h), 5, bh, 2, cLine)
	}
}

func (g *Game) moveBtns(id int) []cellBtn {
	i := g.S.Cards[id]
	if i == nil {
		return nil
	}
	mk := func(l, z string, o moveOpt, always bool) *cellBtn {
		if i.Zone == z && !always {
			return nil
		}
		return &cellBtn{l, func() { g.snap(); g.move(id, z, o) }}
	}
	var out []cellBtn
	for _, b := range []*cellBtn{mk("Mão", "hand", moveOpt{}, false), mk("Campo", "battlefield", moveOpt{}, false), mk("Cemit.", "graveyard", moveOpt{}, false),
		mk("Exílio", "exile", moveOpt{}, false), mk("Topo", "library", moveOpt{}, true), mk("Fundo", "library", moveOpt{bottom: true}, true)} {
		if b != nil {
			out = append(out, *b)
		}
	}
	if i.Cmdr && i.Zone != "command" {
		out = append(out, cellBtn{"Comando", func() { g.snap(); g.move(id, "command", moveOpt{}) }})
	}
	return out
}

func (g *Game) cellCard(id int, cx, cy, cw float64) {
	i := g.S.Cards[id]
	g.drawInst(i, cx, cy, cw, false, true, 1)
	c, fi := g.card(i), i.Face
	g.add(region{x: cx, y: cy, w: cw, h: cw * ratio, pv: c, pvFace: fi, click: func() { g.openZoom(c, fi) }})
}

func (g *Game) viewZone(zone string, search bool) {
	shuf := search
	title := zoneName[zone]
	if search {
		title = "Buscar no grimório"
	}
	m := &Modal{title: title, w: 1180, h: 760}
	flt := &zoneFilter{cmc: -1, cols: map[string]bool{}}
	g.zf = flt
	g.fields["vzq"] = ""
	m.draw = func(x, y, w, h float64) {
		ids := append([]int{}, g.S.Z[zone]...)
		total := len(ids)
		if search {
			sort.SliceStable(ids, func(a, b int) bool { return g.card(g.S.Cards[ids[a]]).N < g.card(g.S.Cards[ids[b]]).N })
		} else if zone != "library" {
			for a, b := 0, len(ids)-1; a < b; a, b = a+1, b-1 {
				ids[a], ids[b] = ids[b], ids[a]
			}
		}
		top := 0.0
		if search || zone != "library" { // filtros: nome, tipo, custo e cor
			top = flt.bar(g, m, x, y, w)
			keep := ids[:0]
			for _, id := range ids {
				if flt.match(g.card(g.S.Cards[id]), strings.TrimSpace(g.fields["vzq"])) {
					keep = append(keep, id)
				}
			}
			ids = keep
			if flt.byCMC {
				sort.SliceStable(ids, func(a, b int) bool {
					return manaValue(g.card(g.S.Cards[ids[a]]).FaceAt(0).C) < manaValue(g.card(g.S.Cards[ids[b]]).FaceAt(0).C)
				})
			}
		}
		if len(ids) == 0 {
			msg := "Vazio."
			if total > 0 {
				msg = "Nenhuma carta com esses filtros."
			}
			txt(g.dst, msg, x, y+top, fReg, 14, cMut)
		}
		g.grid(m, x, y+top, w-8, h-44-top, len(ids), 2, func(k int, cx, cy, cw float64) []cellBtn {
			g.cellCard(ids[k], cx, cy, cw)
			return g.moveBtns(ids[k])
		})
		if search {
			lab := "[  ] Embaralhar ao fechar"
			if shuf {
				lab = "[x] Embaralhar ao fechar"
			}
			g.button(x, y+h-32, 0, 30, lab, 0, func() { shuf = !shuf })
		}
		cnt := fmt.Sprintf("%d cartas", len(ids))
		if len(ids) != total {
			cnt = fmt.Sprintf("%d de %d cartas", len(ids), total)
		}
		txt(g.dst, cnt, x+w-110-tw(cnt, fReg, 13), y+h-25, fReg, 13, cMut)
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}
	m.onClose = func() {
		if search && shuf {
			shuffle(g.S.Z["library"])
			g.log("embaralhou o grimório")
		}
	}
	g.openModal(m)
}

func (g *Game) scry(n int) {
	lib := g.S.Z["library"]
	n = min(n, len(lib))
	if n == 0 {
		return
	}
	ids := append([]int{}, lib[:n]...)
	dest := make([]int, n)
	names := []string{"Topo", "Fundo", "Cemitério", "Mão", "Exílio"}
	zones := []string{"", "", "graveyard", "hand", "exile"}
	m := &Modal{title: fmt.Sprintf("Topo do grimório (%d): a da esquerda fica mais em cima", n), w: 1180, h: 640}
	m.draw = func(x, y, w, h float64) {
		g.grid(m, x, y, w-8, h-44, n, 1, func(k int, cx, cy, cw float64) []cellBtn {
			g.cellCard(ids[k], cx, cy, cw)
			b := []cellBtn{}
			if k > 0 {
				b = append(b, cellBtn{"<", func() { ids[k-1], ids[k] = ids[k], ids[k-1]; dest[k-1], dest[k] = dest[k], dest[k-1] }})
			}
			b = append(b, cellBtn{"vai: " + names[dest[k]], func() { dest[k] = (dest[k] + 1) % len(names) }})
			if k < n-1 {
				b = append(b, cellBtn{">", func() { ids[k+1], ids[k] = ids[k], ids[k+1]; dest[k+1], dest[k] = dest[k], dest[k+1] }})
			}
			return b
		})
		txt(g.dst, "Clique em “vai:” para trocar o destino de cada carta.", x, y+h-25, fReg, 13, cMut)
		g.button(x+w-110, y+h-32, 110, 30, "Confirmar", 1, func() {
			g.snap()
			var tops, bots, rest []int
			for k, id := range ids {
				switch dest[k] {
				case 0:
					tops = append(tops, id)
				case 1:
					bots = append(bots, id)
				default:
					rest = append(rest, id)
				}
			}
			l := g.S.Z["library"]
			for _, id := range ids {
				l = remove(l, id)
			}
			g.S.Z["library"] = append(append(tops, l...), bots...)
			for k, id := range ids {
				if dest[k] >= 2 {
					g.S.Z["library"] = append([]int{id}, g.S.Z["library"]...)
					g.move(id, zones[dest[k]], moveOpt{})
				}
			}
			_ = rest
			g.log("olhou %d do topo: %d no topo, %d no fundo", n, len(tops), len(bots))
			g.closeModal()
		})
	}
	g.openModal(m)
}

func (g *Game) tokenModal() {
	for _, k := range []string{"tn", "tt", "tp", "tx"} {
		g.fields[k] = ""
	}
	m := &Modal{title: "Fichas", w: 1100, h: 700}
	create := func() {
		name := strings.TrimSpace(g.fields["tn"])
		if name == "" {
			name = "Ficha"
		}
		typ := strings.TrimSpace(g.fields["tt"])
		if typ == "" {
			typ = "Ficha"
		}
		p, o, _ := strings.Cut(strings.TrimSpace(g.fields["tp"]), "/")
		id := "custom:" + name + "|" + typ + "|" + p + "/" + o + "|" + g.fields["tx"]
		if g.customs[id] == nil {
			g.customs[id] = &Card{ID: id, Custom: true, Face: Face{N: name, T: typ, X: g.fields["tx"], P: strings.TrimSpace(p), O: strings.TrimSpace(o)}}
		}
		g.addToken(id, 1)
		g.closeModal()
	}
	g.onEnter = create
	m.draw = func(x, y, w, h float64) {
		var cs []*Card
		for _, id := range g.deck.Tokens {
			if c := cardByID[id]; c != nil {
				cs = append(cs, c)
			}
		}
		var ck []string
		for k := range g.customs {
			ck = append(ck, k)
		}
		sort.Strings(ck)
		for _, k := range ck {
			cs = append(cs, g.customs[k])
		}
		if len(cs) == 0 {
			txt(g.dst, "Este deck não cria fichas conhecidas. Crie uma personalizada abaixo.", x, y, fReg, 14, cMut)
		}
		g.grid(m, x, y, w-8, h-110, len(cs), 1, func(k int, cx, cy, cw float64) []cellBtn {
			c := cs[k]
			drawTex(g.dst, g.cardTex(c, 0), cx, cy, cw, false, 1)
			g.add(region{x: cx, y: cy, w: cw, h: cw * ratio, pv: c})
			return []cellBtn{{"+1", func() { g.addToken(c.ID, 1) }}, {"+2", func() { g.addToken(c.ID, 2) }}, {"+5", func() { g.addToken(c.ID, 5) }}}
		})
		fy := y + h - 96
		rect(g.dst, x, fy, w, 1, cLine)
		txt(g.dst, "Ficha personalizada", x, fy+8, fBold, 14, cTxt)
		g.field("tn", x, fy+32, 190, "Nome (ex.: Tesouro)")
		g.field("tt", x+198, fy+32, 250, "Tipo (ex.: Criatura — Espírito)")
		g.field("tp", x+456, fy+32, 110, "P/R (ex.: 1/1)")
		g.field("tx", x+574, fy+32, w-574-200, "Texto (opcional)")
		g.button(x+w-190, fy+32, 90, 30, "Criar", 1, create)
		g.button(x+w-92, fy+32, 92, 30, "Fechar", 0, g.closeModal)
		txt(g.dst, "Fichas somem quando saem do campo.", x, fy+72, fReg, 12, cMut)
	}
	g.openModal(m)
}

func (g *Game) diceModal() {
	res := "—"
	roll := func(label string, f func() string) func() {
		return func() { res = f(); g.snap(); g.log("%s: %s", label, res) }
	}
	g.openModal(&Modal{title: "Dado / moeda", w: 380, h: 210, draw: func(x, y, w, h float64) {
		txtC(g.dst, res, x+w/2, y+10, fBold, 34, cAcc)
		g.button(x+20, y+84, 100, 34, "Moeda", 0, roll("Moeda", func() string {
			if rnd(2) == 0 {
				return "Cara"
			}
			return "Coroa"
		}))
		g.button(x+128, y+84, 90, 34, "d6", 0, roll("d6", func() string { return strconv.Itoa(rnd(6) + 1) }))
		g.button(x+226, y+84, 90, 34, "d20", 0, roll("d20", func() string { return strconv.Itoa(rnd(20) + 1) }))
	}})
}

/* ====================== menus ====================== */

func (g *Game) openCtx(items []item) {
	g.ctx = &Ctx{x: float64(g.mx), y: float64(g.my), items: items}
}
func (g *Game) drawCtx() {
	c := g.ctx
	W, H := float64(g.W), float64(g.H)
	const w, rh = 270.0, 25.0
	h := 8.0
	for _, it := range c.items {
		if it.hd != "" {
			h += 22
		} else {
			h += rh
		}
	}
	x, y := max(4, min(c.x, W-w-6)), max(4, min(c.y, H-h-6))
	g.add(region{x: 0, y: 0, w: W, h: H, block: true, click: func() { g.ctx = nil }, rclick: func() {}})
	rrect(g.dst, x-1, y-1, w+2, h+2, 8, cLine)
	rrect(g.dst, x, y, w, h, 7, cPanel)
	g.add(region{x: x, y: y, w: w, h: h, block: true, click: func() {}})
	g.inCtx = true
	cy := y + 4
	for _, it := range c.items {
		it := it
		if it.hd != "" {
			txt(g.dst, strings.ToUpper(fit(it.hd, fBold, 10.5, w-24)), x+10, cy+7, fBold, 10.5, cMut)
			cy += 22
			continue
		}
		if g.hover(x+3, cy, w-6, rh) {
			rrect(g.dst, x+3, cy, w-6, rh, 5, cPanel2)
		}
		txt(g.dst, fit(it.label, fReg, 13.5, w-90), x+10, cy+4, fReg, 13.5, cTxt)
		if it.pm != nil {
			g.button(x+w-78, cy+2, 22, 21, "-", 0, func() { it.pm(-1) })
			txtC(g.dst, strconv.Itoa(it.val()), x+w-43, cy+4, fBold, 13.5, cTxt)
			g.button(x+w-30, cy+2, 22, 21, "+", 0, func() { it.pm(1) })
		} else {
			if it.key != "" {
				txt(g.dst, it.key, x+w-12-tw(it.key, fMono, 11), cy+6, fMono, 11, cMut)
			}
			g.add(region{x: x + 3, y: cy, w: w - 6, h: rh, click: func() { g.ctx = nil; it.fn() }})
		}
		cy += rh
	}
	g.inCtx = false
}

func (g *Game) libMenu() []item {
	return []item{{hd: "Grimório"},
		{label: "Comprar 1", key: "D", fn: func() { g.drawN(1) }},
		{label: "Comprar X…", fn: func() { g.askN("Comprar quantas cartas?", "2", g.drawN) }},
		{label: "Olhar o topo X (vidência)…", fn: func() { g.askN("Olhar quantas cartas do topo?", "1", g.scry) }},
		{label: "Buscar no grimório", fn: func() { g.viewZone("library", true) }},
		{label: "Moer X (topo → cemitério)…", fn: func() {
			g.askN("Moer quantas cartas?", "1", func(n int) {
				g.snap()
				for k := 0; k < n && len(g.S.Z["library"]) > 0; k++ {
					g.move(g.S.Z["library"][0], "graveyard", moveOpt{})
				}
			})
		}},
		{label: "Revelar o topo", fn: func() {
			if l := g.S.Z["library"]; len(l) > 0 {
				g.reveal(l[0])
			}
		}},
		{label: "Embaralhar", key: "S", fn: g.shuffleLib}}
}
func (g *Game) reveal(id int) {
	m := &Modal{title: "Topo do grimório", w: 420, h: 360}
	m.draw = func(x, y, w, h float64) {
		if g.S.Cards[id] == nil || g.S.Cards[id].Zone != "library" {
			txt(g.dst, "A carta saiu do topo.", x, y, fReg, 14, cMut)
			return
		}
		g.grid(m, x, y, w-8, h, 1, 2, func(k int, cx, cy, cw float64) []cellBtn {
			g.cellCard(id, cx, cy, cw)
			return g.moveBtns(id)
		})
	}
	g.openModal(m)
}
func (g *Game) tableMenu() []item {
	return []item{{hd: "Mesa"},
		{label: "Criar ficha…", key: "T", fn: g.tokenModal},
		{label: "Desvirar tudo", key: "U", fn: g.untapAll},
		{label: "Próxima fase", key: "F", fn: g.nextStep},
		{label: "Próximo turno / passar a vez", key: "N", fn: g.nextTurn},
		{label: "Ordem da mesa, tranca e tempo (rede)…", fn: func() {
			if nw.active() {
				g.tableModal()
			}
		}},
		{label: "Arrumar o campo", fn: g.tidy},
		{label: "Apagar minhas setas", fn: func() { g.clearArrows(0) }},
		{label: "Meus marcadores (veneno, energia…)", fn: g.trackerMenu},
		{label: "Cartas na mesa: " + tableLabel() + " (trocar)", key: "E", fn: g.cycleTable},
		{label: "Poder automático nas cartas: " + simNao(!cfg.NoAuto), fn: func() { cfg.NoAuto = !cfg.NoAuto; saveCfg() }},
		{label: "Campo: " + fieldNames[fieldMode()] + " (trocar)", fn: g.cycleField},
		{label: "Encaixar cartas nas zonas: " + simNao(!cfg.NoSnap), fn: func() { cfg.NoSnap = !cfg.NoSnap; saveCfg() }},
		{label: "Esconder / mostrar o painel da esquerda", key: "P", fn: g.togglePanel},
		{label: "Minhas cartas (criar ou pôr na mão)…", fn: g.myModal},
		{label: "Fazer proxies (PNG para imprimir)…", fn: g.proxyModal},
		{label: "Revelar / esconder a mão (rede)", fn: func() {
			if nw.active() {
				g.toggleReveal()
			}
		}},
		{label: "Aparência (playmat e sleeve)…", fn: g.looksModal},
		{label: "Desfazer", key: "Ctrl+Z", fn: g.undoLast}}
}

func (g *Game) cardMenu(id int) {
	if g.S.Phase != "play" || g.denied() {
		return
	}
	i := g.S.Cards[id]
	if i == nil {
		return
	}
	if i.Zone == "library" {
		g.openCtx(g.libMenu())
		return
	}
	c := g.card(i)
	get := func() *Inst { return g.S.Cards[id] }
	title := c.N
	if i.Down {
		title = "Carta virada para baixo"
	}
	items := []item{{hd: title}}
	if !i.Down {
		items = append(items, item{label: "Ver a carta grande", key: keyName("zoom"), fn: func() { g.openZoom(c, get().Face); g.zoom.cnt = get().Counters }})
	}
	if i.Zone == "battlefield" {
		lab := "Virar"
		if i.Tapped {
			lab = "Desvirar"
		}
		items = append(items, item{label: lab, fn: func() { g.snap(); get().Tapped = !get().Tapped }})
		if n := len(c.Faces()); n > 1 {
			items = append(items, item{label: "Transformar (outra face)", fn: func() { g.snap(); get().Face = (get().Face + 1) % n }})
		}
		lab = "Virar a face para baixo"
		if i.Down {
			lab = "Virar a face para cima"
		}
		items = append(items, item{label: lab, fn: func() { g.snap(); get().Down = !get().Down }},
			item{label: "Equipar / anexar em…", fn: func() { g.equip = id }},
			item{label: "Apontar seta para…", fn: func() { g.startArrow(id) }})
		for _, a := range g.arrows {
			if a.From == id || (a.Peer == "" && a.To == id) {
				items = append(items, item{label: "Apagar as setas desta carta", fn: func() { g.clearArrows(id) }})
				break
			}
		}
		if i.Att != 0 {
			items = append(items, item{label: "Desanexar", fn: func() {
				g.snap()
				o := get()
				o.Att, o.X, o.Y = 0, o.X+g.cw*0.5, o.Y+g.cw*0.4
			}})
		}
		items = append(items,
			item{label: "Trazer para a frente", fn: func() { g.snap(); g.S.Z["battlefield"] = append(remove(g.S.Z["battlefield"], id), id) }},
			item{hd: "Contadores"})
		names := []string{"+1/+1", "-1/-1", "Contador"}
		if g.face(i).L != "" {
			names = append(names, "Lealdade")
		}
		var extra []string
		for k := range i.Counters {
			if !strings.Contains("|+1/+1|-1/-1|Contador|Lealdade|", "|"+k+"|") {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, k := range append(names, extra...) {
			k := k
			items = append(items, item{label: k, val: func() int {
				if o := get(); o != nil {
					return o.Counters[k]
				}
				return 0
			}, pm: func(d int) {
				o := get()
				if o == nil {
					return
				}
				g.snap()
				o = get()
				o.Counters[k] = max(0, o.Counters[k]+d)
				if o.Counters[k] == 0 {
					delete(o.Counters, k)
				}
			}})
		}
		items = append(items, item{label: "Outro tipo de contador…", fn: func() {
			g.ask("Nome do contador (ex.: carga, escudo)", "carga", func(k string) { g.snap(); get().Counters[k]++ })
		}}, item{hd: "Fichas"})
		for _, tid := range c.K {
			if t := cardByID[tid]; t != nil {
				tid := tid
				lab := "Criar " + t.N
				if f := t.FaceAt(0); f.P != "" {
					lab += " " + f.P + "/" + f.O
				}
				items = append(items, item{label: lab, fn: func() { g.addToken(tid, 1) }})
			}
		}
		items = append(items, item{label: "Criar cópia (ficha)", fn: func() {
			g.snap()
			o := get()
			t := g.mk(o.Cid, "battlefield")
			t.Token, t.Face, t.X, t.Y = true, o.Face, o.X+24, o.Y+24
			g.log("criou cópia de %s", c.N)
		}})
	}
	if i.Cmdr {
		cid := i.Cid
		items = append(items, item{hd: "Comandante"}, item{label: "Imposto (vezes já conjurado)", val: func() int { return g.S.Tax[cid] }, pm: func(d int) {
			g.snap()
			g.S.Tax[cid] = max(0, g.S.Tax[cid]+d)
		}})
	}
	if c := g.card(i); c != nil && !c.Custom {
		items = append(items, item{hd: "Imagem"}, item{label: "Escolher a arte… (internet)", fn: func() { g.artModal(c) }})
	}
	items = append(items, item{hd: "Mover para"})
	mvKeys := map[string]string{"Mão": keyName("mao"), "Cemitério": keyName("cemiterio"), "Exílio": keyName("exilio"), "Fundo do grimório": keyName("fundo")}
	mv := func(z, lab string, o moveOpt) {
		items = append(items, item{label: lab, key: mvKeys[lab], fn: func() { g.snap(); g.move(id, z, o) }})
	}
	if i.Zone != "battlefield" {
		mv("battlefield", "Campo de batalha", moveOpt{})
		items = append(items, item{label: "Campo, virada para baixo", fn: func() {
			g.snap()
			g.move(id, "battlefield", moveOpt{})
			if o := get(); o != nil {
				o.Down = true
			}
		}})
	}
	if i.Zone != "hand" {
		mv("hand", "Mão", moveOpt{})
	}
	if i.Zone != "graveyard" {
		mv("graveyard", "Cemitério", moveOpt{})
	}
	if i.Zone != "exile" {
		mv("exile", "Exílio", moveOpt{})
	}
	mv("library", "Topo do grimório", moveOpt{})
	mv("library", "Fundo do grimório", moveOpt{bottom: true})
	if i.Cmdr && i.Zone != "command" {
		mv("command", "Zona de comando", moveOpt{})
	}
	if i.Zone == "hand" {
		items = append(items, item{hd: "Mão"},
			item{label: "Descartar a mão inteira", fn: func() {
				g.snap()
				for _, h := range append([]int{}, g.S.Z["hand"]...) {
					g.move(h, "graveyard", moveOpt{})
				}
			}},
			item{label: "Descartar uma aleatória", fn: func() {
				if h := g.S.Z["hand"]; len(h) > 0 {
					g.snap()
					g.move(h[rnd(len(h))], "graveyard", moveOpt{})
				}
			}})
	}
	g.openCtx(items)
}

func exeDir() string {
	if d := os.Getenv("MESA_DIR"); d != "" {
		return d
	}
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return filepath.Dir(p)
}

// bump muda um contador já colocado (clique no − ou no + do próprio contador)
func (g *Game) bump(id int, k string, d int) {
	if g.S == nil || g.S.Cards[id] == nil || g.S.Phase != "play" {
		return
	}
	g.snap()
	o := g.S.Cards[id]
	o.Counters[k] = max(0, o.Counters[k]+d)
	if o.Counters[k] == 0 {
		delete(o.Counters, k)
	}
}

// hoverMoveKey: tecla de mover a carta que está embaixo do mouse
func hoverMoveKey() (zone, lab string) {
	switch {
	case jpA("cemiterio"):
		return "graveyard", "cemitério"
	case jpA("exilio"):
		return "exile", "exílio"
	case jpA("mao"):
		return "hand", "mão"
	case jpA("fundo"):
		return "library", "fundo"
	}
	return "", ""
}

// atalhos que o usuário pode trocar (janela F1). Ctrl+Z, Esc, Espaço e F1 ficam fixos.
var keyActions = []struct{ id, desc, def string }{
	{"comprar", "comprar 1", "D"},
	{"fase", "próxima fase (Espaço também)", "F"},
	{"turno", "próximo turno (desvira, não compra)", "N"},
	{"desvirar", "desvirar tudo", "U"},
	{"embaralhar", "embaralhar o grimório", "S"},
	{"ficha", "criar ficha", "T"},
	{"zoom", "ver a carta grande (mouse em cima)", "Z"},
	{"cemiterio", "mouse sobre a carta: para o cemitério", "G"},
	{"exilio", "mouse sobre a carta: para o exílio", "X"},
	{"mao", "mouse sobre a carta: para a mão", "H"},
	{"fundo", "mouse sobre a carta: fundo do grimório", "B"},
	{"estilo", "estilo das cartas na mesa", "E"},
	{"painel", "esconder/mostrar o painel", "P"},
}

func keyName(id string) string {
	if k, ok := cfg.Keys[id]; ok && k != "" {
		return k
	}
	for _, a := range keyActions {
		if a.id == id {
			return a.def
		}
	}
	return ""
}
func keyOf(id string) (ebiten.Key, bool) {
	var k ebiten.Key
	if err := k.UnmarshalText([]byte(keyName(id))); err != nil {
		return 0, false
	}
	return k, true
}
func jpA(id string) bool {
	k, ok := keyOf(id)
	return ok && inpututil.IsKeyJustPressed(k)
}

// setKey dá a tecla à ação; se outra ação usava essa tecla, as duas trocam
func setKey(id string, k ebiten.Key) {
	if cfg.Keys == nil {
		cfg.Keys = map[string]string{}
	}
	name := k.String()
	old := keyName(id)
	for _, a := range keyActions {
		if a.id != id && keyName(a.id) == name {
			cfg.Keys[a.id] = old
		}
	}
	cfg.Keys[id] = name
	saveCfg()
}

// shortcutsModal: lista de todos os atalhos (F1); os de tecla única podem ser trocados
func (g *Game) shortcutsModal() {
	fixed := [][2]string{
		{"F1", "esta lista de atalhos"},
		{"Clique / duplo clique", "vira no campo, compra / joga da mão"},
		{"Botão direito", "menu da carta; no cemitério/exílio, menu da pilha"},
		{"Ctrl+Z", "desfazer"},
		{"[  ]   1-4   Tab", "tamanho das cartas, visão em rede, troca de mesa"},
		{"Rodinha na vida", "vida +1/-1 (Shift: 5)"},
	}
	const rh = 21.0
	h := float64(len(fixed)+len(keyActions))*rh + 120
	g.openModal(&Modal{title: "Atalhos", w: 640, h: h, draw: func(x, y, w, h float64) {
		for k, l := range fixed {
			yy := y + float64(k)*rh
			txt(g.dst, l[0], x, yy, fBold, 12.5, cAcc)
			txt(g.dst, l[1], x+190, yy, fReg, 12.5, cTxt)
		}
		y0 := y + float64(len(fixed))*rh + 8
		txt(g.dst, "Clique na tecla para trocar e aperte a nova (Esc cancela):", x, y0, fReg, 11.5, cMut)
		y0 += 22
		for k, a := range keyActions {
			a := a
			yy := y0 + float64(k)*rh
			lab := keyName(a.id)
			st := 0
			if g.rebind == a.id {
				lab, st = "aperte…", 1
			}
			g.button(x, yy-2, 110, 19, lab, st, func() { g.rebind = a.id })
			txt(g.dst, a.desc, x+190, yy, fReg, 12.5, cTxt)
		}
		g.button(x+w-150, y0+float64(len(keyActions))*rh+2, 150, 24, "Voltar ao padrão", 0, func() { cfg.Keys = nil; saveCfg() })
	}})
}

// pileMenu: ações para a pilha inteira (botão direito no cemitério ou no exílio)
func (g *Game) pileMenu(z string) []item {
	name := map[string]string{"graveyard": "Cemitério", "exile": "Exílio"}[z]
	n := len(g.S.Z[z])
	items := []item{{hd: fmt.Sprintf("%s (%d)", name, n)}, {label: "Ver as cartas", fn: func() { g.viewZone(z, false) }}}
	if n == 0 {
		return items
	}
	all := func(to string, o moveOpt, msg string) func() {
		return func() {
			g.snap()
			for _, id := range append([]int{}, g.S.Z[z]...) {
				g.move(id, to, o)
			}
			if to == "library" && !o.bottom {
				shuffle(g.S.Z["library"])
			}
			g.announce(msg)
		}
	}
	items = append(items, item{hd: "Todas as cartas"},
		item{label: "Embaralhar no grimório", fn: all("library", moveOpt{}, name+" embaralhado no grimório")},
		item{label: "Para o fundo do grimório", fn: all("library", moveOpt{bottom: true}, name+" no fundo do grimório")},
		item{label: "Para a mão", fn: all("hand", moveOpt{}, name+" voltou para a mão")})
	if z == "graveyard" {
		items = append(items, item{label: "Exilar o cemitério", fn: all("exile", moveOpt{}, "Cemitério exilado")})
	} else {
		items = append(items, item{label: "Para o cemitério", fn: all("graveyard", moveOpt{}, "Exílio foi para o cemitério")})
	}
	return items
}
