package main

// ver.go: carta em tamanho grande (zoom), cemitério dos outros jogadores e
// troca de imagens de cartas entre os jogadores da mesa.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	xdraw "golang.org/x/image/draw"
)

/* ---------------- carta grande ---------------- */

type zoomView struct {
	c   *Card
	fi  int
	cnt map[string]int // contadores que a carta tem na mesa
}

func (g *Game) openZoom(c *Card, fi int) {
	if c == nil {
		return
	}
	g.zoom, g.ctx = &zoomView{c: c, fi: fi}, nil
}

// zoomHover abre a carta que está sob o cursor (tecla Z ou botão do meio)
func (g *Game) zoomHover() {
	if g.zoom != nil {
		g.zoom = nil
		return
	}
	if g.pvNow && g.pv != nil && !g.pvHide {
		g.openZoom(g.pv, g.pvFace)
	}
}

func cardBody(c *Card, fi int) string {
	f, _ := loc(c, fi)
	body := f.T + "\n\n" + f.X
	if f.P != "" || f.O != "" {
		body += "\n\n" + f.P + "/" + f.O
	}
	if f.L != "" {
		body += "\n\nLealdade: " + f.L
	}
	if nt := locNote(c, fi); nt != "" {
		body += "\n\n" + nt
	}
	return body
}

func (g *Game) drawZoom() {
	z := g.zoom
	W, H := float64(g.W), float64(g.H)
	shut := func() { g.zoom = nil }
	rect(g.dst, 0, 0, W, H, color.RGBA{0, 0, 0, 0xdc})
	g.add(region{x: 0, y: 0, w: W, h: H, block: true, click: shut, rclick: shut, wheel: func(float64) {}})
	ch := min(H-56, 940)
	cw := ch / ratio
	tW := min(400, W-cw-80)
	gap := 28.0
	if tW < 200 {
		tW, gap = 0, 0
	}
	x, y := (W-cw-tW-gap)/2, (H-ch)/2
	drawTex(g.dst, g.cardTex(z.c, z.fi), x, y, cw, false, 1)
	g.add(region{x: x, y: y, w: cw, h: ch, click: func() {}}) // clicar na carta não fecha
	n := len(z.c.Faces())
	if tW > 0 {
		tx := x + cw + gap
		f, _ := loc(z.c, z.fi)
		ty := y + 6
		for _, l := range wrap(f.N, fBold, 22, tW) {
			txt(g.dst, l, tx, ty, fBold, 22, cAcc)
			ty += 28
		}
		if f.C != "" {
			txt(g.dst, f.C, tx, ty, fReg, 15, cAcc2)
			ty += 24
		}
		ls := wrap(cardBody(z.c, z.fi), fReg, 15.5, tW)
		if maxL := int((y + ch - 60 - ty) / 20); len(ls) > maxL && maxL > 1 {
			ls = append(ls[:maxL-1], "…")
		}
		for _, l := range ls {
			txt(g.dst, l, tx, ty+6, fReg, 15.5, cTxt)
			ty += 20
		}
		var cs []string
		for k, v := range z.cnt {
			if v != 0 {
				cs = append(cs, fmt.Sprintf("%d %s", v, k))
			}
		}
		if len(cs) > 0 {
			sort.Strings(cs)
			for _, l := range wrap("Contadores: "+strings.Join(cs, ", "), fBold, 15.5, tW) {
				txt(g.dst, l, tx, ty+18, fBold, 15.5, cAcc)
				ty += 20
			}
		}
		bx := tx
		if n > 1 {
			g.button(bx, y+ch-36, 0, 32, "Ver a outra face", 0, func() { z.fi = (z.fi + 1) % n })
			bx += tw("Ver a outra face", fBold, 14) + 44
		}
		g.button(bx, y+ch-36, 0, 32, "Fechar", 1, shut)
	} else if n > 1 {
		g.button(x+cw+8, y+ch-36, 0, 32, "Outra face", 0, func() { z.fi = (z.fi + 1) % n })
	}
	txtC(g.dst, "Esc, Z ou clique fora para fechar", W/2, H-22, fReg, 12.5, cMut)
}

