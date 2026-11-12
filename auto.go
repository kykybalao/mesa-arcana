package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Poder e resistência automáticos: o programa lê o texto (em inglês) da carta e reconhece
// "power is equal to the number of…", "gets +1/+1 for each…" e os bônus de equipamentos e auras anexados.
// Conta só o que dá para ver na mesa; quando não entende a frase, não mostra nada.

type ptBonus struct {
	p, t   int
	phrase string // "" = bônus fixo
}
type cardDyn struct {
	setP, setT string // frases que definem poder / resistência
	tFromP     bool   // resistência = poder + tPlus
	tPlus      int
	self       []ptBonus // "this creature gets +1/+1 for each…"
	host       []ptBonus // "equipped/enchanted creature gets…"
	bad        bool      // tem frase parecida que não foi entendida
}

var (
	reParen = regexp.MustCompile(`\([^)]*\)`)
	reEach  = regexp.MustCompile(`^(.*?) gets? ([+-]\d+)/([+-]\d+) for each (.+)$`)
	reFixed = regexp.MustCompile(`^(?:equipped|enchanted) creature gets ([+-]\d+)/([+-]\d+)(?: and .*)?$`)
	reBoth  = regexp.MustCompile(`power and toughness are each equal to (.+)$`)
	rePow   = regexp.MustCompile(`power is equal to (.+?)(?: and (?:its|his|her|their) toughness is equal to (.+))?$`)
	reTou   = regexp.MustCompile(`toughness is equal to (.+)$`)
	rePlusN = regexp.MustCompile(`^that number plus (\d+)$`)
	dynMemo = map[string]*cardDyn{}
)

func parseDyn(f Face) *cardDyn {
	d := &cardDyn{}
	name := strings.ToLower(f.N)
	short, _, _ := strings.Cut(name, ",")
	text := strings.ToLower(reParen.ReplaceAllString(f.X, ""))
	for _, line := range strings.Split(text, "\n") {
		for _, s := range strings.Split(line, ". ") {
			s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
			if s == "" || strings.Contains(s, "until end of turn") || strings.Contains(s, "as long as") || strings.Contains(s, ":") ||
				strings.HasPrefix(s, "when") || strings.HasPrefix(s, "if ") || strings.Contains(s, "this turn") {
				continue
			}
			if m := reEach.FindStringSubmatch(s); m != nil {
				p, _ := strconv.Atoi(m[2])
				t, _ := strconv.Atoi(m[3])
				ph, _, _ := strings.Cut(m[4], " and has ")
				ph, _, _ = strings.Cut(ph, " and gains ")
				b := ptBonus{p, t, ph}
				switch subj := m[1]; {
				case subj == "equipped creature" || subj == "enchanted creature":
					d.host = append(d.host, b)
				case subj == "this creature" || subj == "this vehicle" || subj == name || subj == short:
					d.self = append(d.self, b)
				}
				continue
			}
			if m := reFixed.FindStringSubmatch(s); m != nil {
				p, _ := strconv.Atoi(m[1])
				t, _ := strconv.Atoi(m[2])
				d.host = append(d.host, ptBonus{p, t, ""})
				continue
			}
			if strings.HasPrefix(s, "equipped creature") || strings.HasPrefix(s, "enchanted creature") || strings.Contains(s, "target ") {
				continue
			}
			if m := reBoth.FindStringSubmatch(s); m != nil {
				d.setP, d.setT = m[1], m[1]
			} else if m := rePow.FindStringSubmatch(s); m != nil {
				d.setP = m[1]
				if m[2] != "" {
					if k := rePlusN.FindStringSubmatch(m[2]); k != nil {
						d.tFromP = true
						d.tPlus, _ = strconv.Atoi(k[1])
					} else {
						d.setT = m[2]
					}
				}
			} else if m := reTou.FindStringSubmatch(s); m != nil {
				d.setT = m[1]
			}
		}
	}
	return d
}

func dynOf(c *Card, fi int) *cardDyn {
	k := c.ID + ":" + strconv.Itoa(fi)
	if d, ok := dynMemo[k]; ok {
		return d
	}
	if len(dynMemo) > 3000 {
		dynMemo = map[string]*cardDyn{}
	}
	d := parseDyn(c.FaceAt(fi))
	dynMemo[k] = d
	return d
}

// perm: o que se sabe de uma carta para contar
type perm struct {
	id     int
	t      string // linha de tipo em minúsculas
	tapped bool
	token  bool
	mine   bool
}

