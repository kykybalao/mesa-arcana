package main

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"
)

// Ordem da mesa em rede: quem começa (sorteado no dado), de quem é a vez, tranca e tempo por turno.
// Não há um servidor mandando: cada jogador publica a versão que conhece e todos ficam com a mais nova.
type TableTurn struct {
	Seq   int64          `json:"s"`
	By    string         `json:"by,omitempty"`
	Order []string       `json:"o,omitempty"` // nomes na ordem de jogo; o primeiro é quem começa
	Rolls map[string]int `json:"r,omitempty"` // o dado de cada um no sorteio
	Act   int            `json:"a"`           // posição em Order de quem está na vez
	N     int            `json:"n"`           // número da vez (1 = primeira)
	Lock  bool           `json:"l,omitempty"` // tranca: só joga quem está na vez
	Limit int            `json:"t,omitempty"` // segundos por turno (0 = sem limite)
}

var limitSteps = []int{0, 60, 120, 180, 300, 600}

func (t *TableTurn) active() string {
	if t == nil || len(t.Order) == 0 {
		return ""
	}
	return t.Order[t.Act%len(t.Order)]
}

func (g *Game) hasOrder() bool {
	return nw.active() && !g.tableOff && g.table != nil && len(g.table.Order) > 0
}
func (g *Game) myTurn() bool { return g.hasOrder() && g.table.active() == nw.myName() }

// lockedOut: a tranca está ligada e a vez é de outro jogador
func (g *Game) lockedOut() bool {
	return g.hasOrder() && g.table.Lock && !g.myTurn() && !g.unlocked && g.S != nil && g.S.Phase == "play"
}

// denied avisa e devolve true quando a ação está trancada
func (g *Game) denied() bool {
	if !g.lockedOut() {
		return false
	}
	g.announce("Vez de " + g.table.active() + " (tranca ligada)")
	return true
}

// setTable publica uma versão nova da ordem da mesa
func (g *Game) setTable(mod func(t *TableTurn)) {
	old := g.table
	n := TableTurn{}
	if old != nil {
		n = *old
		n.Order = append([]string{}, old.Order...)
		n.Rolls = map[string]int{}
		for k, v := range old.Rolls {
			n.Rolls[k] = v
		}
	}
	mod(&n)
	n.Seq++
	n.By = nw.myName()
	g.table = &n
	g.tableChanged(old)
}

func (g *Game) tableChanged(old *TableTurn) {
	t := g.table
	if old != nil && old.active() == t.active() && old.N == t.N {
		return
	}
	g.turnStart = time.Now()
	g.unlocked = false
	if t.active() == "" || g.S == nil || g.S.Phase != "play" {
		return
	}
	if g.myTurn() {
		if t.N > 1 { // a vez voltou para mim: começa o meu turno, como o botão "Próximo turno" fazia
			g.beginTurn()
		}
		g.announce("Sua vez")
	} else {
		g.announce("Vez de " + t.active())
	}
}

// syncTable fica com a versão mais nova entre a minha e as dos outros jogadores
func (g *Game) syncTable() {
	if !nw.active() {
		g.table = nil
		return
	}
	for _, p := range nw.peerList() {
		if p.State == nil || p.State.Table == nil {
			continue
		}
		t := p.State.Table
		if g.table == nil || t.Seq > g.table.Seq || (t.Seq == g.table.Seq && t.By > g.table.By) {
			old := g.table
			cp := *t
			g.table = &cp
			g.tableChanged(old)
		}
	}
}

func (g *Game) tableNames() []string {
	out := []string{nw.myName()}
	for _, p := range nw.peerList() {
		out = append(out, p.Name)
	}
	return out
}

// rollFirst: cada jogador ganha um d20; o maior começa e a ordem segue do maior para o menor
func (g *Game) rollFirst() {
	names := g.tableNames()
	rolls := map[string]int{}
	for tries := 0; tries < 50; tries++ {
		seen := map[int]bool{}
		ok := true
		for _, n := range names {
			rolls[n] = rnd(20) + 1
			ok = ok && !seen[rolls[n]]
			seen[rolls[n]] = true
		}
		if ok {
			break
		}
	}
	sort.SliceStable(names, func(a, b int) bool { return rolls[names[a]] > rolls[names[b]] })
	var parts []string
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s %d", n, rolls[n]))
	}
	g.setTable(func(t *TableTurn) { t.Order, t.Rolls, t.Act, t.N = names, rolls, 0, 1 })
	if g.S != nil {
		g.logP("", "sorteio no d20: %s. %s começa", strings.Join(parts, ", "), names[0])
	}
}

// passTurn passa a vez para o próximo jogador que ainda está na mesa
func (g *Game) passTurn() {
	if !g.hasOrder() {
		return
	}
	here := map[string]bool{}
	for _, n := range g.tableNames() {
		here[n] = true
	}
	from := g.table.active()
	g.setTable(func(t *TableTurn) {
		for k := 0; k < len(t.Order); k++ {
			t.Act = (t.Act + 1) % len(t.Order)
			if here[t.Order[t.Act]] {
				break
			}
		}
		t.N++
	})
	if g.S != nil {
		g.logP("", "vez de %s terminou; agora é a vez de %s", from, g.table.active())
	}
}

// turnTag: marca do jogador na faixa ("1º" para quem começa) e se a vez é dele
func (g *Game) turnTag(name string) (string, bool) {
	if !g.hasOrder() {
		return "", false
	}
	if name == "" {
		name = nw.myName()
	}
	tag := ""
	for k, n := range g.table.Order {
		if n == name {
			tag = fmt.Sprintf("%dº ", k+1)
		}
	}
	return tag, g.table.active() == name
}

