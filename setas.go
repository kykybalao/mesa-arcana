package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Setas: indicam uma ação (alvo de uma mágica, ataque, etc.) de uma carta sua para outra carta,
// na sua mesa ou na mesa de outro jogador. Vão junto com o estado para todos na rede.
type Arrow struct {
	From int    `json:"f,omitempty"` // carta de quem criou a seta (0 = o jogador)
	To   int    `json:"t"`           // carta alvo
	Peer string `json:"p,omitempty"` // nome do dono da mesa alvo ("" = a mesa de quem criou)
}

type arrowUI struct {
	pick  bool // escolhendo o alvo
	src   int
	inst  map[int][4]float64    // minhas cartas no campo, neste quadro
	pub   map[[2]int][4]float64 // cartas das mesas dos outros jogadores que estão na tela (jogador, carta)
	chips map[string][2]float64 // ponta de cada jogador na faixa de jogadores ("" = eu)
	cnt   bool                  // desenhando o meu campo: os contadores ganham botões
	btns  []region
}

func (n *Net) myName() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.name
}

func (g *Game) arrowReset() {
	g.au.inst, g.au.pub, g.au.chips = map[int][4]float64{}, map[[2]int][4]float64{}, map[string][2]float64{}
}

// pruneArrows tira as setas cuja carta saiu do campo
func (g *Game) pruneArrows() {
	if g.S == nil {
		g.arrows = nil
		return
	}
	on := func(id int) bool { i := g.S.Cards[id]; return i != nil && i.Zone == "battlefield" }
	out := g.arrows[:0]
	for _, a := range g.arrows {
		if (a.From == 0 || on(a.From)) && (a.Peer != "" || on(a.To)) {
			out = append(out, a)
		}
	}
	g.arrows = out
}

func (g *Game) addArrow(a Arrow, what string) {
	for _, o := range g.arrows {
		if o == a {
			return
		}
	}
	g.arrows = append(g.arrows, a)
	src := "uma seta"
	if i := g.S.Cards[a.From]; i != nil {
		src = g.card(i).N
	}
	g.logP("", "aponta %s → %s", src, what)
}

func (g *Game) startArrow(id int) {
	g.au.pick, g.au.src = true, id
	g.equip = 0
}

func (g *Game) clearArrows(from int) {
	out := g.arrows[:0]
	for _, a := range g.arrows {
		if from != 0 && a.From != from && !(a.Peer == "" && a.To == from) {
			out = append(out, a)
		}
	}
	g.arrows = out
}

func mid(r [4]float64) (float64, float64) { return r[0] + r[2]/2, r[1] + r[3]/2 }

// drawArrow desenha uma seta curva de (x1,y1) até (x2,y2), com um rótulo opcional no meio
func (g *Game) drawArrow(x1, y1, x2, y2 float64, col color.RGBA, label string) {
	dx, dy := x2-x1, y2-y1
	d := math.Hypot(dx, dy)
	if d < 8 {
		return
	}
	nx, ny := -dy/d, dx/d
	bend := min(60, d*0.18)
	cx, cy := (x1+x2)/2+nx*bend, (y1+y2)/2+ny*bend
	// a ponta para um pouco antes do centro do alvo
	tdx, tdy := x2-cx, y2-cy
	td := math.Hypot(tdx, tdy)
	ux, uy := tdx/td, tdy/td
	const head = 22.0
	ex, ey := x2-ux*head*0.8, y2-uy*head*0.8
	S := float32(uiS)
	line := func(w float32, c color.Color) {
		p := &vector.Path{}
		p.MoveTo(float32(x1)*S, float32(y1)*S)
		p.QuadTo(float32(cx)*S, float32(cy)*S, float32(ex)*S, float32(ey)*S)
		strokeP(g.dst, p, w*S, c)
	}
	tip := func(grow float64, c color.Color) {
		h := head + grow
		bx, by := x2-ux*h, y2-uy*h
		wx, wy := -uy*h*0.55, ux*h*0.55
		p := &vector.Path{}
		p.MoveTo(float32(x2+ux*grow*0.6)*S, float32(y2+uy*grow*0.6)*S)
		p.LineTo(float32(bx+wx)*S, float32(by+wy)*S)
		p.LineTo(float32(bx-wx)*S, float32(by-wy)*S)
		p.Close()
		fillP(g.dst, p, c)
	}
	dark := color.RGBA{0, 0, 0, 0xd0}
	line(9, dark)
	tip(4, dark)
	line(5, col)
	tip(0, col)
	circ(g.dst, x1, y1, 7, dark)
	circ(g.dst, x1, y1, 4.5, col)
	if label != "" {
		lx, ly := (x1+2*cx+x2)/4, (y1+2*cy+y2)/4
		w := tw(label, fBold, 12) + 14
		rrect(g.dst, lx-w/2, ly-10, w, 20, 9, color.RGBA{0, 0, 0, 0xe0})
		txtC(g.dst, label, lx, ly-8, fBold, 12, col)
	}
}

func pubName(g *Game, p *Peer, id int) string {
	if p == nil || p.State == nil {
		return ""
	}
	for _, pi := range p.State.BF {
		if pi.ID == id {
			if c := g.pubCard(p, pi); c != nil && !pi.Down {
				f, _ := loc(c, pi.Face)
				return f.N
			}
			return "carta virada"
		}
	}
	return ""
}