func (g *Game) permsOf(zone string) []perm {
	var out []perm
	for _, id := range g.S.Z[zone] {
		i := g.S.Cards[id]
		if i == nil || (i.Down && zone == "battlefield") {
			continue
		}
		out = append(out, perm{id, strings.ToLower(g.face(i).T), i.Tapped, i.Token, true})
	}
	return out
}
func (g *Game) peerPerms(gy bool) []perm {
	var out []perm
	for _, p := range nw.peerList() {
		if p.State == nil {
			continue
		}
		list := p.State.BF
		if gy {
			list = p.State.GY
		}
		for _, pi := range list {
			if c := g.pubCard(p, pi); c != nil && !pi.Down {
				out = append(out, perm{0, strings.ToLower(c.FaceAt(pi.Face).T), pi.Tapped, pi.Token, false})
			}
		}
	}
	return out
}

func hasWord(t, w string) bool {
	for _, x := range strings.FieldsFunc(t, func(r rune) bool { return r == ' ' || r == '—' || r == '-' || r == '/' }) {
		if x == w {
			return true
		}
	}
	return false
}

// singulars: formas no singular possíveis de uma palavra no plural
func singulars(w string) []string {
	out := []string{w}
	switch {
	case w == "elves":
		return []string{"elf"}
	case w == "wolves", w == "werewolves", w == "dwarves":
		return []string{strings.TrimSuffix(w, "ves") + "f"}
	case strings.HasSuffix(w, "ies"):
		out = append(out, strings.TrimSuffix(w, "ies")+"y", strings.TrimSuffix(w, "s"))
	case strings.HasSuffix(w, "es"):
		out = append(out, strings.TrimSuffix(w, "s"), strings.TrimSuffix(w, "es"))
	case strings.HasSuffix(w, "s"):
		out = append(out, strings.TrimSuffix(w, "s"))
	}
	return out
}

// matchTerm: a carta é um(a) "tapped artifact creature", "nontoken zombie" etc.?
func matchTerm(p perm, term string) (bool, bool) {
	for _, w := range strings.Fields(term) {
		switch w {
		case "tapped":
			if !p.tapped {
				return false, true
			}
		case "untapped":
			if p.tapped {
				return false, true
			}
		case "token", "tokens":
			if !p.token {
				return false, true
			}
		case "nontoken":
			if p.token {
				return false, true
			}
		case "permanent", "permanents", "card", "cards", "a", "an", "each":
		default:
			if strings.HasPrefix(w, "non") && len(w) > 4 {
				if hasWord(p.t, strings.TrimPrefix(w, "non")) {
					return false, true
				}
				continue
			}
			ok := false
			for _, s := range singulars(w) {
				ok = ok || hasWord(p.t, s)
			}
			if !ok {
				return false, true
			}
		}
	}
	return true, true
}

var reAlt = regexp.MustCompile(` and/or | or | and `)

func countPerms(list []perm, what string, selfID int, other bool) (int, bool) {
	if strings.Contains(what, " with ") || strings.Contains(what, " that ") || strings.Contains(what, " named ") || strings.Contains(what, "'") || what == "" {
		return 0, false
	}
	terms := reAlt.Split(what, -1)
	n := 0
	for _, p := range list {
		if other && p.mine && p.id == selfID {
			continue
		}
		for _, t := range terms {
			if ok, _ := matchTerm(p, strings.TrimSpace(t)); ok {
				n++
				break
			}
		}
	}
	return n, true
}

var (
	reNPlus    = regexp.MustCompile(`^(\d+) plus (.+)$`)
	basicTypes = []string{"plains", "island", "swamp", "mountain", "forest"}
	cardTypes  = []string{"artifact", "battle", "creature", "enchantment", "instant", "kindred", "land", "planeswalker", "sorcery"}
)