func clock(d time.Duration) string {
	s := int(d.Seconds())
	if s < 0 {
		s = -s
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// drawTimer: tempo do turno, alinhado à direita em xr. Com limite na mesa, conta para trás.
func (g *Game) drawTimer(xr, y float64) {
	if g.turnStart.IsZero() {
		g.turnStart = time.Now()
	}
	el := time.Since(g.turnStart)
	lab, col := "Turno há "+clock(el), color.Color(cMut)
	if g.hasOrder() {
		who := "Vez de " + g.table.active()
		if g.myTurn() {
			who, col = "Sua vez", cAcc
		}
		lab = who + " · " + clock(el)
		if lim := g.table.Limit; lim > 0 {
			left := time.Duration(lim)*time.Second - el
			if left > 0 {
				lab = who + " · faltam " + clock(left)
			} else {
				lab, col = who+" · tempo esgotado há "+clock(left), cBad
			}
		}
	}
	txt(g.dst, lab, xr-tw(lab, fBold, 12.5), y, fBold, 12.5, col)
}

// lockBanner: aviso no campo para quem está trancado, com o botão de responder
func (g *Game) lockBanner(bx, by, bw, bh float64) {
	if !g.lockedOut() || g.au.pick {
		return
	}
	msg := "Vez de " + g.table.active() + ". A tranca está ligada: você joga na sua vez."
	w := tw(msg, fReg, 14) + 150
	ex := bx + (bw-w)/2
	rrect(g.dst, ex-1, by+bh-55, w+2, 44, 10, cAcc)
	rrect(g.dst, ex, by+bh-54, w, 42, 9, color.RGBA{0x10, 0x14, 0x17, 0xf2})
	txt(g.dst, msg, ex+14, by+bh-42, fReg, 14, cTxt)
	g.button(ex+w-122, by+bh-48, 110, 30, "Responder", 0, func() {
		g.unlocked = true
		g.logP("", "pediu para responder na vez de %s", g.table.active())
	})
}

func (g *Game) tableModal() {
	m := &Modal{title: "Ordem da mesa e vez", w: 620, h: 470}
	m.draw = func(x, y, w, h float64) {
		cy := y
		if !g.hasOrder() {
			for _, l := range wrap("Ainda não foi decidido quem começa. No sorteio cada jogador ganha um d20: o maior começa e a ordem segue do maior para o menor.", fReg, 13.5, w) {
				txt(g.dst, l, x, cy, fReg, 13.5, cTxt)
				cy += 18
			}
		} else {
			t := g.table
			for k, n := range t.Order {
				line := fmt.Sprintf("%dº  %s", k+1, n)
				if r, ok := t.Rolls[n]; ok {
					line += fmt.Sprintf("   (dado %d)", r)
				}
				col := color.Color(cTxt)
				if k == 0 {
					line += "   começa"
				}
				if n == t.active() {
					line += "   ← está na vez"
					col = cAcc
				}
				txt(g.dst, line, x, cy, fBold, 14, col)
				cy += 22
			}
			cy += 4
			txt(g.dst, fmt.Sprintf("Vez nº %d da mesa.", t.N), x, cy, fReg, 13, cMut)
			cy += 18
		}
		cy = max(cy+10, y+150)
		bw := (w - 10) / 2
		g.button(x, cy, bw, 34, "Sortear quem começa (d20)", 1, g.rollFirst)
		st := 0
		if !g.hasOrder() {
			st = 3
		}
		lab := "Passar a vez"
		if g.hasOrder() && !g.myTurn() {
			lab = "Pular a vez de " + g.table.active()
		}
		g.button(x+bw+10, cy, bw, 34, lab, st, g.passTurn)
		cy += 42
		lock, lim := false, 0
		if g.table != nil {
			lock, lim = g.table.Lock, g.table.Limit
		}
		g.button(x, cy, bw, 34, "Tranca: "+map[bool]string{true: "ligada", false: "desligada"}[lock], st, func() {
			g.setTable(func(t *TableTurn) { t.Lock = !t.Lock })
		})
		tl := "sem limite"
		if lim > 0 {
			tl = fmt.Sprintf("%d min", lim/60)
		}
		g.button(x+bw+10, cy, bw, 34, "Tempo por turno: "+tl, st, func() {
			g.setTable(func(t *TableTurn) {
				k := 0
				for i, v := range limitSteps {
					if v == t.Limit {
						k = i
					}
				}
				t.Limit = limitSteps[(k+1)%len(limitSteps)]
			})
		})
		cy += 42
		g.button(x, cy, bw, 34, "Desfazer a ordem da mesa", st, func() {
			g.setTable(func(t *TableTurn) { t.Order, t.Rolls, t.Act, t.N, t.Lock = nil, nil, 0, 0, false })
		})
		cy += 46
		for _, l := range wrap("Vale para a mesa toda: qualquer jogador pode mudar. Com a tranca, quem não está na vez não mexe nas cartas, mas pode mudar vida e contadores, e o botão Responder libera para jogar uma mágica na vez do outro. O tempo só avisa quando acaba; nada passa sozinho.", fReg, 12.5, w) {
			txt(g.dst, l, x, cy, fReg, 12.5, cMut)
			cy += 16
		}
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}
	g.openModal(m)
}
