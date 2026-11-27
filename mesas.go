package main

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/vector"
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
	if mt := g.peerMat(st); mt != nil {
		drawCover(g.dst, mt, x, y+ph, w, h-ph, 0.82)
	}
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
	tag, act := g.turnTag(p.Name)
	plate := tag + p.Name
	if act {
		plate += " · na vez"
		rect(g.dst, x, y, w, ph, color.RGBA{0x3a, 0x2f, 0x12, 0xff})
	}
	txt(g.dst, plate, x+10, y+5, fBold, 13.5, cAcc2)
	if st != nil && st.InGame {
		info := fmt.Sprintf("vida %d · mão %d · grimório %d · turno %d", st.Life, st.Hand, st.Library, st.Turn)
		if t := trkText(st.Trk); t != "" {
			info += " · " + t
		}
		nx := x + 10 + tw(plate, fBold, 13.5) + 12
		id := p.ID
		g.add(region{x: x, y: y, w: w, h: ph, click: func() { g.viewMode, g.viewPeer, g.clampBF = 1, id, true }, rclick: func() { g.peerMenu(p) }})
		zx := g.plateZones(p, x+w, y)
		txt(g.dst, fit(info, fReg, 12.5, zx-nx-8), nx, y+6, fReg, 12.5, cTxt)
	} else {
		id := p.ID
		g.add(region{x: x, y: y, w: w, h: ph, click: func() { g.viewMode, g.viewPeer, g.clampBF = 1, id, true }})
	}
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

var fieldNames = []string{"faixas", "zonas", "limpo"}

func fieldMode() int {
	if cfg.Field == 0 && cfg.NoBands {
		return 2
	}
	return max(0, min(2, cfg.Field))
}
func simNao(b bool) string {
	if b {
		return "sim"
	}
	return "não"
}
func (g *Game) cycleField() {
	cfg.Field, cfg.NoBands = (fieldMode()+1)%3, false
	saveCfg()
}
func (g *Game) togglePanel() {
	cfg.HideLeft = !cfg.HideLeft
	saveCfg()
	g.clampBF = true
}

// fieldZones: zonas do campo (em coordenadas do campo): criaturas, artefatos e encantamentos, terrenos
func (g *Game) fieldZones(W, H float64) [3][4]float64 {
	const m, gap = 6.0, 6.0
	ch := g.cw * ratio
	lh := min(ch+34, H*0.42)
	th := H - lh - gap - 2*m
	cwid := (W - 2*m - gap) * 0.64
	return [3][4]float64{
		{m, m, cwid, th},
		{m + cwid + gap, m, W - 2*m - gap - cwid, th},
		{m, m + th + gap, W - 2*m, lh},
	}
}

var zoneLabels = [3]string{"CRIATURAS", "ARTEFATOS E ENCANTAMENTOS", "TERRENOS"}

func zoneOf(t string) int {
	switch {
	case strings.Contains(t, "Land"):
		return 2
	case strings.Contains(t, "Creature"), strings.Contains(t, "Planeswalker"), strings.Contains(t, "Battle"):
		return 0
	}
	return 1
}

const zoneTop, zoneRow = 34.0, 30.0

// zonePos: lugar livre para uma carta nova, dentro da zona do tipo dela
func (g *Game) zonePos(i *Inst, W, H float64) (float64, float64) {
	z := g.fieldZones(W, H)[zoneOf(g.face(i).T)]
	cw := g.cw
	per := max(1, int((z[2]-12)/(cw+8)))
	x0, y0 := z[0]+8, z[1]+zoneTop
	taken := map[int]bool{}
	for _, id := range g.S.Z["battlefield"] {
		o := g.S.Cards[id]
		if o == i || o.Att != 0 || o.X < z[0]-cw/2 || o.X > z[0]+z[2] || o.Y < z[1]-10 || o.Y > z[1]+z[3] {
			continue
		}
		col := int((o.X-x0)/(cw+8) + 0.5)
		row := int((o.Y-y0)/zoneRow + 0.5)
		taken[row*per+col] = true
	}
	n := 0
	for taken[n] && n < per*6 {
		n++
	}
	return x0 + float64(n%per)*(cw+8), y0 + float64(n/per)*zoneRow
}

