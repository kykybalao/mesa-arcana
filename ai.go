package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// Partida contra a IA (pedido do Kyky): um oponente simples que joga sozinho na mesma tela.
// Ela NÃO lê o efeito das cartas: joga um terreno por turno, conjura o que der pela curva de mana,
// ataca quando é seguro e bloqueia para não morrer. Entende só palavras-chave de combate
// (voar, alcance, atropelar, toque mortífero, vínculo com a vida, ímpeto, vigilância, defensor)
// e alguns feitiços simples (destruir/exilar criatura, destruir todas as criaturas, comprar cartas).
// Ela aparece para a mesa como se fosse um jogador da rede (modo "ia" do nw).

const aiPeerID = 900

type aiCard struct {
	id   int
	c    *Card
	tap  bool
	sick bool
	cmdr bool
}

type aiPlayer struct {
	on     bool
	name   string
	deck   *Deck
	lib    []*aiCard
	hand   []*aiCard
	bf     []*aiCard
	gy     []*aiCard
	ex     []*aiCard
	cmd    []*aiCard
	life   int
	turn   int
	tax    int
	nextID int
	over   bool
}

var ai aiPlayer

func (a *aiCard) face() Face       { return a.c.Faces()[0] }
func (a *aiCard) nm() string       { f, _ := loc(a.c, 0); return f.N }
func (a *aiCard) typ() string      { return a.c.typeLine() }
func (a *aiCard) is(t string) bool { return strings.Contains(a.typ(), t) }
func (a *aiCard) text() string     { return a.face().X }
func ptNum(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
func (a *aiCard) pow() int         { return ptNum(a.face().P) }
func (a *aiCard) tou() int         { return ptNum(a.face().O) }
func hasKw(text, kw string) bool   { return strings.Contains(strings.ToLower(text), strings.ToLower(kw)) }
func (a *aiCard) kw(k string) bool { return hasKw(a.text(), k) }
func (a *aiCard) mv() int          { return manaValue(a.face().C) }

// fighter é uma criatura em combate, minha ou da IA, com o que o combate precisa saber
type fighter struct {
	name                    string
	pow, tou                int
	fly, reach, trample     bool
	death, life, vig, defen bool
	mine                    int     // id da minha carta (0 = carta da IA)
	ac                      *aiCard // carta da IA
}

func fighterAI(a *aiCard) fighter {
	t := a.text()
	return fighter{name: a.nm(), pow: a.pow(), tou: a.tou(), fly: hasKw(t, "Flying"), reach: hasKw(t, "Reach"), trample: hasKw(t, "Trample"),
		death: hasKw(t, "Deathtouch"), life: hasKw(t, "Lifelink"), vig: hasKw(t, "Vigilance"), defen: hasKw(t, "Defender"), ac: a}
}
func (g *Game) fighterMine(id int) fighter {
	i := g.S.Cards[id]
	c := g.card(i)
	fs := c.Faces()
	f := fs[min(i.Face, len(fs)-1)]
	t := f.X
	p, o := ptNum(f.P)+i.Counters["+1/+1"]-i.Counters["-1/-1"], ptNum(f.O)+i.Counters["+1/+1"]-i.Counters["-1/-1"]
	lf, _ := loc(c, i.Face)
	return fighter{name: lf.N, pow: p, tou: o, fly: hasKw(t, "Flying"), reach: hasKw(t, "Reach"), trample: hasKw(t, "Trample"),
		death: hasKw(t, "Deathtouch"), life: hasKw(t, "Lifelink"), vig: hasKw(t, "Vigilance"), defen: hasKw(t, "Defender"), mine: id}
}
func canBlock(att, blk fighter) bool { return !att.fly || blk.fly || blk.reach }
func kills(src, dst fighter) bool    { return src.pow > 0 && (src.death || src.pow >= dst.tou) }

// minhas criaturas no campo (não viradas)
func (g *Game) myCreatures(untapped bool) []int {
	var out []int
	for _, id := range g.S.Z["battlefield"] {
		i := g.S.Cards[id]
		if i.Down || (untapped && i.Tapped) {
			continue
		}
		if strings.Contains(g.card(i).typeLine(), "Creature") {
			out = append(out, id)
		}
	}
	return out
}

/* ---------- começo da partida ---------- */

func (g *Game) aiModal(mine *Deck) {
	g.openModal(&Modal{title: "Jogar contra a IA", w: 620, h: 520, draw: func(x, y, w, h float64) {
		ls := wrap("Escolha o deck da IA. Ela joga sozinha: um terreno por turno, conjura o que der pela curva, ataca e bloqueia olhando poder, resistência e as palavras-chave de combate. Ela não faz os efeitos das cartas (só destruir/exilar criatura, destruir todas e comprar). Quando você clica em “Próximo turno”, a IA joga o turno dela.", fReg, 13, w)
		txt(g.dst, strings.Join(ls, "\n"), x, y, fReg, 13, cMut)
		y += float64(len(ls))*16.5 + 14
		for _, d := range g.decks {
			d := d
			if len(d.Entries) == 0 {
				continue
			}
			lab := d.Name
			if d == mine {
				lab += " (espelho)"
			}
			g.button(x, y, w, 32, lab, 0, func() { g.closeModal(); g.startAI(mine, d) })
			y += 38
		}
	}})
}

func (g *Game) startAI(mine, theirs *Deck) {
	aiStop()
	nw.stop()
	g.startGame(mine)
	ai = aiPlayer{on: true, name: "IA", deck: theirs, life: 40, nextID: 1}
	cmd := map[string]bool{}
	for _, id := range theirs.Cmd {
		cmd[id] = true
	}
	for _, e := range theirs.Entries {
		for k := 0; k < e.Qty; k++ {
			a := &aiCard{id: ai.nextID, c: e.Card}
			ai.nextID++
			if cmd[e.Card.ID] && k == 0 {
				a.cmdr = true
				ai.cmd = append(ai.cmd, a)
			} else {
				ai.lib = append(ai.lib, a)
			}
		}
	}
	if len(ai.cmd) == 0 {
		ai.life = 20
	}
	rand.Shuffle(len(ai.lib), func(i, j int) { ai.lib[i], ai.lib[j] = ai.lib[j], ai.lib[i] })
	ai.draw(7)
	nw.mu.Lock()
	nw.mode, nw.status = "ia", "Partida contra a IA"
	nw.peers = map[int]*Peer{aiPeerID: {ID: aiPeerID, Name: "IA"}}
	nw.mu.Unlock()
	g.setMode(2)
	g.aiPublish()
	g.aiSay("vai jogar com " + theirs.Name)
}

func (a *aiPlayer) draw(n int) int {
	k := 0
	for ; k < n && len(a.lib) > 0; k++ {
		a.hand = append(a.hand, a.lib[0])
		a.lib = a.lib[1:]
	}
	return k
}

func aiActive() bool { return ai.on && nw.active() }

// aiSay escreve no registro como se a IA fosse um jogador da rede
func (g *Game) aiSay(text string) {
	nw.mu.Lock()
	nw.events = append(nw.events, netEvent{from: aiPeerID, name: "IA", text: text})
	nw.mu.Unlock()
}

// aiPublish monta o que a mesa mostra da IA (campo arrumado em fileiras)
func (g *Game) aiPublish() {
	conv := func(l []*aiCard) []PubInst {
		out := []PubInst{}
		for _, a := range l {
			out = append(out, PubInst{ID: a.id, Cid: a.c.ID, Tapped: a.tap, Cmdr: a.cmdr})
		}
		return out
	}
	const bw, bh, cw = 1000.0, 560.0, 92.0
	rows := [3][]*aiCard{}
	for _, a := range ai.bf {
		switch {
		case a.is("Creature"):
			rows[0] = append(rows[0], a)
		case a.is("Land"):
			rows[2] = append(rows[2], a)
		default:
			rows[1] = append(rows[1], a)
		}
	}
	var bf []PubInst
	for r, row := range rows {
		step := cw + 14
		if n := float64(len(row)); n > 1 && step*n > bw-cw {
			step = (bw - cw - 20) / (n - 1)
		}
		for k, a := range row {
			bf = append(bf, PubInst{ID: a.id, Cid: a.c.ID, Tapped: a.tap, Cmdr: a.cmdr, X: 12 + float64(k)*step, Y: 10 + float64(r)*(cw*ratio+12)})
		}
	}
	if bf == nil {
		bf = []PubInst{}
	}
	st := &PubState{InGame: true, Deck: ai.deck.Name, Life: ai.life, Turn: ai.turn, Phase: "play", Hand: len(ai.hand), Library: len(ai.lib),
		BF: bf, GY: conv(ai.gy), EX: conv(ai.ex), CMD: conv(ai.cmd), BW: bw, BH: bh, CW: cw, Tax: map[string]int{}}
	for _, a := range ai.cmd {
		st.Tax[a.c.ID] = ai.tax
	}
	nw.mu.Lock()
	if p := nw.peers[aiPeerID]; p != nil {
		p.State = st
	}
	nw.mu.Unlock()
}

/* ---------- o turno da IA ---------- */

// aiTurn roda o turno inteiro da IA; o meu turno começa quando ela termina (ou quando eu decido os bloqueios)
func (g *Game) aiTurn() {
	if ai.over {
		g.beginTurn()
		return
	}
	ai.turn++
	for _, a := range ai.bf {
		a.tap, a.sick = false, false
	}
	if ai.draw(1) == 0 {
		g.aiSay("não tinha carta para comprar e perdeu")
		g.aiLose()
		g.beginTurn()
		return
	}
	g.aiSay(fmt.Sprintf("— turno %d", ai.turn))
	// terreno
	for k, a := range ai.hand {
		if a.is("Land") {
			ai.hand = append(ai.hand[:k], ai.hand[k+1:]...)
			a.tap = hasKw(a.text(), "enters the battlefield tapped") || hasKw(a.text(), "enters tapped")
			ai.bf = append(ai.bf, a)
			g.aiSay("jogou " + a.nm())
			break
		}
	}
	g.aiCast()
	g.aiPublish()
	att := g.aiAttackers()
	if len(att) == 0 {
		g.aiEnd()
		return
	}
	g.blockModal(att)
}

func (g *Game) aiEnd() {
	for len(ai.hand) > 7 { // descarta a mais cara
		sort.Slice(ai.hand, func(i, j int) bool { return ai.hand[i].mv() < ai.hand[j].mv() })
		a := ai.hand[len(ai.hand)-1]
		ai.hand = ai.hand[:len(ai.hand)-1]
		ai.gy = append(ai.gy, a)
		g.aiSay("descartou " + a.nm())
	}
	g.aiPublish()
	g.aiCheck()
	g.beginTurn()
}

// mana: terrenos desvirados + permanentes que geram mana (sem enjoo, se forem criaturas)
func (g *Game) aiMana() []*aiCard {
	var out []*aiCard
	for _, a := range ai.bf {
		if a.tap {
			continue
		}
		if a.is("Land") || (hasKw(a.text(), "{T}: Add") && !(a.is("Creature") && a.sick)) {
			out = append(out, a)
		}
	}
	return out
}

func (g *Game) aiCast() {
	for {
		src := g.aiMana()
		mana := len(src)
		pay := func(n int) {
			for k := 0; k < n && k < len(src); k++ {
				src[k].tap = true
			}
		}
		// comandante primeiro
		if len(ai.cmd) > 0 {
			a := ai.cmd[0]
			if cost := a.mv() + 2*ai.tax; cost <= mana {
				pay(cost)
				ai.cmd = ai.cmd[1:]
				ai.tax++
				a.sick = true
				ai.bf = append(ai.bf, a)
				g.aiSay("conjurou o comandante " + a.nm())
				continue
			}
		}
		var opts []int
		for k, a := range ai.hand {
			if !a.is("Land") && a.mv() <= mana && g.aiWorth(a) {
				opts = append(opts, k)
			}
		}
		if len(opts) == 0 {
			return
		}
		sort.Slice(opts, func(i, j int) bool { return ai.hand[opts[i]].mv() > ai.hand[opts[j]].mv() })
		k := opts[0]
		a := ai.hand[k]
		ai.hand = append(ai.hand[:k], ai.hand[k+1:]...)
		pay(a.mv())
		if a.is("Instant") || a.is("Sorcery") {
			g.aiSpell(a)
			ai.gy = append(ai.gy, a)
		} else {
			a.sick = true
			ai.bf = append(ai.bf, a)
			g.aiSay("conjurou " + a.nm())
		}
	}
}

// aiWorth: não gasta remoção sem alvo nem varre a mesa quando está ganhando
func (g *Game) aiWorth(a *aiCard) bool {
	t := strings.ToLower(a.text())
	if !(a.is("Instant") || a.is("Sorcery")) {
		return true
	}
	if strings.Contains(t, "destroy all creatures") || strings.Contains(t, "destroy all nonland") {
		mine, theirs := 0, 0
		for _, id := range g.myCreatures(false) {
			mine += max(1, g.fighterMine(id).pow)
		}
		for _, b := range ai.bf {
			if b.is("Creature") {
				theirs += max(1, b.pow())
			}
		}
		return mine >= theirs+3
	}
	if strings.Contains(t, "target creature") || strings.Contains(t, "target nonland permanent") || strings.Contains(t, "any target") {
		return len(g.myCreatures(false)) > 0
	}
	return true
}

func (g *Game) aiSpell(a *aiCard) {
	t := strings.ToLower(a.text())
	name := a.nm()
	best := func() int { // minha criatura mais forte
		ids := g.myCreatures(false)
		if len(ids) == 0 {
			return 0
		}
		sort.Slice(ids, func(i, j int) bool {
			fi, fj := g.fighterMine(ids[i]), g.fighterMine(ids[j])
			return fi.pow+fi.tou > fj.pow+fj.tou
		})
		return ids[0]
	}
	switch {
	case strings.Contains(t, "destroy all creatures") || strings.Contains(t, "destroy all nonland"):
		g.snap()
		for _, id := range g.myCreatures(false) {
			g.move(id, "graveyard", moveOpt{})
		}
		var keep []*aiCard
		for _, b := range ai.bf {
			if b.is("Creature") {
				ai.gy = append(ai.gy, b)
			} else {
				keep = append(keep, b)
			}
		}
		ai.bf = keep
		g.aiSay("conjurou " + name + ": destruiu todas as criaturas")
		g.announce("IA: " + name + " destruiu todas as criaturas")
	case (strings.Contains(t, "exile target creature") || strings.Contains(t, "exile target nonland")) && best() != 0:
		id := best()
		n := g.fighterMine(id).name
		g.snap()
		g.move(id, "exile", moveOpt{})
		g.aiSay("conjurou " + name + " e exilou " + n)
		g.announce("IA exilou " + n)
	case (strings.Contains(t, "destroy target creature") || strings.Contains(t, "destroy target nonland")) && best() != 0:
		id := best()
		n := g.fighterMine(id).name
		g.snap()
		g.move(id, "graveyard", moveOpt{})
		g.aiSay("conjurou " + name + " e destruiu " + n)
		g.announce("IA destruiu " + n)
	case strings.Contains(t, "draw two cards"):
		ai.draw(2)
		g.aiSay("conjurou " + name + " e comprou 2 cartas")
	case strings.Contains(t, "draw three cards"):
		ai.draw(3)
		g.aiSay("conjurou " + name + " e comprou 3 cartas")
	case strings.Contains(t, "draw a card"):
		ai.draw(1)
		g.aiSay("conjurou " + name + " e comprou 1 carta")
	default:
		g.aiSay("conjurou " + name + " (o efeito não é simulado)")
	}
}

/* ---------- combate ---------- */

func (g *Game) aiAttackers() []fighter {
	var mine []fighter
	for _, id := range g.myCreatures(true) {
		mine = append(mine, g.fighterMine(id))
	}
	var cand []fighter
	total := 0
	for _, a := range ai.bf {
		if !a.is("Creature") || a.tap || (a.sick && !a.kw("Haste")) {
			continue
		}
		f := fighterAI(a)
		if f.pow <= 0 || f.defen {
			continue
		}
		cand = append(cand, f)
		total += f.pow
	}
	if total >= g.S.Life && len(mine) == 0 { // ataque letal
		return cand
	}
	var out []fighter
	for _, f := range cand {
		safe := true
		for _, b := range mine {
			if canBlock(f, b) && kills(b, f) && !kills(f, b) {
				safe = false
			}
		}
		if safe {
			out = append(out, f)
		}
	}
	return out
}

// blockModal: a IA ataca; eu escolho quem bloqueia quem e o combate se resolve
var lastAtt []fighter

func (g *Game) blockModal(att []fighter) {
	lastAtt = att
	blk := make([]int, len(att)) // id da minha criatura bloqueando cada atacante (0 = ninguém)
	sum := 0
	for _, f := range att {
		sum += f.pow
	}
	names := []string{}
	for _, f := range att {
		names = append(names, f.name)
	}
	g.aiSay(fmt.Sprintf("ataca com %s (%d de dano)", strings.Join(names, ", "), sum))
	g.announce(fmt.Sprintf("A IA ataca: %d de dano", sum))
	done := false
	finish := func() { // fechar a janela sem confirmar conta como "sem bloqueios"
		if done {
			return
		}
		done = true
		g.resolveCombat(att, blk, true)
		g.aiEnd()
	}
	g.openModal(&Modal{title: "A IA está atacando você", w: 680, h: 140 + float64(len(att))*44 + 90, onClose: finish, draw: func(x, y, w, h float64) {
		txt(g.dst, "Toque no botão de cada atacante para escolher quem bloqueia (o mesmo bloqueador não serve duas vezes).", x, y, fReg, 13, cMut)
		y += 30
		mine := g.myCreatures(true)
		used := map[int]bool{}
		for _, b := range blk {
			used[b] = true
		}
		for k, f := range att {
			k, f := k, f
			kws := ""
			if f.fly {
				kws += " · voar"
			}
			if f.trample {
				kws += " · atropelar"
			}
			if f.death {
				kws += " · toque mortífero"
			}
			txt(g.dst, fmt.Sprintf("%s  %d/%d%s", f.name, f.pow, f.tou, kws), x, y+8, fBold, 14, cTxt)
			lab := "Sem bloqueio"
			if blk[k] != 0 {
				b := g.fighterMine(blk[k])
				lab = fmt.Sprintf("Bloqueia: %s %d/%d", b.name, b.pow, b.tou)
			}
			g.button(x+330, y, w-330, 32, lab, 0, func() { // passa para o próximo bloqueador possível
				opts := []int{0}
				for _, id := range mine {
					if (!used[id] || id == blk[k]) && canBlock(f, g.fighterMine(id)) {
						opts = append(opts, id)
					}
				}
				cur := 0
				for j, id := range opts {
					if id == blk[k] {
						cur = j
					}
				}
				blk[k] = opts[(cur+1)%len(opts)]
			})
			y += 44
		}
		y += 10
		g.button(x, y, 0, 36, "Confirmar bloqueios", 1, g.closeModal)
	}})
}

// resolveCombat: atk ataca; blk[k] é o bloqueador de atk[k] (meu id ou índice de carta da IA)
func (g *Game) resolveCombat(att []fighter, blkMine []int, aiAttacks bool) {
	g.snap()
	dmg := 0
	gain := 0
	var lines []string
	for k, f := range att {
		var b *fighter
		if aiAttacks {
			if blkMine[k] != 0 {
				bf := g.fighterMine(blkMine[k])
				b = &bf
			}
		}
		if b == nil {
			dmg += f.pow
			if f.life {
				gain += f.pow
			}
			continue
		}
		over := 0
		if f.trample && !b.death {
			over = max(0, f.pow-b.tou)
		}
		dmg += over
		if f.life {
			gain += f.pow
		}
		if kills(f, *b) {
			g.move(b.mine, "graveyard", moveOpt{})
			lines = append(lines, b.name+" morreu")
		}
		if kills(*b, f) {
			g.aiDies(f.ac)
			lines = append(lines, f.name+" (IA) morreu")
		}
	}
	for _, f := range att {
		if !f.vig && f.ac != nil {
			f.ac.tap = true
		}
	}
	g.S.Life -= dmg
	ai.life += gain
	msg := fmt.Sprintf("você recebeu %d de dano", dmg)
	if len(lines) > 0 {
		msg += "; " + strings.Join(lines, ", ")
	}
	g.log("combate contra a IA: %s", msg)
	g.announce(fmt.Sprintf("Combate: %d de dano em você", dmg))
	g.aiPublish()
	if g.S.Life <= 0 {
		g.aiSay("venceu a partida")
		g.announce("A IA venceu")
		ai.over = true
	}
}

func (g *Game) aiDies(a *aiCard) {
	for k, b := range ai.bf {
		if b == a {
			ai.bf = append(ai.bf[:k], ai.bf[k+1:]...)
			if a.cmdr {
				ai.cmd = append(ai.cmd, a) // comandante volta para a zona de comando
			} else {
				ai.gy = append(ai.gy, a)
			}
			return
		}
	}
}

// attackModal: eu escolho quem ataca a IA; ela decide os bloqueios
func (g *Game) attackModal() {
	if !aiActive() || ai.over || !g.playing() {
		return
	}
	pick := map[int]bool{}
	mine := g.myCreatures(true)
	if len(mine) == 0 {
		g.announce("Nenhuma criatura desvirada para atacar")
		return
	}
	g.openModal(&Modal{title: "Atacar a IA", w: 620, h: 130 + float64(len(mine))*40 + 70, draw: func(x, y, w, h float64) {
		txt(g.dst, "Escolha quem ataca. As atacantes viram; a IA decide os bloqueios.", x, y, fReg, 13, cMut)
		y += 30
		for _, id := range mine {
			id := id
			f := g.fighterMine(id)
			lab := fmt.Sprintf("%s  %d/%d", f.name, f.pow, f.tou)
			st := 0
			if pick[id] {
				lab = "✓ " + lab
				st = 1
			}
			g.button(x, y, w, 32, lab, st, func() { pick[id] = !pick[id] })
			y += 40
		}
		y += 8
		g.button(x, y, 0, 36, "Atacar", 1, func() {
			g.closeModal()
			var ids []int
			for _, id := range mine {
				if pick[id] {
					ids = append(ids, id)
				}
			}
			g.playerAttack(ids)
		})
	}})
}

func (g *Game) playerAttack(ids []int) {
	if len(ids) == 0 || ai.over {
		return
	}
	g.snap()
	var att []fighter
	for _, id := range ids {
		f := g.fighterMine(id)
		att = append(att, f)
		if !f.vig {
			g.S.Cards[id].Tapped = true
		}
	}
	// a IA bloqueia: troca boa, ou bloqueio de sacrifício se o ataque mataria
	sort.Slice(att, func(i, j int) bool { return att[i].pow > att[j].pow })
	total := 0
	for _, f := range att {
		total += f.pow
	}
	used := map[*aiCard]bool{}
	blockers := make([]*aiCard, len(att))
	var avail []*aiCard
	for _, a := range ai.bf {
		if a.is("Creature") && !a.tap {
			avail = append(avail, a)
		}
	}
	for k, f := range att {
		var best *aiCard
		for _, a := range avail {
			if used[a] || !canBlock(f, fighterAI(a)) {
				continue
			}
			b := fighterAI(a)
			if kills(b, f) && !kills(f, b) {
				best = a
				break
			}
		}
		if best == nil && total >= ai.life {
			for _, a := range avail { // precisa bloquear: usa a menor
				if !used[a] && canBlock(f, fighterAI(a)) && (best == nil || a.pow()+a.tou() < best.pow()+best.tou()) {
					best = a
				}
			}
		}
		if best != nil {
			used[best] = true
			blockers[k] = best
			total -= f.pow
		}
	}
	dmg, gain := 0, 0
	var lines []string
	for k, f := range att {
		if f.life {
			gain += f.pow
		}
		a := blockers[k]
		if a == nil {
			dmg += f.pow
			continue
		}
		b := fighterAI(a)
		lines = append(lines, b.name+" bloqueou "+f.name)
		if f.trample && !b.death {
			dmg += max(0, f.pow-b.tou)
		}
		if kills(f, b) {
			g.aiDies(a)
			lines = append(lines, b.name+" (IA) morreu")
		}
		if kills(b, f) {
			g.move(f.mine, "graveyard", moveOpt{})
			lines = append(lines, f.name+" morreu")
		}
	}
	ai.life -= dmg
	g.S.Life += gain
	msg := fmt.Sprintf("a IA recebeu %d de dano", dmg)
	if len(lines) > 0 {
		msg += "; " + strings.Join(lines, ", ")
	}
	g.log("ataque na IA: %s", msg)
	g.announce(fmt.Sprintf("A IA recebeu %d de dano", dmg))
	g.aiPublish()
	g.aiCheck()
}

func (g *Game) aiCheck() {
	if ai.life <= 0 && !ai.over {
		ai.over = true
		g.aiSay("perdeu a partida")
		g.announce("Você venceu a IA!")
	}
}
func (g *Game) aiLose() {
	ai.over = true
	g.announce("Você venceu a IA!")
}

// aiStop sai do modo IA (sair da partida, trocar de deck, entrar na rede)
func aiStop() {
	if !ai.on {
		return
	}
	ai = aiPlayer{}
	nw.mu.Lock()
	if nw.mode == "ia" {
		nw.mode, nw.status = "", ""
		nw.peers = map[int]*Peer{}
	}
	nw.mu.Unlock()
}
