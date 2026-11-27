package main

// busca.go: filtros da janela de buscar no grimório (e de ver cemitério/exílio): nome, tipo, custo e cor.

import (
	"regexp"
	"strconv"
	"strings"
)

type zoneFilter struct {
	typ   string          // tipo em inglês ("" = todos)
	cmc   int             // -1 = todos; 7 = 7 ou mais
	cols  map[string]bool // W U B R G e "C" (incolor)
	byCMC bool            // ordenar por custo em vez de nome
}

var filterTypes = [][2]string{{"", "Todos"}, {"Creature", "Criatura"}, {"Land", "Terreno"}, {"Artifact", "Artefato"}, {"Enchantment", "Encantamento"}, {"Instant", "Instantânea"}, {"Sorcery", "Feitiço"}, {"Planeswalker", "Planeswalker"}}
var filterCols = [][2]string{{"W", "Branco"}, {"U", "Azul"}, {"B", "Preto"}, {"R", "Vermelho"}, {"G", "Verde"}, {"C", "Incolor"}}

var costSymRe = regexp.MustCompile(`\{([^}]+)\}`)

// manaValue: valor de mana convertido de um custo como {2}{W/B}{X}
func manaValue(cost string) int {
	n := 0
	for _, m := range costSymRe.FindAllStringSubmatch(cost, -1) {
		s := m[1]
		if v, err := strconv.Atoi(s); err == nil {
			n += v
		} else if s == "X" || s == "Y" || s == "Z" {
		} else if strings.HasPrefix(s, "2/") {
			n += 2
		} else if s == "HW" || s == "HR" {
		} else {
			n++
		}
	}
	return n
}

// costColors: cores que aparecem no custo
func costColors(cost string) map[string]bool {
	out := map[string]bool{}
	for _, m := range costSymRe.FindAllStringSubmatch(cost, -1) {
		for _, c := range "WUBRG" {
			if strings.ContainsRune(m[1], c) {
				out[string(c)] = true
			}
		}
	}
	return out
}

func (f *zoneFilter) on() bool { return f.typ != "" || f.cmc >= 0 || len(f.cols) > 0 }

func (f *zoneFilter) match(c *Card, name string) bool {
	if name != "" {
		l, _ := loc(c, 0)
		if !strings.Contains(squash(strings.ToLower(c.N+" "+l.N)), squash(strings.ToLower(name))) {
			return false
		}
	}
	face := c.FaceAt(0)
	if f.typ != "" {
		ok := false
		for _, fc := range c.Faces() {
			ok = ok || strings.Contains(strings.Split(fc.T, "—")[0], f.typ)
		}
		if !ok {
			return false
		}
	}
	if f.cmc >= 0 {
		v := manaValue(face.C)
		if !(v == f.cmc || (f.cmc == 7 && v >= 7)) {
			return false
		}
	}
	if len(f.cols) > 0 {
		cc := costColors(face.C)
		ok := f.cols["C"] && len(cc) == 0
		for k := range cc {
			ok = ok || f.cols[k]
		}
		if !ok {
			return false
		}
	}
	return true
}

// bar desenha os filtros e devolve a altura usada
func (f *zoneFilter) bar(g *Game, m *Modal, x, y, w float64) float64 {
	chip := func(px *float64, py float64, lab string, on bool, fn func()) {
		cw := tw(lab, fReg, 12.5) + 18
		if on {
			rrect(g.dst, *px, py, cw, 24, 8, cAcc)
			txtC(g.dst, lab, *px+cw/2, py+4.5, fBold, 12.5, cDark)
		} else {
			rrect(g.dst, *px, py, cw, 24, 8, cPanel2)
			txtC(g.dst, lab, *px+cw/2, py+4.5, fReg, 12.5, cTxt)
		}
		g.add(region{x: *px, y: py, w: cw, h: 24, click: func() { fn(); m.scroll = 0 }})
		*px += cw + 5
	}
	g.field("vzq", x, y, 210, "Nome da carta")
	px := x + 222
	txt(g.dst, "Tipo", px, y+7, fBold, 12, cMut)
	px += 36
	for _, t := range filterTypes {
		chip(&px, y+3, t[1], f.typ == t[0], func() { f.typ = t[0] })
	}
	y2 := y + 36
	px = x
	txt(g.dst, "Custo", px, y2+5, fBold, 12, cMut)
	px += 44
	chip(&px, y2, "Todos", f.cmc < 0, func() { f.cmc = -1 })
	for v := 0; v <= 7; v++ {
		lab := strconv.Itoa(v)
		if v == 7 {
			lab = "7+"
		}
		chip(&px, y2, lab, f.cmc == v, func() { f.cmc = v })
	}
	px += 14
	txt(g.dst, "Cor", px, y2+5, fBold, 12, cMut)
	px += 32
	for _, c := range filterCols {
		chip(&px, y2, c[1], f.cols[c[0]], func() {
			if f.cols[c[0]] {
				delete(f.cols, c[0])
			} else {
				f.cols[c[0]] = true
			}
		})
	}
	px += 14
	lab := "Ordem: nome"
	if f.byCMC {
		lab = "Ordem: custo"
	}
	chip(&px, y2, lab, false, func() { f.byCMC = !f.byCMC })
	if f.on() || g.fields["vzq"] != "" {
		chip(&px, y2, "Limpar filtros", false, func() {
			f.typ, f.cmc, f.cols = "", -1, map[string]bool{}
			g.fields["vzq"] = ""
		})
	}
	return 70
}