/* ---------------- cemitério e exílio dos outros ---------------- */

// peerMenu: o que dá para ver de outro jogador
func (g *Game) peerMenu(p *Peer) {
	st := p.State
	if st == nil || !st.InGame {
		return
	}
	id := p.ID
	g.openCtx([]item{
		{hd: p.Name},
		{label: fmt.Sprintf("Ver o cemitério (%d)", len(st.GY)), fn: func() { g.peerZone(id, "CEMITÉRIO") }},
		{label: fmt.Sprintf("Ver o exílio (%d)", len(st.EX)), fn: func() { g.peerZone(id, "EXÍLIO") }},
		{label: "Ver a mesa inteira", fn: func() { g.viewPeer = id; g.setMode(1) }},
	})
}

// peerZone abre a pilha de outro jogador; a lista acompanha o que ele faz enquanto a janela está aberta
func (g *Game) peerZone(id int, title string) {
	p := nw.peer(id)
	if p == nil {
		return
	}
	m := &Modal{title: strings.Title(strings.ToLower(title)) + " de " + p.Name, w: 1100, h: 700}
	m.draw = func(x, y, w, h float64) {
		var list []PubInst
		if q := nw.peer(id); q != nil && q.State != nil {
			p = q
			list = q.State.GY
			if title == "EXÍLIO" {
				list = q.State.EX
			}
		}
		if len(list) == 0 {
			txt(g.dst, "Vazio.", x, y, fReg, 14, cMut)
		}
		g.grid(m, x, y, w-8, h-44, len(list), 0, func(k int, cx, cy, cw float64) []cellBtn {
			g.drawPubInst(p, list[len(list)-1-k], cx, cy, cw, true)
			return nil
		})
		txt(g.dst, fmt.Sprintf("%d carta(s) · a mais nova primeiro · clique em uma carta para ver grande", len(list)), x, y+h-25, fReg, 13, cMut)
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}
	g.openModal(m)
}

// plateZones: botões de cemitério e exílio na placa de um jogador; devolve onde eles começam
func (g *Game) plateZones(p *Peer, xr, y float64) float64 {
	st := p.State
	if st == nil || !st.InGame {
		return xr
	}
	id := p.ID
	x := xr - 6
	for _, z := range []struct {
		t string
		n int
	}{{"EXÍLIO", len(st.EX)}, {"CEMITÉRIO", len(st.GY)}} {
		lab := fmt.Sprintf("%s %d", strings.ToLower(z.t), z.n)
		w := tw(lab, fReg, 11.5) + 14
		x -= w
		rrect(g.dst, x, y+4, w, 18, 6, cPanel2)
		txtC(g.dst, lab, x+w/2, y+6, fReg, 11.5, cTxt)
		t := z.t
		g.add(region{x: x, y: y + 4, w: w, h: 18, click: func() { g.peerZone(id, t) }})
		x -= 4
	}
	return x
}

/* ---------------- imagens das cartas dos outros ---------------- */

var imgKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}:[01]$`)

type imgReq struct {
	from string
	keys []string
}
type imgIn struct {
	from string
	key  string
	data []byte
}

type imgShare struct {
	have  map[string]bool // já está em dados/imagens
	chk   map[string]int  // quadro da última conferida no disco
	asked map[string]int  // quadro do último pedido sem resposta
	tries map[string]int
	none  map[string]int // "chave|jogador": quadro em que ele disse que também não tem
	got   int            // recebidas nesta sessão
	sent  int
}

var ish = &imgShare{have: map[string]bool{}, chk: map[string]int{}, asked: map[string]int{}, tries: map[string]int{}, none: map[string]int{}}

// sharePath: onde fica o arquivo de uma chave trocada na rede ("a|id:face" é só a ilustração)
// "p|arquivo" e "s|arquivo" são o playmat e a sleeve que o outro jogador pôs nas pastas dele; o que chega fica em dados/rede.
func sharePath(key string) string {
	if strings.HasPrefix(key, "a|") {
		return artPath(key[2:])
	}
	if kind, name := lookKey(key); kind != "" {
		return filepath.Join(baseDir, "dados", "rede", kind, name)
	}
	return imgPath(key)
}

var lookNameRe = regexp.MustCompile(`(?i)^[^/\\:*?"<>|\x00-\x1f]{1,90}\.(png|jpe?g)$`)

func lookKey(key string) (kind, name string) {
	switch {
	case strings.HasPrefix(key, "p|"):
		kind = "playmats"
	case strings.HasPrefix(key, "s|"):
		kind = "sleeves"
	default:
		return "", ""
	}
	return kind, key[2:]
}
func shareOK(key string) bool {
	if kind, name := lookKey(key); kind != "" {
		return lookNameRe.MatchString(name) && !strings.Contains(name, "..")
	}
	return imgKeyRe.MatchString(strings.TrimPrefix(key, "a|"))
}

var lookSend = map[string][]byte{}

// shareData: o que eu mando quando me pedem uma chave (playmat e sleeve vão reduzidos, em JPEG)
func shareData(key string) []byte {
	kind, name := lookKey(key)
	if kind == "" {
		if fi, err := os.Stat(sharePath(key)); err == nil && fi.Size() < 3<<20 {
			b, _ := os.ReadFile(sharePath(key))
			return b
		}
		return nil
	}
	if b, ok := lookSend[key]; ok {
		return b
	}
	var out []byte
	if f, err := os.Open(filepath.Join(lookDir(kind), name)); err == nil {
		if src, _, err := image.Decode(f); err == nil {
			b := src.Bounds()
			lim := 1600.0
			if kind == "sleeves" {
				lim = 700
			}
			k := min(1, lim/float64(max(b.Dx(), b.Dy())))
			dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*k)), max(1, int(float64(b.Dy())*k))))
			draw.Draw(dst, dst.Bounds(), image.Black, image.Point{}, draw.Src)
			xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
			var buf bytes.Buffer
			if jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 86}) == nil {
				out = buf.Bytes()
			}
		}
		f.Close()
	}
	lookSend[key] = out
	return out
}

// peerMat e peerSleeve: o playmat e o verso que o outro jogador escolheu
func peerLook(kind, key string) (*ebiten.Image, bool) {
	if strings.HasPrefix(key, "f:") {
		ck := "rede/" + kind + "/" + key
		if im, ok := lookTex[ck]; ok {
			return im, im != nil
		}
		sk := map[string]string{"playmats": "p|", "sleeves": "s|"}[kind] + key[2:]
		if !shareOK(sk) {
			return nil, false
		}
		if _, err := os.Stat(sharePath(sk)); err != nil {
			return nil, false // ainda não chegou
		}
		im := loadPicture(sharePath(sk), 1920, kind == "sleeves")
		lookTex[ck] = im
		return im, im != nil
	}
	if l := findLook(kind, key); l != nil {
		return lookImage(kind, l), true
	}
	return nil, false
}
func (g *Game) peerMat(st *PubState) *ebiten.Image {
	buildMats()
	if st == nil || cfg.NoPeerLook {
		return nil
	}
	im, _ := peerLook("playmats", st.Mat)
	return im
}
func (g *Game) peerSleeve(st *PubState) *ebiten.Image {
	buildSleeves()
	if st == nil || cfg.NoPeerLook {
		return g.backTex()
	}
	key := st.Sleeve
	if key == "" {
		key = "b:Arcana"
	}
	if im, ok := peerLook("sleeves", key); ok && im != nil {
		return im
	}
	return sleevesBuilt[0].tex
}

func (g *Game) hasImg(key string) bool {
	if ish.have[key] {
		return true
	}
	if f, ok := ish.chk[key]; ok && g.frame-f < 300 {
		return false
	}
	ish.chk[key] = g.frame
	if _, err := os.Stat(sharePath(key)); err == nil {
		ish.have[key] = true
		return true
	}
	return false
}