// count resolve uma frase como "artifacts you control" para a carta self
func (g *Game) count(phrase string, self *Inst) (int, bool) {
	ph := strings.TrimSpace(phrase)
	mult, add := 1, 0
	if m := reNPlus.FindStringSubmatch(ph); m != nil {
		add, _ = strconv.Atoi(m[1])
		ph = m[2]
	}
	if strings.HasPrefix(ph, "twice ") {
		mult, ph = 2, strings.TrimPrefix(ph, "twice ")
	}
	ph = strings.TrimPrefix(strings.TrimPrefix(ph, "the total number of "), "the number of ")
	res := func(n int, ok bool) (int, bool) { return n*mult + add, ok }
	distinct := func(list []perm, words []string) (int, bool) {
		n := 0
		for _, w := range words {
			for _, p := range list {
				if hasWord(p.t, w) {
					n++
					break
				}
			}
		}
		return res(n, true)
	}
	bf := func() []perm { return g.permsOf("battlefield") }
	switch ph {
	case "your life total":
		return res(g.S.Life, true)
	case "basic land types among lands you control", "basic land type among lands you control":
		return distinct(bf(), basicTypes)
	case "card types among cards in your graveyard", "card type among cards in your graveyard":
		return distinct(g.permsOf("graveyard"), cardTypes)
	case "card types among cards in all graveyards", "card type among cards in all graveyards":
		return distinct(append(g.permsOf("graveyard"), g.peerPerms(true)...), cardTypes)
	}
	other := false
	cut := func(suffix string) bool {
		if strings.HasSuffix(ph, suffix) {
			ph = strings.TrimSuffix(ph, suffix)
			if strings.HasPrefix(ph, "other ") {
				other, ph = true, strings.TrimPrefix(ph, "other ")
			}
			return true
		}
		return false
	}
	switch {
	case cut(" you control"):
		return res(countPerms(bf(), ph, self.ID, other))
	case cut(" on the battlefield"):
		return res(countPerms(append(bf(), g.peerPerms(false)...), ph, self.ID, other))
	case cut(" your opponents control"):
		return res(countPerms(g.peerPerms(false), ph, self.ID, other))
	case cut(" in your graveyard"):
		return res(countPerms(g.permsOf("graveyard"), ph, self.ID, other))
	case cut(" in all graveyards"):
		return res(countPerms(append(g.permsOf("graveyard"), g.peerPerms(true)...), ph, self.ID, other))
	case cut(" in your hand"):
		return res(countPerms(g.permsOf("hand"), ph, self.ID, other))
	case cut(" attached to it"), cut(" attached to this creature"):
		var list []perm
		for _, p := range bf() {
			if o := g.S.Cards[p.id]; o != nil && o.Att == self.ID {
				list = append(list, p)
			}
		}
		return res(countPerms(list, ph, self.ID, false))
	case cut(" counters on it"), cut(" counter on it"), cut(" counters on this creature"), cut(" counter on this creature"):
		for k, v := range self.Counters {
			if strings.EqualFold(k, ph) {
				return res(v, true)
			}
		}
		return res(0, true)
	}
	return 0, false
}

func baseNum(s string) (int, bool) {
	if strings.Contains(s, "*") { // valor variável: a frase do texto já traz o "mais 1"
		return 0, true
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil
}

// autoPT devolve o poder/resistência calculado de uma carta no meu campo e se vale a pena mostrar
// (a carta tem valor variável, ou contadores e anexos mudaram o valor impresso).
func (g *Game) autoPT(i *Inst) (string, bool) {
	if cfg.NoAuto || i == nil || i.Down || i.Zone != "battlefield" {
		return "", false
	}
	c := g.card(i)
	f := c.FaceAt(i.Face)
	if f.P == "" && f.O == "" {
		return "", false
	}
	p, ok1 := baseNum(f.P)
	t, ok2 := baseNum(f.O)
	if !ok1 || !ok2 {
		return "", false
	}
	d := dynOf(c, i.Face)
	show := false
	if d.setP != "" {
		n, ok := g.count(d.setP, i)
		if !ok {
			return "", false
		}
		p, show = p+n, true
		if d.tFromP {
			t = n + d.tPlus
		}
	}
	if d.setT != "" {
		n, ok := g.count(d.setT, i)
		if !ok {
			return "", false
		}
		t, show = t+n, true
	}
	apply := func(bs []ptBonus, src *Inst) bool {
		for _, b := range bs {
			n := 1
			if b.phrase != "" {
				var ok bool
				if n, ok = g.count(b.phrase, src); !ok {
					return false
				}
			}
			p, t, show = p+b.p*n, t+b.t*n, true
		}
		return true
	}
	if !apply(d.self, i) {
		return "", false
	}
	for _, id := range g.S.Z["battlefield"] {
		if a := g.S.Cards[id]; a != nil && a.Att == i.ID && !a.Down {
			if !apply(dynOf(g.card(a), a.Face).host, a) {
				return "", false
			}
		}
	}
	if n := i.Counters["+1/+1"] - i.Counters["-1/-1"]; n != 0 {
		p, t, show = p+n, t+n, true
	}
	return fmt.Sprintf("%d/%d", p, t), show
}
