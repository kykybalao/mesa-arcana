package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Análise local do deck: bracket estimada e combos, sem internet.
// Combos: banco do Commander Spellbook (a popularidade deles vem dos decks do EDHREC).
// Listas de Game Changers, turnos extras, destruição em massa de terrenos e tutores: Scryfall.

//go:embed dados/analise.json.gz
var anGz []byte

type Combo struct {
	ID string   `json:"id"`
	C  []string `json:"c"`
	P  []string `json:"p"`
	B  string   `json:"b"`
	I  string   `json:"i"`
	N  int      `json:"n"`
	T  []string `json:"t"`
	R  string   `json:"r"`
	M  string   `json:"m"`
	D  string   `json:"d"` // passo a passo (texto do Commander Spellbook)
	E  string   `json:"e"` // o que precisa estar pronto antes
}

// comboHow devolve o que o combo precisa e os passos, um por linha
func comboHow(c *Combo) (need string, steps []string) {
	var ns []string
	for _, x := range []string{c.E, c.R} {
		if x = strings.TrimSpace(x); x != "" {
			ns = append(ns, strings.TrimRight(x, "."))
		}
	}
	if c.M != "" {
		ns = append(ns, "mana disponível: "+c.M)
	}
	need = strings.Join(ns, " · ")
	for _, l := range strings.Split(c.D, "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-•*"))
		if l != "" {
			steps = append(steps, l)
		}
	}
	return
}

var (
	anOnce   sync.Once
	combos   []Combo
	comboIdx map[string][]int32
	anLists  = map[string]map[string]bool{}
	anDate   string
)

func loadAnalysis() {
	anOnce.Do(func() {
		raw := anGz
		if b, err := os.ReadFile(filepath.Join(baseDir, "dados", "analise.json.gz")); err == nil {
			raw = b
		}
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return
		}
		var db struct {
			Data   string              `json:"data"`
			Combos []Combo             `json:"combos"`
			Listas map[string][]string `json:"listas"`
		}
		if json.NewDecoder(zr).Decode(&db) != nil {
			return
		}
		anDate, combos = db.Data, db.Combos
		comboIdx = map[string][]int32{}
		for i, c := range combos {
			for _, n := range c.C {
				k := nameKey(n)
				comboIdx[k] = append(comboIdx[k], int32(i))
			}
		}
		for k, l := range db.Listas {
			m := map[string]bool{}
			for _, n := range l {
				m[nameKey(n)] = true
			}
			anLists[k] = m
		}
	})
}

// nameKey: nome em minúsculas, só a face da frente, sem a marca de "precisa ser o comandante"
func nameKey(n string) string {
	n = strings.ToLower(strings.TrimSuffix(n, "*"))
	if i := strings.Index(n, " // "); i > 0 {
		n = n[:i]
	}
	return n
}

type Almost struct {
	Combo   *Combo
	Missing string
}
type Analysis struct {
	Bracket int
	Reasons []string
	GC      []string
	MLD     []string
	ET      []string
	Tutors  []string
	In      []*Combo // todas as peças estão no deck
	InTpl   []*Combo // peças no deck, mas pede também uma carta genérica (ex.: "um jeito de ganhar vida")
	Almost  []Almost // falta exatamente 1 carta
	Two     int      // combos de 2 cartas no deck
}

var bracketNames = []string{"", "Exhibition", "Core", "Upgraded", "Optimized", "cEDH"}
var tagBracket = map[string]int{"E": 1, "C": 2, "O": 1, "P": 3, "S": 3, "R": 4}
var tagName = map[string]string{"E": "Exhibition", "C": "Core", "O": "Oddball", "P": "Powerful", "S": "Spicy", "R": "Ruthless"}

var (
	etRe   = regexp.MustCompile(`(?i)takes? (an|two|three|\w+) extra turns?|extra turn after this one`)
	mldRe  = regexp.MustCompile(`(?i)destroy all lands|each player sacrifices all lands|destroy all (non-?basic )?lands|lands don't untap during`)
	symRe  = regexp.MustCompile(`\{([^}]+)\}`)
	wubrgs = "WUBRG"
)

func cardText(c *Card) string {
	var b strings.Builder
	for _, f := range c.Faces() {
		b.WriteString(f.C + " " + f.X + "\n")
	}
	return b.String()
}