// drawArrows desenha as setas entre as cartas que estão na tela; quando uma ponta está em uma mesa
// que não está aparecendo, a seta vai até o nome do jogador na faixa de jogadores.
func (g *Game) drawArrows() {
	gold, red := color.RGBA{0xf2, 0xc2, 0x4e, 0xff}, color.RGBA{0xf0, 0x6a, 0x5e, 0xff}
	me := nw.myName()
	peers := nw.peerList()
	byName := func(n string) *Peer {
		for _, p := range peers {
			if p.Name == n {
				return p
			}
		}
		return nil
	}
	myCard := func(id int) string {
		if i := g.S.Cards[id]; i != nil {
			f, _ := loc(g.card(i), i.Face)
			return f.N
		}
		return ""
	}
	// ponta: posição, se é a própria carta (e não o nome do jogador) e o rótulo usado quando não é
	type end struct {
		x, y  float64
		card  bool
		label string
		ok    bool
	}
	mine := func(id int) end {
		if r, ok := g.au.inst[id]; ok && id != 0 {
			x, y := mid(r)
			return end{x, y, true, "", true}
		}
		c, ok := g.au.chips[""]
		lab := "você"
		if n := myCard(id); n != "" {
			lab = n + " (você)"
		}
		return end{c[0], c[1], false, lab, ok}
	}
	theirs := func(p *Peer, id int) end {
		if p == nil {
			return end{}
		}
		if r, ok := g.au.pub[[2]int{p.ID, id}]; ok && id != 0 {
			x, y := mid(r)
			return end{x, y, true, "", true}
		}
		c, ok := g.au.chips[p.Name]
		lab := p.Name
		if n := pubName(g, p, id); n != "" {
			lab = n + " (" + p.Name + ")"
		}
		return end{c[0], c[1], false, lab, ok}
	}
	draw := func(a, b end, col color.RGBA) {
		if !a.ok || !b.ok || (!a.card && !b.card) {
			return
		}
		lab := a.label
		if lab == "" {
			lab = b.label
		}
		g.drawArrow(a.x, a.y, b.x, b.y, col, lab)
	}
	for _, a := range g.arrows {
		if a.Peer == "" {
			draw(mine(a.From), mine(a.To), gold)
		} else {
			draw(mine(a.From), theirs(byName(a.Peer), a.To), gold)
		}
	}
	for _, p := range peers {
		if p.State == nil {
			continue
		}
		for _, a := range p.State.Arrows {
			switch {
			case a.Peer == "":
				draw(theirs(p, a.From), theirs(p, a.To), red)
			case a.Peer == me && me != "":
				draw(theirs(p, a.From), mine(a.To), red)
			default:
				draw(theirs(p, a.From), theirs(byName(a.Peer), a.To), red)
			}
		}
	}
}

// arrowBanner: aviso enquanto se escolhe o alvo da seta
func (g *Game) arrowBanner(bx, by, bw, bh float64) {
	if !g.au.pick {
		return
	}
	nm := "sua mesa"
	if i := g.S.Cards[g.au.src]; i != nil {
		nm = g.card(i).N
	} else if g.au.src != 0 {
		g.au.pick = false
		return
	}
	msg := fmt.Sprintf("Seta de %s: clique na carta alvo", nm)
	if nw.active() {
		msg += " (pode abrir a mesa de outro jogador)"
	}
	w := tw(msg, fReg, 14) + 140
	ex := bx + (bw-w)/2
	rrect(g.dst, ex-1, by+bh-55, w+2, 44, 10, cAcc)
	rrect(g.dst, ex, by+bh-54, w, 42, 9, color.RGBA{0x10, 0x14, 0x17, 0xf2})
	txt(g.dst, msg, ex+14, by+bh-42, fReg, 14, cTxt)
	g.button(ex+w-112, by+bh-48, 100, 30, "Cancelar", 0, func() { g.au.pick = false })
}

// pubClick: clique numa carta da mesa de outro jogador
func (g *Game) pubClick(p *Peer, pi PubInst, menu bool) {
	name := "carta virada"
	if c := g.pubCard(p, pi); c != nil && !pi.Down {
		f, _ := loc(c, pi.Face)
		name = f.N
	}
	mk := func(from int) {
		g.addArrow(Arrow{From: from, To: pi.ID, Peer: p.Name}, name+" de "+p.Name)
		g.au.pick = false
	}
	if g.au.pick && !menu {
		mk(g.au.src)
		return
	}
	if c := g.pubCard(p, pi); !menu && c != nil && !pi.Down {
		g.openZoom(c, pi.Face)
		g.zoom.cnt = pi.Counters
		return
	}
	if menu && g.S != nil && g.S.Phase == "play" {
		items := []item{{hd: name + " (" + p.Name + ")"}}
		if c := g.pubCard(p, pi); c != nil && !pi.Down {
			items = append(items, item{label: "Ver a carta grande", key: keyName("zoom"), fn: func() { g.openZoom(c, pi.Face); g.zoom.cnt = pi.Counters }})
		}
		items = append(items, item{label: "Ver o cemitério de " + p.Name, fn: func() { g.peerZone(p.ID, "CEMITÉRIO") }})
		items = append(items, []item{{label: "Apontar uma seta para esta carta", fn: func() { mk(0) }}}...)
		has := false
		for _, a := range g.arrows {
			has = has || (a.Peer == p.Name && a.To == pi.ID)
		}
		if has {
			items = append(items, item{label: "Apagar minhas setas para esta carta", fn: func() {
				out := g.arrows[:0]
				for _, a := range g.arrows {
					if !(a.Peer == p.Name && a.To == pi.ID) {
						out = append(out, a)
					}
				}
				g.arrows = out
			}})
		}
		g.openCtx(items)
	}
}