// snapZone encaixa a carta solta na grade da zona em que ela caiu (só no campo com zonas)
func (g *Game) snapZone(x, y float64) (float64, float64) {
	if fieldMode() != 1 || cfg.NoSnap {
		return x, y
	}
	W, H := float64(g.bf.Dx()), float64(g.bf.Dy())
	cw := g.cw
	ch := cw * ratio
	cx, cy := x+cw/2, y+ch*0.3
	for _, z := range g.fieldZones(W, H) {
		if cx < z[0] || cx >= z[0]+z[2]+6 || cy < z[1] || cy >= z[1]+z[3]+6 {
			continue
		}
		per := max(1, int((z[2]-12)/(cw+8)))
		col := max(0, min(per-1, int((x-z[0]-8)/(cw+8)+0.5)))
		rows := max(1, int((z[3]-zoneTop-ch)/zoneRow)+1)
		row := max(0, min(rows-1, int((y-z[1]-zoneTop)/zoneRow+0.5)))
		return z[0] + 8 + float64(col)*(cw+8), z[1] + zoneTop + float64(row)*zoneRow
	}
	return x, y
}

// drawBands: divisões do campo, só visuais: faixas (linha entre criaturas e terrenos) ou zonas com moldura
func (g *Game) drawBands(bx, by, bw, bh float64) {
	switch fieldMode() {
	case 2:
		return
	case 1:
		gold := color.RGBA{0xe0, 0xb3, 0x4a, 0x9c}
		for k, z := range g.fieldZones(bw, bh) {
			x, y, w, h := bx+z[0], by+z[1], z[2], z[3]
			rrect(g.dst, x, y, w, h, 10, color.RGBA{0, 0, 0, 0x4a})
			rstroke(g.dst, x+0.5, y+0.5, w-1, h-1, 10, 1.2, gold)
			rstroke(g.dst, x+5.5, y+5.5, w-11, h-11, 6, 0.8, color.RGBA{0xe0, 0xb3, 0x4a, 0x38})
			// cantos: o losango da marca do programa
			for _, c := range [][2]float64{{x + 14, y + 14}, {x + w - 14, y + 14}, {x + 14, y + h - 14}, {x + w - 14, y + h - 14}} {
				p := &vector.Path{}
				S := float32(uiS)
				p.MoveTo(float32(c[0])*S, float32(c[1]-6)*S)
				p.LineTo(float32(c[0]+6)*S, float32(c[1])*S)
				p.LineTo(float32(c[0])*S, float32(c[1]+6)*S)
				p.LineTo(float32(c[0]-6)*S, float32(c[1])*S)
				p.Close()
				fillP(g.dst, p, color.RGBA{0x10, 0x14, 0x17, 0xff})
				strokeP(g.dst, p, 1.2*S, gold)
				circ(g.dst, c[0], c[1], 1.8, color.RGBA{0x5c, 0xc2, 0xa6, 0xd0})
			}
			lab := zoneLabels[k]
			lw := tw(lab, fBold, 11) + 20
			if lw < w-60 {
				rrect(g.dst, x+w/2-lw/2, y+h-9, lw, 16, 8, color.RGBA{0x10, 0x14, 0x17, 0xff})
				txtC(g.dst, lab, x+w/2, y+h-8, fBold, 11, color.RGBA{0xe0, 0xb3, 0x4a, 0xd8})
			}
		}
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

// floatPreview: com o painel escondido, a carta sob o cursor aparece grande do lado oposto ao cursor
func (g *Game) floatPreview(right, y float64) {
	if g.pv == nil || g.pvHide || !g.pvNow || g.drag.moved || g.modal != nil || g.ctx != nil {
		return
	}
	w := 250.0
	x := right - w - 12
	if float64(g.mx) > float64(g.W)/2 {
		x = 12
	}
	rrect(g.dst, x-4, y+8, w+8, w*ratio+8, 14, color.RGBA{0, 0, 0, 0xc0})
	drawTex(g.dst, g.cardTex(g.pv, g.pvFace), x, y+12, w, false, 1)
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
