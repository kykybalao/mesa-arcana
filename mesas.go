package main

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
)

// Modos de visão da mesa em rede: 1 uma mesa por vez, 2 lado a lado, 3 empilhado (oponentes em cima), 4 quadrante.
var modeNames = []string{"", "Uma mesa", "Lado a lado", "Empilhado", "Quadrante"}

type pane struct {
	p          *Peer
	x, y, w, h float64
}

func (g *Game) setMode(m int) {
	if m < 1 || m > 4 || !nw.active() {
		return
	}
	if m != g.viewMode {
		g.viewMode = m
		g.clampBF = true // o campo muda de tamanho: nenhuma carta fica escondida fora dele
	}
}

func (g *Game) cyclePeer() {
	ps := nw.peerList()
	if len(ps) == 0 {
		return
	}
	ids := []int{}
	if g.viewMode <= 1 {
		ids = append(ids, 0)
	}
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	k := 0
	for i, id := range ids {
		if id == g.viewPeer {
			k = (i + 1) % len(ids)
		}
	}
	g.viewPeer = ids[k]
}

// layoutPanes divide a área do campo entre a minha mesa e as dos outros jogadores
func (g *Game) layoutPanes(ps []*Peer, x, y, w, h float64) (mine [4]float64, out []pane) {
	const gap = 4.0
	n := len(ps)
	switch {
	case n == 0 || g.viewMode <= 1:
		return [4]float64{x, y, w, h}, nil
	case g.viewMode == 2:
		p := ps[0]
		for _, q := range ps {
			if q.ID == g.viewPeer {
				p = q
			}
		}
		hw := (w - gap) / 2
		return [4]float64{x, y, hw, h}, []pane{{p, x + hw + gap, y, hw, h}}
	case g.viewMode == 3:
		th := h * 0.42
		pw := (w - gap*float64(n-1)) / float64(n)
		for k, p := range ps {
			out = append(out, pane{p, x + float64(k)*(pw+gap), y, pw, th})
		}
		return [4]float64{x, y + th + gap, w, h - th - gap}, out
	}
	// quadrante: a minha embaixo à esquerda
	hw, hh := (w-gap)/2, (h-gap)/2
	topN := min(n, 2)
	if n > 3 {
		topN = n - 1
	}
	tw0 := (w - gap*float64(topN-1)) / float64(topN)
	for k := 0; k < topN; k++ {
		out = append(out, pane{ps[k], x + float64(k)*(tw0+gap), y, tw0, hh})
	}
	if n > topN {
		out = append(out, pane{ps[n-1], x + hw + gap, y + hh + gap, hw, hh})
		return [4]float64{x, y + hh + gap, hw, hh}, out
	}
	return [4]float64{x, y + hh + gap, w, hh}, out
}

func trkText(t map[string]int) string {
	var ks []string
	for k, v := range t {
		if v != 0 {
			ks = append(ks, k)
		}
	}
	sort.Strings(ks)
	var out []string
	for _, k := range ks {
		out = append(out, fmt.Sprintf("%s %d", strings.ToLower(k), t[k]))
	}
	return strings.Join(out, " · ")
}

// drawPeerPane mostra a mesa de outro jogador em um quadro (só leitura), com a placa dele em cima
func (g *Game) drawPeerPane(pn pane) {
	p, x, y, w, h := pn.p, pn.x, pn.y, pn.w, pn.h
	st := p.State
	const ph = 26.0
	rect(g.dst, x, y, w, h, color.RGBA{0x1c, 0x2f, 0x3d, 0xff})
	g.add(region{x: x, y: y, w: w, h: h, block: true, click: func() {}, rclick: func() {}})
	if st != nil && st.InGame {
		f := 1.0
		if st.BW > 0 && st.BH > 0 {
			f = min(w/st.BW, (h-ph)/st.BH)
		}
		ox := x + (w-st.BW*f)/2
		old := g.dst
		g.dst = clip(old, x, y+ph, w, h-ph)
		for _, pi := range st.BF {
			g.drawPubInst(p, pi, ox+pi.X*f, y+ph+pi.Y*f, st.CW*f, false)
		}
		g.dst = old
		if len(st.BF) == 0 {
			txtC(g.dst, "campo vazio", x+w/2, y+h/2-8, fBold, 15, color.RGBA{0xff, 0xff, 0xff, 0x33})
		}
	} else {
		txtC(g.dst, "ainda não começou", x+w/2, y+h/2-8, fBold, 15, color.RGBA{0xff, 0xff, 0xff, 0x44})
	}
	// placa do jogador
	rect(g.dst, x, y, w, ph, cPanel)
	rect(g.dst, x, y+ph-1, w, 1, cLine)
	txt(g.dst, p.Name, x+10, y+5, fBold, 13.5, cAcc2)
	if st != nil && st.InGame {
		info := fmt.Sprintf("vida %d · mão %d · grimório %d · turno %d", st.Life, st.Hand, st.Library, st.Turn)
		if t := trkText(st.Trk); t != "" {
			info += " · " + t
		}
		nx := x + 10 + tw(p.Name, fBold, 13.5) + 12
		txt(g.dst, fit(info, fReg, 12.5, x+w-nx-8), nx, y+6, fReg, 12.5, cTxt)
	}
	id := p.ID
	g.add(region{x: x, y: y, w: w, h: ph, click: func() { g.viewMode, g.viewPeer, g.clampBF = 1, id, true }})
	rstroke(g.dst, x+0.5, y+0.5, w-1, h-1, 0, 1, cLine)
}