// identity estima a identidade de cor pelas cores no custo e no texto
func identity(cs []*Card) string {
	has := map[byte]bool{}
	for _, c := range cs {
		for _, m := range symRe.FindAllStringSubmatch(cardText(c), -1) {
			for i := 0; i < len(m[1]); i++ {
				if strings.IndexByte(wubrgs, m[1][i]) >= 0 {
					has[m[1][i]] = true
				}
			}
		}
	}
	out := ""
	for i := 0; i < 5; i++ {
		if has[wubrgs[i]] {
			out += wubrgs[i : i+1]
		}
	}
	return out
}
func subset(a, b string) bool {
	for i := 0; i < len(a); i++ {
		if strings.IndexByte(wubrgs, a[i]) >= 0 && strings.IndexByte(b, a[i]) < 0 {
			return false
		}
	}
	return true
}

// analyze aplica os critérios oficiais das brackets de Commander às cartas do deck
func analyze(cards []*Card, cmd []*Card) *Analysis {
	loadAnalysis()
	a := &Analysis{}
	have := map[string]bool{}
	isCmd := map[string]bool{}
	for _, c := range cmd {
		isCmd[nameKey(c.N)] = true
	}
	seen := map[string]bool{}
	for _, c := range cards {
		k := nameKey(c.N)
		if seen[k] {
			continue
		}
		seen[k] = true
		have[k] = true
		txt := cardText(c)
		if anLists["gc"][k] {
			a.GC = append(a.GC, c.N)
		}
		if anLists["mld"][k] || mldRe.MatchString(txt) {
			a.MLD = append(a.MLD, c.N)
		}
		if anLists["et"][k] || etRe.MatchString(txt) {
			a.ET = append(a.ET, c.N)
		}
		if anLists["tutor"][k] {
			a.Tutors = append(a.Tutors, c.N)
		}
	}
	idCards := cmd
	if len(idCards) == 0 {
		idCards = cards
	}
	ident := identity(idCards)
	// combos: conta quantas peças de cada combo estão no deck
	cnt := map[int32]int{}
	for k := range have {
		for _, ci := range comboIdx[k] {
			cnt[ci]++
		}
	}
	for ci, n := range cnt {
		c := &combos[ci]
		if n < len(c.C)-1 {
			continue
		}
		miss, ok := "", true
		for _, p := range c.C {
			k := nameKey(p)
			if !have[k] {
				miss = strings.TrimSuffix(p, "*")
			} else if strings.HasSuffix(p, "*") && !isCmd[k] {
				ok = false // essa peça precisa ser o comandante
			}
		}
		if !ok {
			continue
		}
		switch {
		case miss == "" && len(c.T) == 0:
			a.In = append(a.In, c)
			if len(c.C) == 2 {
				a.Two++
			}
		case miss == "":
			a.InTpl = append(a.InTpl, c)
		case len(c.T) == 0 && n == len(c.C)-1 && subset(c.I, ident):
			a.Almost = append(a.Almost, Almost{c, miss})
		}
	}
	byPop := func(l []*Combo) {
		sort.SliceStable(l, func(x, y int) bool {
			if l[x].N != l[y].N {
				return l[x].N > l[y].N
			}
			return l[x].ID < l[y].ID
		})
	}
	byPop(a.In)
	byPop(a.InTpl)
	sort.SliceStable(a.Almost, func(x, y int) bool {
		if a.Almost[x].Combo.N != a.Almost[y].Combo.N {
			return a.Almost[x].Combo.N > a.Almost[y].Combo.N
		}
		return a.Almost[x].Combo.ID < a.Almost[y].Combo.ID
	})

	// critérios oficiais (Game Changers, terrenos, turnos extras, combos de 2 cartas)
	b := 1
	up := func(n int, why string) {
		if n > b {
			b = n
		}
		a.Reasons = append(a.Reasons, fmt.Sprintf("[%d] %s", n, why))
	}
	switch g := len(a.GC); {
	case g >= 4:
		up(4, fmt.Sprintf("%d Game Changers (a bracket 3 aceita no máximo 3)", g))
	case g >= 1:
		up(3, fmt.Sprintf("%d Game Changer(s) (as brackets 1 e 2 não aceitam nenhum)", g))
	}
	if n := len(a.MLD); n > 0 {
		up(4, fmt.Sprintf("%d carta(s) de destruição/negação em massa de terrenos (só na bracket 4 ou 5)", n))
	}
	switch n := len(a.ET); {
	case n >= 2:
		up(4, fmt.Sprintf("%d cartas de turno extra (dá para encadear turnos; só na bracket 4 ou 5)", n))
	case n == 1:
		up(2, "1 carta de turno extra (a bracket 1 não usa turnos extras)")
	}
	r, s, oth := 0, 0, 0
	for _, c := range a.In {
		switch tagBracket[c.B] {
		case 4:
			r++
		case 3:
			s++
		default:
			oth++
		}
	}
	if r > 0 {
		up(4, fmt.Sprintf("%d combo(s) que o Commander Spellbook classifica como Ruthless (combo de 2 cartas cedo no jogo, trava ou similar)", r))
	}
	if s > 0 {
		up(3, fmt.Sprintf("%d combo(s) de fim de jogo (Spicy/Powerful): a bracket 2 não aceita combos infinitos de 2 cartas", s))
	}
	note := ""
	if oth > 0 {
		note = fmt.Sprintf("Obs.: há %d combo(s) de 3 ou mais cartas ou de baixo impacto. Pelas regras eles não sobem a bracket, mas um deck com vários combos costuma jogar acima da bracket 2: vale avisar a mesa.", oth)
	}
	defer func() {
		if note != "" {
			a.Reasons = append(a.Reasons, note)
		}
	}()
	if len(a.Reasons) == 0 {
		a.Reasons = append(a.Reasons, "Nenhum Game Changer, destruição em massa de terrenos, turno extra ou combo de 2 cartas: pelas cartas o deck cabe na bracket 1 ou 2. A diferença entre elas é a intenção (deck temático ou deck no nível de um pré-construído).")
	}
	a.Bracket = b
	return a
}