// imgSync: pede aos outros jogadores a imagem das cartas deles que eu não tenho e atende aos pedidos deles
func (g *Game) imgSync() {
	nw.mu.Lock()
	reqs, ins := nw.imgReq, nw.imgIn
	nw.imgReq, nw.imgIn = nil, nil
	nw.mu.Unlock()
	if cfg.NoShare {
		return
	}
	for _, in := range ins {
		delete(ish.asked, in.key)
		delete(ish.tries, in.key)
		if !shareOK(in.key) {
			continue
		}
		if len(in.data) == 0 {
			ish.none[in.key+"|"+in.from] = g.frame
			continue
		}
		if g.hasImg(in.key) || len(in.data) > 3<<20 {
			continue
		}
		cf, kind, err := image.DecodeConfig(bytes.NewReader(in.data))
		if err != nil || (kind != "jpeg" && kind != "png") || cf.Width > 2400 || cf.Height > 3400 || cf.Width < 100 {
			continue
		}
		path := sharePath(in.key)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if os.WriteFile(path+".tmp", in.data, 0o644) == nil && os.Rename(path+".tmp", path) == nil {
			ish.have[in.key] = true
			ish.got++
			forgetMissing()
			if strings.HasPrefix(in.key, "a|") {
				artDirty.Store(true)
			}
		}
	}
	for _, rq := range reqs {
		for k, key := range rq.keys {
			if k >= 4 || !shareOK(key) {
				break
			}
			data := shareData(key)
			if len(data) > 0 {
				ish.sent++
			}
			nw.send(&Msg{T: "img", To: rq.from, Keys: []string{key}, Data: data})
		}
	}
	// o que falta pedir
	again := 1800 // quem não tinha a imagem pode ter baixado depois: pergunta de novo a cada meio minuto
	if g.test != nil {
		again = 60
	}
	pend := 0
	for k, f := range ish.asked {
		if g.frame-f > 600 { // sem resposta: tenta de novo mais tarde
			delete(ish.asked, k)
		} else {
			pend++
		}
	}
	pre, cards := "", true // com a nossa moldura na mesa, o que interessa é só a ilustração
	switch cfg.Table {
	case "simples":
		cards = false
	case "moldura", "estendida", "cena":
		pre = "a|"
	}
	for _, p := range nw.peerList() {
		st := p.State
		if st == nil || !st.InGame || pend >= 3 {
			continue
		}
		var keys []string
		for _, lk := range [][2]string{{"p|", st.Mat}, {"s|", st.Sleeve}} { // playmat e sleeve que ele pôs na pasta dele
			key := lk[0] + strings.TrimPrefix(lk[1], "f:")
			if !strings.HasPrefix(lk[1], "f:") || cfg.NoPeerLook || !shareOK(key) || ish.tries[key] >= 5 || g.hasImg(key) {
				continue
			}
			if f, no := ish.none[key+"|"+p.Name]; no && g.frame-f < again {
				continue
			}
			if _, on := ish.asked[key]; !on {
				ish.asked[key] = g.frame
				ish.tries[key]++
				keys = append(keys, key)
				pend++
			}
		}
		if !cards {
			if len(keys) > 0 {
				nw.send(&Msg{T: "imgreq", To: p.Name, Keys: keys})
			}
			continue
		}
		for _, list := range [][]PubInst{st.BF, st.CMD, st.Shown, st.GY, st.EX} {
			for _, pi := range list {
				c := cardByID[pi.Cid]
				if pi.Down || c == nil || c.Custom || pend >= 3 {
					continue
				}
				key := pre + pi.Cid + ":" + string(rune('0'+pi.Face))
				if !shareOK(key) || ish.tries[key] >= 5 || g.hasImg(key) {
					continue
				}
				if f, no := ish.none[key+"|"+p.Name]; no && g.frame-f < again {
					continue
				}
				if _, on := ish.asked[key]; on {
					continue
				}
				ish.asked[key] = g.frame
				ish.tries[key]++
				keys = append(keys, key)
				pend++
			}
		}
		if len(keys) > 0 {
			nw.send(&Msg{T: "imgreq", To: p.Name, Keys: keys})
		}
	}
}
