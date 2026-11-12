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
	pub   map[int][4]float64    // cartas da mesa do outro jogador que está na tela
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
	g.au.inst, g.au.pub, g.au.chips = map[int][4]float64{}, map[int][4]float64{}, map[string][2]float64{}
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

// drawArrows desenha as setas que fazem sentido na mesa que está na tela (view == nil: a minha)
func (g *Game) drawArrows(view *Peer) {
	gold, red := color.RGBA{0xf2, 0xc2, 0x4e, 0xff}, color.RGBA{0xf0, 0x6a, 0x5e, 0xff}
	me := nw.myName()
	chip := func(name string) ([2]float64, bool) { c, ok := g.au.chips[name]; return c, ok }
	myCard := func(id int) string {
		if i := g.S.Cards[id]; i != nil {
			f, _ := loc(g.card(i), i.Face)
			return f.N
		}
		return ""
	}
	if view == nil {
		for _, a := range g.arrows {
			var x1, y1 float64
			if r, ok := g.au.inst[a.From]; ok {
				x1, y1 = mid(r)
			} else if c, ok := chip(""); ok {
				x1, y1 = c[0], c[1]
			} else {
				continue
			}
			if a.Peer == "" {
				if r, ok := g.au.inst[a.To]; ok {
					x2, y2 := mid(r)
					g.drawArrow(x1, y1, x2, y2, gold, "")
				}
			} else if c, ok := chip(a.Peer); ok {
				lab := "mesa de " + a.Peer
				for _, p := range nw.peerList() {
					if p.Name == a.Peer {
						if n := pubName(g, p, a.To); n != "" {
							lab = n + " (" + a.Peer + ")"
						}
					}
				}
				g.drawArrow(x1, y1, c[0], c[1], gold, lab)
			}
		}
		for _, p := range nw.peerList() {
			if p.State == nil {
				continue
			}
			for _, a := range p.State.Arrows {
				if a.Peer != me || me == "" {
					continue
				}
				c, ok := chip(p.Name)
				r, ok2 := g.au.inst[a.To]
				if ok && ok2 {
					x2, y2 := mid(r)
					lab := p.Name
					if n := pubName(g, p, a.From); n != "" {
						lab += ": " + n
					}
					g.drawArrow(c[0], c[1], x2, y2, red, lab)
				}
			}
		}
		return
	}
	// mesa de outro jogador
	if view.State != nil {
		for _, a := range view.State.Arrows {
			r, ok := g.au.pub[a.From]
			if !ok {
				continue
			}
			x1, y1 := mid(r)
			switch {
			case a.Peer == "":
				if t, ok := g.au.pub[a.To]; ok {
					x2, y2 := mid(t)
					g.drawArrow(x1, y1, x2, y2, red, "")
				}
			case a.Peer == me:
				if c, ok := chip(""); ok {
					lab := "minha mesa"
					if n := myCard(a.To); n != "" {
						lab = "minha " + n
					}
					g.drawArrow(x1, y1, c[0], c[1], red, lab)
				}
			default:
				if c, ok := chip(a.Peer); ok {
					g.drawArrow(x1, y1, c[0], c[1], red, "mesa de "+a.Peer)
				}
			}
		}
	}
	for _, a := range g.arrows {
		if a.Peer != view.Name {
			continue
		}
		c, ok := chip("")
		t, ok2 := g.au.pub[a.To]
		if ok && ok2 {
			x2, y2 := mid(t)
			lab := "você"
			if n := myCard(a.From); n != "" {
				lab = "sua " + n
			}
			g.drawArrow(c[0], c[1], x2, y2, gold, lab)
		}
	}
	for _, p := range nw.peerList() {
		if p.ID == view.ID || p.State == nil {
			continue
		}
		for _, a := range p.State.Arrows {
			if a.Peer != view.Name {
				continue
			}
			c, ok := chip(p.Name)
			t, ok2 := g.au.pub[a.To]
			if ok && ok2 {
				x2, y2 := mid(t)
				lab := p.Name
				if n := pubName(g, p, a.From); n != "" {
					lab += ": " + n
				}
				g.drawArrow(c[0], c[1], x2, y2, red, lab)
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
	if menu && g.S != nil && g.S.Phase == "play" {
		items := []item{{hd: name + " (" + p.Name + ")"}, {label: "Apontar uma seta para esta carta", fn: func() { mk(0) }}}
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