func (a *Analysis) title() string {
	if a.Bracket <= 1 {
		return "Bracket 1 ou 2 (Exhibition / Core)"
	}
	return fmt.Sprintf("Bracket %d (%s)", a.Bracket, bracketNames[a.Bracket])
}

func comboLine(c *Combo) string {
	s := strings.ReplaceAll(strings.Join(c.C, " + "), "*", " (comandante)")
	if len(c.T) > 0 {
		s += " + [" + strings.Join(c.T, "; ") + "]"
	}
	return s
}
func comboResult(c *Combo) string {
	s := "→ " + strings.Join(c.P, ", ")
	if t := tagName[c.B]; t != "" {
		s += "  · " + t
	}
	if c.R != "" {
		s += "  · precisa: " + c.R
	}
	return s
}

/* ---------- conferência online (opcional, só onde o Commander Spellbook não é bloqueado) ---------- */

type onlineCheck struct {
	mu      sync.Mutex
	running bool
	msg     string
}

var online onlineCheck

func (o *onlineCheck) get() (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.msg, o.running
}
func (o *onlineCheck) start(cards []*Card, cmd []*Card) {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	o.running, o.msg = true, ""
	o.mu.Unlock()
	go func() {
		msg := spellbookOnline(cards, cmd)
		o.mu.Lock()
		o.running, o.msg = false, msg
		o.mu.Unlock()
	}()
}
func spellbookOnline(cards []*Card, cmd []*Card) string {
	type cn struct {
		Card string `json:"card"`
	}
	body := struct {
		Commanders []cn `json:"commanders"`
		Main       []cn `json:"main"`
	}{Commanders: []cn{}, Main: []cn{}}
	isCmd := map[string]bool{}
	for _, c := range cmd {
		isCmd[c.ID] = true
		body.Commanders = append(body.Commanders, cn{c.N})
	}
	for _, c := range cards {
		if !isCmd[c.ID] {
			body.Main = append(body.Main, cn{c.N})
		}
	}
	b, _ := json.Marshal(body)
	cl := &http.Client{Timeout: 40 * time.Second}
	post := func(path string, out any) error {
		req, _ := http.NewRequest("POST", "https://backend.commanderspellbook.com/"+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "MesaArcana/1.0")
		res, err := cl.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("HTTP %d", res.StatusCode)
		}
		raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, out)
	}
	var est struct {
		BracketTag string `json:"bracketTag"`
	}
	if err := post("estimate-bracket/", &est); err != nil {
		return "Sem acesso ao Commander Spellbook nesta rede (" + shortErr(err) + "). A análise local acima continua valendo."
	}
	var fm struct {
		Results struct {
			Included       []json.RawMessage `json:"included"`
			AlmostIncluded []json.RawMessage `json:"almostIncluded"`
		} `json:"results"`
	}
	extra := ""
	if err := post("find-my-combos/", &fm); err == nil {
		extra = fmt.Sprintf(" · %d combos no deck · %d a 1 carta", len(fm.Results.Included), len(fm.Results.AlmostIncluded))
	}
	return fmt.Sprintf("Commander Spellbook agora: %s (bracket %d)%s", tagName[est.BracketTag], tagBracket[est.BracketTag], extra)
}