/* ---------------- marcadores do jogador ---------------- */

func (g *Game) trackerNames() []string {
	out := []string{"Veneno", "Energia", "Experiência"}
	if ps := nw.peerList(); len(ps) > 0 {
		for _, p := range ps {
			out = append(out, "Dano de comandante de "+p.Name)
		}
	} else {
		for k := 1; k <= 3; k++ {
			out = append(out, fmt.Sprintf("Dano de comandante do oponente %d", k))
		}
	}
	for k := range g.S.Trk { // marcadores de jogadores que já saíram continuam aparecendo
		found := false
		for _, o := range out {
			found = found || o == k
		}
		if !found {
			out = append(out, k)
		}
	}
	return out
}

func (g *Game) bumpTrk(k string, d int) {
	g.snap()
	if g.S.Trk == nil {
		g.S.Trk = map[string]int{}
	}
	g.S.Trk[k] = max(0, g.S.Trk[k]+d)
	if g.S.Trk[k] == 0 {
		delete(g.S.Trk, k)
	}
}

func (g *Game) trackerMenu() {
	items := []item{{hd: "Meus marcadores"}}
	for _, k := range g.trackerNames() {
		items = append(items, item{label: k, val: func() int { return g.S.Trk[k] }, pm: func(d int) { g.bumpTrk(k, d) }})
	}
	g.openCtx(items)
}

// drawTrackers: fila de marcadores no canto do meu campo (veneno, energia, experiência, dano de comandante)
func (g *Game) drawTrackers(x, y, maxW float64) {
	cx := x
	chipW := tw("Marcadores", fReg, 12) + 18
	rrect(g.dst, cx, y, chipW, 21, 10, color.RGBA{0, 0, 0, 0xb0})
	txtC(g.dst, "Marcadores", cx+chipW/2, y+3.5, fReg, 12, cTxt)
	g.add(region{x: cx, y: y, w: chipW, h: 21, click: g.trackerMenu})
	cx += chipW + 6
	var ks []string
	for k, v := range g.S.Trk {
		if v != 0 {
			ks = append(ks, k)
		}
	}
	sort.Strings(ks)
	for _, k := range ks {
		lab := fmt.Sprintf("%s %d", strings.Replace(k, "Dano de comandante d", "Cmd d", 1), g.S.Trk[k])
		w := tw(lab, fBold, 12) + 50
		if cx+w > x+maxW {
			break
		}
		rrect(g.dst, cx, y, w, 21, 10, cAcc)
		rrect(g.dst, cx+1, y+1, w-2, 19, 9, color.RGBA{0, 0, 0, 0xe6})
		txtC(g.dst, "−", cx+11, y+2.5, fBold, 13, cAcc)
		txtC(g.dst, lab, cx+w/2, y+3.5, fBold, 12, color.White)
		txtC(g.dst, "+", cx+w-11, y+2.5, fBold, 13, cAcc)
		g.add(region{x: cx, y: y, w: 22, h: 21, click: func() { g.bumpTrk(k, -1) }})
		g.add(region{x: cx + w - 22, y: y, w: 22, h: 21, click: func() { g.bumpTrk(k, 1) }})
		cx += w + 6
	}
}

// drawBands: faixas do campo (criaturas e encantamentos em cima, terrenos embaixo), só visuais
func (g *Game) drawBands(bx, by, bw, bh float64) {
	if cfg.NoBands {
		return
	}
	ly := by + bh - g.cw*ratio - 22
	if ly < by+bh*0.35 {
		return
	}
	rect(g.dst, bx, ly, bw, 1, color.RGBA{0xff, 0xff, 0xff, 0x26})
	faint := color.RGBA{0xff, 0xff, 0xff, 0x48}
	lab := "CRIATURAS / ENCANTAMENTOS"
	txt(g.dst, lab, bx+bw-10-tw(lab, fBold, 10.5), by+10, fBold, 10.5, faint)
	txt(g.dst, "TERRENOS", bx+bw-10-tw("TERRENOS", fBold, 10.5), ly+5, fBold, 10.5, faint)
}

// drawModes: botões dos modos de visão, no fim da faixa de jogadores
func (g *Game) drawModes(xr, y float64) {
	x := xr
	for m := 4; m >= 1; m-- {
		lab := fmt.Sprintf("%d %s", m, modeNames[m])
		w := tw(lab, fReg, 12) + 16
		x -= w
		if m == max(1, g.viewMode) {
			rrect(g.dst, x, y+3, w, 21, 6, cAcc)
			txtC(g.dst, lab, x+w/2, y+6.5, fBold, 12, color.RGBA{0x1b, 0x16, 0x08, 0xff})
		} else {
			rrect(g.dst, x, y+3, w, 21, 6, cPanel2)
			txtC(g.dst, lab, x+w/2, y+6.5, fReg, 12, cMut)
		}
		g.add(region{x: x, y: y + 3, w: w, h: 21, click: func() { g.setMode(m) }})
		x -= 4
	}
}
