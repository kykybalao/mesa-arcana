package main

import (
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

type Inst struct {
	ID       int
	Cid      string
	Zone     string
	Tapped   bool
	Down     bool
	Token    bool
	Cmdr     bool
	Face     int
	Counters map[string]int
	X, Y     float64
	Att      int // id da carta em que esta está anexada/equipada (0 = solta)
}

type State struct {
	N     int
	Cards map[int]*Inst
	Z     map[string][]int
	Life  int
	Opp   [3]int
	Trk   map[string]int // marcadores do jogador: veneno, energia, experiência, dano de comandante
	Turn  int
	Tax   map[string]int
	Mulls int
	Phase string // mull, bottom, play
	Step  int    // fase do turno (índice em stepNames)
	Pick  []int
	Log   []string
}

var stepNames = []string{"Manutenção", "Compra", "Principal 1", "Combate", "Principal 2", "Final"}

var zoneName = map[string]string{"library": "Grimório", "hand": "Mão", "battlefield": "Campo", "graveyard": "Cemitério", "exile": "Exílio", "command": "Zona de comando"}

func rnd(n int) int {
	if n <= 1 {
		return 0
	}
	v, err := crand.Int(crand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}
func shuffle(a []int) {
	for i := len(a) - 1; i > 0; i-- {
		j := rnd(i + 1)
		a[i], a[j] = a[j], a[i]
	}
}
func remove(a []int, id int) []int {
	out := a[:0:0]
	for _, x := range a {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}
func contains(a []int, id int) bool {
	for _, x := range a {
		if x == id {
			return true
		}
	}
	return false
}

func (g *Game) card(i *Inst) *Card {
	if c := cardByID[i.Cid]; c != nil {
		return c
	}
	if c := g.customs[i.Cid]; c != nil {
		return c
	}
	return &Card{ID: i.Cid, Face: Face{N: "?"}}
}
func (g *Game) face(i *Inst) Face { return g.card(i).FaceAt(i.Face) }

func (g *Game) snap() {
	b, _ := json.Marshal(g.S)
	g.undo = append(g.undo, b)
	if len(g.undo) > 80 {
		g.undo = g.undo[1:]
	}
}
func (g *Game) undoLast() {
	if len(g.undo) == 0 {
		return
	}
	var s State
	if json.Unmarshal(g.undo[len(g.undo)-1], &s) == nil {
		g.S = &s
	}
	g.undo = g.undo[:len(g.undo)-1]
}
func (g *Game) log(f string, a ...any) {
	g.logP("", f, a...)
}

// logP registra no meu histórico; para a mesa vai pub (ou o mesmo texto, se pub for vazio; "-" não manda nada)
func (g *Game) logP(pub string, f string, a ...any) {
	line := fmt.Sprintf(f, a...)
	if pub == "" {
		pub = line
	}
	if pub != "-" {
		g.pubLog(pub)
	}
	g.S.Log = append(g.S.Log, fmt.Sprintf("T%d: ", g.S.Turn)+line)
	if len(g.S.Log) > 200 {
		g.S.Log = g.S.Log[1:]
	}
}
func (g *Game) mk(cid, zone string) *Inst {
	s := g.S
	s.N++
	i := &Inst{ID: s.N, Cid: cid, Zone: zone, Counters: map[string]int{}}
	s.Cards[i.ID] = i
	s.Z[zone] = append(s.Z[zone], i.ID)
	return i
}

func (g *Game) startGame(d *Deck) {
	g.deck = d
	g.undo = nil
	g.showHand = false
	g.S = &State{Cards: map[int]*Inst{}, Z: map[string][]int{"library": {}, "hand": {}, "battlefield": {}, "graveyard": {}, "exile": {}, "command": {}},
		Life: 40, Opp: [3]int{40, 40, 40}, Turn: 1, Tax: map[string]int{}, Phase: "mull"}
	if len(d.Cmd) == 0 { // deck sem comandante (ex.: 40/60 cartas): vida normal de 20
		g.S.Life, g.S.Opp = 20, [3]int{20, 20, 20}
	}
	cmd := map[string]bool{}
	for _, id := range d.Cmd {
		cmd[id] = true
	}
	for _, e := range d.Entries {
		for k := 0; k < e.Qty; k++ {
			if cmd[e.Card.ID] && k == 0 {
				g.mk(e.Card.ID, "command").Cmdr = true
			} else {
				g.mk(e.Card.ID, "library")
			}
		}
	}
	shuffle(g.S.Z["library"])
	for k := 0; k < 7; k++ {
		g.drawRaw()
	}
	g.log("Nova partida. Mão inicial de 7.")
	g.started, g.saved = time.Now(), false
	g.screen = "game"
	g.modal, g.ctx = nil, nil
}
func (g *Game) drawRaw() bool {
	s := g.S
	if len(s.Z["library"]) == 0 {
		return false
	}
	id := s.Z["library"][0]
	s.Z["library"] = s.Z["library"][1:]
	s.Cards[id].Zone = "hand"
	s.Z["hand"] = append(s.Z["hand"], id)
	return true
}
func (g *Game) drawN(n int) {
	if g.denied() {
		return
	}
	g.snap()
	k := 0
	for i := 0; i < n; i++ {
		if g.drawRaw() {
			k++
		}
	}
	if k > 0 {
		g.log("comprou %d", k)
	} else {
		g.log("grimório vazio!")
	}
}

// posição automática no campo: criaturas em cima, terrenos embaixo, o resto no meio
func (g *Game) autoPos(i *Inst) (float64, float64) {
	W, H := float64(g.bf.Dx()), float64(g.bf.Dy())
	if W < 100 {
		W, H = 900, 500
	}
	if fieldMode() == 1 {
		return g.zonePos(i, W, H)
	}
	cw := g.cw
	ch := cw * ratio
	t := g.face(i).T
	y0 := max(8, (H-ch)/2)
	if strings.Contains(t, "Land") {
		y0 = max(8, H-ch-10)
	} else if strings.Contains(t, "Creature") {
		y0 = 8 + ch*0.24 // sobra espaço em cima para equipamentos aparecerem
	}
	per := max(1, int((W-16)/(cw+8)))
	taken := map[int]bool{}
	for _, id := range g.S.Z["battlefield"] {
		o := g.S.Cards[id]
		if o != i && abs(o.Y-y0) < ch*0.45 {
			taken[int((o.X-10)/(cw+8)+0.5)] = true
		}
	}
	n := 0
	for taken[n] && n < per*3 {
		n++
	}
	return 10 + float64(n%per)*(cw+8), y0 + float64(n/per)*26
}
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

type moveOpt struct {
	bottom bool
	pos    bool
	x, y   float64
}

func (g *Game) move(id int, zone string, o moveOpt) {
	s := g.S
	i := s.Cards[id]
	if i == nil {
		return
	}
	from := i.Zone
	s.Z[from] = remove(s.Z[from], id)
	name := g.card(i).N
	if i.Down {
		name = "carta virada para baixo"
	}
	if i.Token && zone != "battlefield" {
		delete(s.Cards, id)
		g.log("ficha %s saiu do jogo", g.card(i).N)
		return
	}
	if from == "battlefield" && zone != "battlefield" {
		i.Tapped, i.Face, i.Down, i.Att = false, 0, false, 0
		i.Counters = map[string]int{}
	}
	if zone != "battlefield" && zone != from {
		i.Down = false
	}
	i.Zone = zone
	if zone == "library" && !o.bottom {
		s.Z[zone] = append([]int{id}, s.Z[zone]...)
	} else {
		s.Z[zone] = append(s.Z[zone], id)
	}
	if zone == "battlefield" {
		if o.pos {
			i.X, i.Y, i.Att = o.x, o.y, 0 // arrastar uma carta anexada solta ela
		} else if from != "battlefield" {
			i.X, i.Y = g.autoPos(i)
		}
		if from != "battlefield" {
			if l, err := strconv.Atoi(g.face(i).L); err == nil {
				i.Counters["Lealdade"] = l
			}
		}
	}
	if from != zone {
		extra := ""
		if zone == "library" {
			extra = " (topo)"
			if o.bottom {
				extra = " (fundo)"
			}
		}
		pub := ""
		hidden := func(z string) bool { return z == "library" || z == "hand" }
		if hidden(from) && hidden(zone) { // a mesa não fica sabendo qual carta andou entre mão e grimório
			pub = fmt.Sprintf("uma carta: %s → %s%s", zoneName[from], zoneName[zone], extra)
		}
		g.logP(pub, "%s: %s → %s%s", name, zoneName[from], zoneName[zone], extra)
	}
}
func (g *Game) untapAll() {
	if g.denied() {
		return
	}
	g.snap()
	for _, id := range g.S.Z["battlefield"] {
		g.S.Cards[id].Tapped = false
	}
	g.log("desvirou tudo")
}
func (g *Game) nextTurn() {
	if g.S.Phase != "play" {
		return
	}
	if g.hasOrder() { // com a ordem da mesa decidida, o botão passa a vez; o meu turno começa quando ela volta
		if g.myTurn() {
			g.passTurn()
		} else {
			g.announce("Vez de " + g.table.active())
		}
		return
	}
	if aiActive() { // contra a IA: ela joga o turno dela e depois começa o meu
		g.aiTurn()
		return
	}
	g.beginTurn()
}

// beginTurn começa um turno meu: desvira tudo e anuncia a manutenção (nada mais acontece sozinho)
func (g *Game) beginTurn() {
	g.turnStart = time.Now()
	g.snap()
	for _, id := range g.S.Z["battlefield"] {
		g.S.Cards[id].Tapped = false
	}
	g.S.Turn++
	g.S.Step = 0
	g.log("— turno %d — desvirou tudo. Manutenção", g.S.Turn)
	g.announce(fmt.Sprintf("Turno %d · Manutenção", g.S.Turn))
}

// as fases são só anunciadas: nada acontece sozinho (nem a compra), por causa dos gatilhos
func (g *Game) setStep(k int) {
	if g.S.Phase != "play" || k == g.S.Step || k < 0 || k >= len(stepNames) || g.denied() {
		return
	}
	g.snap()
	g.S.Step = k
	g.log("fase: %s", stepNames[k])
	g.announce(stepNames[k])
}
func (g *Game) nextStep() {
	if g.S.Phase != "play" {
		return
	}
	if g.S.Step >= len(stepNames)-1 {
		g.nextTurn()
		return
	}
	g.setStep(g.S.Step + 1)
}
func (g *Game) announce(s string) { g.banner, g.bannerT = s, 80 }

// attach anexa/equipa a carta src na carta dst (as duas no campo)
func (g *Game) attach(src, dst int) {
	a, b := g.S.Cards[src], g.S.Cards[dst]
	if a == nil || b == nil || src == dst || a.Zone != "battlefield" || b.Zone != "battlefield" {
		return
	}
	if b.Att != 0 { // alvo já anexado em outra: vai para a mesma carta de base
		dst = b.Att
		b = g.S.Cards[dst]
		if b == nil || dst == src {
			return
		}
	}
	g.snap()
	a = g.S.Cards[src]
	for _, id := range g.S.Z["battlefield"] { // o que estava anexado em src passa a ficar solto
		if o := g.S.Cards[id]; o.Att == src {
			o.Att = 0
		}
	}
	a.Att = dst
	g.log("%s anexada em %s", g.card(a).N, g.card(g.S.Cards[dst]).N)
}

// syncAtt mantém as cartas anexadas coladas na carta de base, aparecendo por trás dela
func (g *Game) syncAtt() {
	s := g.S
	bf := s.Z["battlefield"]
	n := map[int]int{}
	any := false
	for _, id := range bf {
		i := s.Cards[id]
		if i.Att == 0 {
			continue
		}
		h := s.Cards[i.Att]
		if h == nil || h.Zone != "battlefield" || h.Att != 0 {
			i.Att = 0
			continue
		}
		any = true
		n[i.Att]++
		k := float64(n[i.Att])
		i.X, i.Y = h.X+g.cw*0.10*k, h.Y-g.cw*ratio*0.115*k
	}
	if !any {
		return
	}
	out := make([]int, 0, len(bf))
	for _, id := range bf {
		if s.Cards[id].Att != 0 {
			continue
		}
		var att []int
		for _, a := range bf {
			if s.Cards[a].Att == id {
				att = append(att, a)
			}
		}
		for k := len(att) - 1; k >= 0; k-- {
			out = append(out, att[k])
		}
		out = append(out, id)
	}
	s.Z["battlefield"] = out
}
func (g *Game) shuffleLib() {
	g.snap()
	shuffle(g.S.Z["library"])
	g.log("embaralhou o grimório")
}
func (g *Game) addToken(cid string, n int) {
	g.snap()
	for k := 0; k < n; k++ {
		i := g.mk(cid, "battlefield")
		i.Token = true
		i.X, i.Y = g.autoPos(i)
		if l, err := strconv.Atoi(g.face(i).L); err == nil {
			i.Counters["Lealdade"] = l
		}
	}
	c := cardByID[cid]
	if c == nil {
		c = g.customs[cid]
	}
	g.log("criou %d ficha(s) %s", n, c.N)
}
func (g *Game) tidy() {
	g.snap()
	ids := append([]int{}, g.S.Z["battlefield"]...)
	for _, id := range ids {
		g.S.Cards[id].X, g.S.Cards[id].Y = -9999, -9999
	}
	for _, id := range ids {
		if g.S.Cards[id].Att == 0 {
			g.S.Cards[id].X, g.S.Cards[id].Y = g.autoPos(g.S.Cards[id])
		}
	}
}

/* mulligan de Londres, o primeiro é grátis */
func (g *Game) bottomCount() int { return max(0, g.S.Mulls-1) }
func (g *Game) mulligan() {
	g.snap()
	s := g.S
	for _, id := range s.Z["hand"] {
		s.Cards[id].Zone = "library"
		s.Z["library"] = append(s.Z["library"], id)
	}
	s.Z["hand"] = []int{}
	shuffle(s.Z["library"])
	for k := 0; k < 7; k++ {
		g.drawRaw()
	}
	s.Mulls++
	g.log("mulligan nº %d", s.Mulls)
}
func (g *Game) keep() {
	g.snap()
	if g.bottomCount() > 0 {
		g.S.Phase, g.S.Pick = "bottom", []int{}
	} else {
		g.S.Phase = "play"
		g.log("manteve a mão")
	}
}
func (g *Game) confirmBottom() {
	if len(g.S.Pick) != g.bottomCount() {
		return
	}
	g.snap()
	for _, id := range g.S.Pick {
		g.move(id, "library", moveOpt{bottom: true})
	}
	g.S.Pick = nil
	g.S.Phase = "play"
	g.log("manteve com %d cartas", len(g.S.Z["hand"]))
}