/* ---------- janela da análise ---------- */

func (g *Game) analysisModal(name string, cards []*Card, cmd []*Card) {
	a := analyze(cards, cmd)
	online.mu.Lock()
	online.msg = ""
	online.mu.Unlock()
	type ln struct {
		s    string
		kind int // 0 texto, 1 título, 2 fraco, 3 destaque
	}
	var lines []ln
	built := 0.0
	showAlmost, builtAlmost := false, false
	build := func(w float64) {
		if built == w && builtAlmost == showAlmost {
			return
		}
		built, builtAlmost = w, showAlmost
		lines = lines[:0]
		add := func(kind int, s string) {
			src, size := fReg, 13.5
			if kind == 1 {
				src, size = fBold, 14.5
			}
			for _, l := range wrap(s, src, size, w) {
				lines = append(lines, ln{l, kind})
			}
		}
		list := func(title string, l []string) {
			if len(l) > 0 {
				add(1, fmt.Sprintf("%s (%d)", title, len(l)))
				add(0, strings.Join(l, " · "))
				add(0, "")
			}
		}
		add(1, "Por que essa bracket")
		for _, r := range a.Reasons {
			add(0, r)
		}
		add(2, "O número entre colchetes é a bracket mínima que cada critério exige; vale o maior. A bracket 5 (cEDH) depende da intenção de jogar no nível competitivo, então nenhum programa detecta só pelas cartas.")
		add(0, "")
		list("Game Changers", a.GC)
		list("Destruição/negação em massa de terrenos", a.MLD)
		list("Turnos extras", a.ET)
		list("Tutores e buscas no grimório (não mudam a bracket)", a.Tutors)
		add(1, fmt.Sprintf("Combos completos no deck (%d)", len(a.In)))
		if len(a.In) == 0 {
			add(2, "Nenhum combo conhecido com todas as peças no deck.")
		}
		for k, c := range a.In {
			if k >= 40 {
				add(2, fmt.Sprintf("… e mais %d.", len(a.In)-40))
				break
			}
			add(3, fmt.Sprintf("%d) %s", k+1, comboLine(c)))
			add(2, "   Resultado: "+strings.TrimPrefix(comboResult(c), "→ "))
			need, steps := comboHow(c)
			if need != "" {
				add(0, "   Antes de começar: "+need)
			}
			if len(steps) > 0 {
				add(0, "   Como fazer:")
				for n, st := range steps {
					add(0, fmt.Sprintf("      %d. %s", n+1, st))
				}
			}
			add(0, "")
		}
		if len(a.In) > 0 && a.In[0].D != "" {
			add(2, "Passos do Commander Spellbook, traduzidos automaticamente. Os que o programa não soube traduzir ficam em inglês.")
		}
		add(0, "")
		if len(a.InTpl) > 0 {
			add(1, fmt.Sprintf("Combos que pedem mais uma carta genérica (%d)", len(a.InTpl)))
			add(2, "As cartas nomeadas estão no deck; entre colchetes está o tipo de carta que completa o combo.")
			for k, c := range a.InTpl {
				if k >= 20 {
					add(2, fmt.Sprintf("… e mais %d.", len(a.InTpl)-20))
					break
				}
				add(3, comboLine(c))
				add(2, "   "+comboResult(c))
			}
			add(0, "")
		}
		if !showAlmost {
			add(2, fmt.Sprintf("Quase combos escondidos: %d combos a 1 carta de distância. Use o botão “Mostrar o que falta 1 carta” aqui embaixo para ver.", len(a.Almost)))
		} else {
			add(1, fmt.Sprintf("Quase combos: falta 1 carta (%d)", len(a.Almost)))
			if len(a.Almost) == 0 {
				add(2, "Nenhum combo a 1 carta de distância dentro das cores do comandante.")
			}
		}
		for k, al := range a.Almost {
			if !showAlmost {
				break
			}
			if k >= 30 {
				add(2, fmt.Sprintf("… e mais %d (mostrando os mais populares).", len(a.Almost)-30))
				break
			}
			add(3, "Falta "+al.Missing+":  "+comboLine(al.Combo))
			add(2, "   "+comboResult(al.Combo))
		}
		add(0, "")
		add(2, fmt.Sprintf("Fontes: critérios oficiais das brackets de Commander; combos e classificação do Commander Spellbook (%d combos, ordenados pela popularidade nos decks do EDHREC); listas do Scryfall; dados de %s.", len(combos), anDate))
	}
	m := &Modal{title: "Análise de " + name, w: 980, h: 760}
	m.draw = func(x, y, w, h float64) {
		col := cAcc2
		if a.Bracket >= 4 {
			col = cBad
		} else if a.Bracket == 3 {
			col = cAcc
		}
		txt(g.dst, a.title(), x, y, fBold, 24, col)
		txt(g.dst, fmt.Sprintf("%d Game Changers · %d combos no deck (%d de 2 cartas) · %d quase combos", len(a.GC), len(a.In), a.Two, len(a.Almost)), x, y+34, fReg, 13.5, cMut)
		top := y + 62
		lh := 19.0
		listH := h - 62 - 44
		build(w - 16)
		m.scroll = max(0, min(m.scroll, float64(len(lines))*lh-listH))
		g.add(region{x: x, y: top, w: w, h: listH, wheel: func(dy float64) { m.scroll -= dy * lh * 3 }})
		old := g.dst
		g.dst = clip(old, x, top, w, listH)
		for n, l := range lines {
			yy := top + float64(n)*lh - m.scroll
			if yy+lh < top || yy > top+listH {
				continue
			}
			switch l.kind {
			case 1:
				txt(g.dst, l.s, x, yy, fBold, 14.5, cAcc)
			case 2:
				txt(g.dst, l.s, x, yy, fReg, 13.5, cMut)
			default:
				txt(g.dst, l.s, x, yy, fReg, 13.5, cTxt)
			}
		}
		g.dst = old
		if tot := float64(len(lines)) * lh; tot > listH {
			bh := listH * listH / tot
			rrect(g.dst, x+w-4, top+(listH-bh)*m.scroll/(tot-listH), 4, bh, 2, cLine)
		}
		msg, run := online.get()
		lab := "Conferir online no Commander Spellbook"
		if run {
			lab = "Consultando…"
		}
		bw := g.button(x, y+h-32, 0, 30, lab, 0, func() { online.start(cards, cmd) })
		lab2 := "Mostrar o que falta 1 carta"
		if showAlmost {
			lab2 = "Esconder o que falta 1 carta"
		}
		bw += 8 + g.button(x+bw+8, y+h-32, 0, 30, lab2, 0, func() { showAlmost = !showAlmost })
		txt(g.dst, fit(msg, fReg, 12.5, w-bw-120), x+bw+10, y+h-25, fReg, 12.5, cAcc2)
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}
	g.openModal(m)
}

func deckCards(d *Deck) (cards []*Card, cmd []*Card) {
	for _, e := range d.Entries {
		cards = append(cards, e.Card)
	}
	for _, id := range d.Cmd {
		if c := cardByID[id]; c != nil {
			cmd = append(cmd, c)
		}
	}
	return
}
func (e *Editor) cards() (cards []*Card, cmd []*Card) {
	for _, r := range e.rows {
		cards = append(cards, r.card)
		if r.cmdr {
			cmd = append(cmd, r.card)
		}
	}
	return
}

// cachedAnalysis refaz a análise só quando a lista de cartas muda
func (g *Game) cachedAnalysis(cards []*Card, cmd []*Card) *Analysis {
	var b strings.Builder
	for _, c := range cards {
		b.WriteString(c.ID[:8])
	}
	b.WriteString("|")
	for _, c := range cmd {
		b.WriteString(c.ID[:8])
	}
	if k := b.String(); k != g.anKey || g.anRes == nil {
		g.anKey, g.anRes = k, analyze(cards, cmd)
	}
	return g.anRes
}
