package main

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Editor de deck: colar uma lista, conferir o que foi reconhecido, corrigir nomes,
// buscar cartas no banco local e salvar na pasta decks.

type edRow struct {
	qty  int
	card *Card
	cmdr bool
}
type edMiss struct {
	qty  int
	name string
	sugg []*Card
}
type Editor struct {
	orig    *Deck
	rows    []edRow
	miss    []edMiss
	scroll  float64
	lastQ   string
	results []*Card
	note    string
}

var catNames = []string{"Comandante", "Criaturas", "Planeswalkers", "Artefatos", "Encantamentos", "Mágicas instantâneas", "Feitiços", "Terrenos", "Outras"}

func rowCat(r edRow) int {
	if r.cmdr {
		return 0
	}
	t := r.card.FaceAt(0).T
	if r.card.T != "" {
		t = strings.Split(r.card.T, " // ")[0]
	}
	switch {
	case strings.Contains(t, "Creature"):
		return 1
	case strings.Contains(t, "Land"):
		return 7
	case strings.Contains(t, "Planeswalker"):
		return 2
	case strings.Contains(t, "Artifact"):
		return 3
	case strings.Contains(t, "Enchantment"):
		return 4
	case strings.Contains(t, "Instant"):
		return 5
	case strings.Contains(t, "Sorcery"):
		return 6
	}
	return 8
}

func (g *Game) openEditor(d *Deck, text string) {
	e := &Editor{orig: d}
	g.ed = e
	g.fields["edname"], g.fields["edq"] = "", ""
	if d != nil {
		g.fields["edname"] = d.Name
		if b, err := os.ReadFile(d.Path); err == nil {
			p := parseDeck(d.Name, string(b))
			p.Cmd = d.Cmd
			e.load(p, true)
		}
	}
	if text != "" {
		g.edAddText(text)
	}
	g.screen, g.focus, g.msg = "edit", "", ""
	if d == nil && text == "" {
		g.focus = "edname"
	}
}

func (e *Editor) load(p *Deck, useCmd bool) {
	cmd := map[string]bool{}
	for _, id := range p.Cmd {
		cmd[id] = true
	}
	for _, en := range p.Entries {
		e.add(en.Card, en.Qty, useCmd && cmd[en.Card.ID])
	}
	for k, n := range p.Missing {
		e.miss = append(e.miss, edMiss{qty: p.MissQty[k], name: n, sugg: suggest(n, 3)})
	}
}
func (e *Editor) add(c *Card, qty int, cmdr bool) {
	for k := range e.rows {
		if e.rows[k].card.ID == c.ID {
			e.rows[k].qty += qty
			e.rows[k].cmdr = e.rows[k].cmdr || cmdr
			return
		}
	}
	e.rows = append(e.rows, edRow{qty, c, cmdr})
}
func (e *Editor) total() int {
	n := 0
	for _, r := range e.rows {
		n += r.qty
	}
	return n
}
func (e *Editor) cmdCount() int {
	n := 0
	for _, r := range e.rows {
		if r.cmdr {
			n++
		}
	}
	return n
}

// edAddText junta uma lista colada/arrastada ao que já está no editor
func (g *Game) edAddText(text string) {
	e := g.ed
	p := parseDeck("", text)
	if len(p.Entries) == 0 && len(p.Missing) == 0 {
		e.note = "O texto não parece uma lista de cartas (formato: “1 Nome da carta” por linha)."
		return
	}
	first := len(e.rows) == 0
	before := e.total()
	e.load(p, first && e.cmdCount() == 0)
	e.note = fmt.Sprintf("Lista lida: %d cartas reconhecidas", e.total()-before)
	if n := len(p.Missing); n > 0 {
		e.note += fmt.Sprintf(", %d nome(s) não encontrados (veja em vermelho).", n)
	} else {
		e.note += "."
	}
	if strings.TrimSpace(g.fields["edname"]) == "" && len(e.rows) > 0 {
		nm := e.rows[0].card.N
		for _, r := range e.rows {
			if r.cmdr {
				nm = r.card.N
				break
			}
		}
		g.fields["edname"] = strings.Split(nm, " // ")[0]
	}
}

func (g *Game) edPaste() {
	s, ok := clipboardText()
	if !ok || strings.TrimSpace(s) == "" {
		g.ed.note = "Não achei texto copiado. Copie a lista (Ctrl+C) no site ou no bloco de notas e tente de novo."
		return
	}
	if g.focus != "" && !strings.ContainsAny(strings.TrimSpace(s), "\n\r") && !lineRe.MatchString(strings.TrimSpace(s)) {
		g.fields[g.focus] += strings.TrimSpace(s) // texto curto: cola no campo em foco
		return
	}
	g.edAddText(s)
}

func (e *Editor) text() string {
	var b strings.Builder
	rows := append([]edRow{}, e.rows...)
	sort.SliceStable(rows, func(a, c int) bool {
		if ca, cc := rowCat(rows[a]), rowCat(rows[c]); ca != cc {
			return ca < cc
		}
		return rows[a].card.N < rows[c].card.N
	})
	if e.cmdCount() > 0 {
		b.WriteString("Commander\n")
		for _, r := range rows {
			if r.cmdr {
				fmt.Fprintf(&b, "1 %s\n", r.card.N)
			}
		}
		b.WriteString("\nDeck\n")
	}
	for _, r := range rows {
		q := r.qty
		if r.cmdr {
			q--
		}
		if q > 0 {
			fmt.Fprintf(&b, "%d %s\n", q, r.card.N)
		}
	}
	for _, m := range e.miss {
		fmt.Fprintf(&b, "%d %s\n", m.qty, m.name)
	}
	return b.String()
}

func (g *Game) edSave() {
	e := g.ed
	if len(e.rows) == 0 {
		e.note = "O deck está vazio. Cole uma lista ou adicione cartas pela busca."
		return
	}
	name := strings.TrimSpace(fileSafe.ReplaceAllString(g.fields["edname"], ""))
	if name == "" {
		name = "Deck novo"
	}
	var err error
	if e.orig != nil && e.orig.Name == name {
		err = os.WriteFile(e.orig.Path, []byte(e.text()), 0o644)
	} else {
		name, err = saveDeckText(name, e.text())
		if err == nil && e.orig != nil {
			os.Remove(e.orig.Path) // renomeado: apaga só o arquivo antigo deste deck
			delete(cfg.Cmd, e.orig.Name)
		}
	}
	if err != nil {
		e.note = "Não consegui gravar na pasta decks: " + err.Error()
		return
	}
	delete(cfg.Cmd, name) // o comandante agora está escrito no próprio arquivo
	saveCfg()
	g.reloadDecks(name)
	g.msg = fmt.Sprintf("Deck “%s” salvo com %d cartas.", name, e.total())
	g.screen, g.ed, g.focus = "home", nil, ""
}

func (g *Game) drawEditor() {
	e := g.ed
	W, H := float64(g.W), float64(g.H)
	g.logo(28, 20, 26)
	title := "Novo deck"
	if e.orig != nil {
		title = "Editar deck"
	}
	txt(g.dst, title, 64, 18, fBold, 22, cAcc)
	g.field("edname", 220, 18, 320, "Nome do deck")
	// botões do topo
	x := W - 28
	rb := func(label string, style int, fn func()) {
		w := tw(label, fReg, 14) + 26
		x -= w
		g.button(x, 18, w, 32, label, style, fn)
		x -= 8
	}
	rb("Salvar deck", 1, g.edSave)
	rb("Cancelar", 0, func() { g.screen, g.ed, g.focus = "home", nil, "" })
	if e.orig != nil {
		rb("Excluir deck", 2, func() {
			d := e.orig
			g.confirm("Excluir o deck “"+d.Name+"”? O arquivo sai da pasta decks.", func() {
				os.Remove(d.Path)
				delete(cfg.Cmd, d.Name)
				saveCfg()
				g.screen, g.ed = "home", nil
				g.reloadDecks("")
				g.msg = "Deck “" + d.Name + "” excluído."
			})
		})
	}

	// coluna esquerda: colar e buscar
	lx, ly, lw := 28.0, 70.0, 400.0
	rrect(g.dst, lx, ly, lw, H-ly-20, 10, cPanel)
	y := ly + 14
	txt(g.dst, "1. Colar uma lista", lx+14, y, fBold, 15, cAcc)
	y += 28
	bw := g.button(lx+14, y, 0, 32, "Colar lista copiada (Ctrl+V)", 1, g.edPaste)
	g.button(lx+22+bw, y, 0, 32, "Limpar tudo", 0, func() { e.rows, e.miss, e.note = nil, nil, "" })
	y += 42
	hl := wrap("Aceita listas copiadas do Moxfield, Archidekt, Arena, MTGO ou do bloco de notas (“1 Nome da carta”, “1x Nome”, com ou sem edição). Também dá para arrastar um arquivo .txt para a janela. Colar de novo soma à lista.", fReg, 12.5, lw-28)
	txt(g.dst, strings.Join(hl, "\n"), lx+14, y, fReg, 12.5, cMut)
	y += float64(len(hl))*15.7 + 14
	txt(g.dst, "2. Adicionar carta pelo nome", lx+14, y, fBold, 15, cAcc)
	y += 28
	g.field("edq", lx+14, y, lw-28, "Digite parte do nome em inglês (ex.: sol ri)")
	y += 38
	if q := g.fields["edq"]; q != e.lastQ {
		e.lastQ = q
		e.results = searchCards(q, 9)
	}
	for _, c := range e.results {
		hov := g.hover(lx+14, y, lw-28, 26)
		if hov {
			rrect(g.dst, lx+14, y, lw-28, 26, 5, cPanel2)
		}
		txt(g.dst, fit(c.N, fReg, 13.5, lw-150), lx+22, y+5, fReg, 13.5, cTxt)
		t := fit(strings.Split(c.typeLine(), " — ")[0], fReg, 11.5, 110)
		txt(g.dst, t, lx+lw-22-tw(t, fReg, 11.5), y+7, fReg, 11.5, cMut)
		g.add(region{x: lx + 14, y: y, w: lw - 28, h: 26, pv: c, click: func() { e.add(c, 1, false); e.note = "Adicionada: " + c.N }})
		y += 27
	}
	if len(g.fields["edq"]) >= 2 && len(e.results) == 0 {
		txt(g.dst, "Nenhuma carta com esse nome no banco.", lx+22, y+4, fReg, 13, cMut)
	}
	// prévia da carta sob o mouse
	pvW := min(210.0, (H-20-(ly+470))/ratio)
	if g.pv != nil && pvW > 110 {
		drawTex(g.dst, g.cardTex(g.pv, 0), lx+(lw-pvW)/2, H-30-pvW*ratio, pvW, false, 1)
	}

	// coluna direita: o deck
	rx := lx + lw + 20
	rw := W - rx - 28
	rrect(g.dst, rx, ly, rw, H-ly-20, 10, cPanel)
	tot := e.total()
	st := fmt.Sprintf("%d cartas", tot)
	col := color.Color(cAcc2)
	if tot != 100 {
		st += " (Commander usa 100, contando o comandante)"
		col = cAcc
	}
	txt(g.dst, st, rx+16, ly+14, fBold, 15, col)
	if len(e.rows) > 0 {
		cards, cmd := e.cards()
		a := g.cachedAnalysis(cards, cmd)
		bcol := color.Color(cAcc2)
		if a.Bracket >= 4 {
			bcol = cBad
		} else if a.Bracket == 3 {
			bcol = cAcc
		}
		bwid := tw("Ver bracket e combos", fReg, 14) + 22
		g.button(rx+rw-16-bwid, ly+10, bwid, 30, "Ver bracket e combos", 0, func() {
			g.analysisModal(strings.TrimSpace(g.fields["edname"]), cards, cmd)
		})
		lab := fmt.Sprintf("%s · %d combos", a.title(), len(a.In))
		txt(g.dst, lab, rx+rw-26-bwid-tw(lab, fBold, 14), ly+16, fBold, 14, bcol)
	}
	sub := "Clique em “Cmd” para marcar o comandante."
	if n := e.cmdCount(); n > 2 {
		sub = "Há mais de 2 comandantes marcados."
	} else if n == 0 && len(e.rows) > 0 {
		sub = "Nenhum comandante marcado: clique em “Cmd” na carta certa."
	}
	if e.note != "" {
		sub = e.note
	}
	txt(g.dst, fit(sub, fReg, 13, rw-32), rx+16, ly+38, fReg, 13, cMut)

	type line struct {
		kind int // 0 título, 1 carta, 2 não encontrada
		k    int
		s    string
	}
	var lines []line
	if len(e.miss) > 0 {
		lines = append(lines, line{0, 0, fmt.Sprintf("Não encontradas (%d): clique numa sugestão para corrigir", len(e.miss))})
		for k := range e.miss {
			lines = append(lines, line{2, k, ""})
		}
	}
	idx := make([]int, len(e.rows))
	for k := range idx {
		idx[k] = k
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if ca, cb := rowCat(e.rows[idx[a]]), rowCat(e.rows[idx[b]]); ca != cb {
			return ca < cb
		}
		return e.rows[idx[a]].card.N < e.rows[idx[b]].card.N
	})
	last := -1
	for _, k := range idx {
		if c := rowCat(e.rows[k]); c != last {
			last = c
			n := 0
			for _, r := range e.rows {
				if rowCat(r) == c {
					n += r.qty
				}
			}
			lines = append(lines, line{0, 0, fmt.Sprintf("%s (%d)", catNames[c], n)})
		}
		lines = append(lines, line{1, k, ""})
	}
	if len(lines) == 0 {
		txt(g.dst, "O deck está vazio. Cole uma lista ou busque cartas ao lado.", rx+16, ly+80, fReg, 14, cMut)
	}
	const rh = 28.0
	listY, listH := ly+66, H-ly-20-76
	e.scroll = max(0, min(e.scroll, float64(len(lines))*rh-listH))
	g.add(region{x: rx, y: listY, w: rw, h: listH, wheel: func(dy float64) { e.scroll -= dy * rh * 3 }})
	old := g.dst
	subImg := clip(g.dst, rx, listY, rw, listH)
	for n, ln := range lines {
		yy := listY + float64(n)*rh - e.scroll
		if yy+rh < listY || yy > listY+listH {
			continue
		}
		vis := yy >= listY-2 && yy+rh <= listY+listH+2
		g.dst = subImg
		switch ln.kind {
		case 0:
			c := color.Color(cAcc)
			if strings.HasPrefix(ln.s, "Não encontradas") {
				c = cBad
			}
			txt(g.dst, ln.s, rx+16, yy+8, fBold, 12.5, c)
		case 1:
			r := &e.rows[ln.k]
			k := ln.k
			if g.hover(rx+10, yy, rw-20, rh) {
				rrect(g.dst, rx+10, yy+1, rw-20, rh-2, 5, cPanel2)
			}
			if !vis {
				txt(g.dst, r.card.N, rx+96, yy+6, fReg, 13.5, cTxt)
				break
			}
			g.dst = old
			g.add(region{x: rx + 10, y: yy, w: rw - 20, h: rh, pv: r.card})
			g.dst = subImg
			g.button(rx+16, yy+3, 22, 22, "-", 0, func() {
				e.rows[k].qty--
				if e.rows[k].qty <= 0 {
					e.rows = append(e.rows[:k], e.rows[k+1:]...)
				}
			})
			txtC(g.dst, fmt.Sprint(r.qty), rx+52, yy+5, fBold, 14, cTxt)
			g.button(rx+66, yy+3, 22, 22, "+", 0, func() { e.rows[k].qty++ })
			txt(g.dst, fit(r.card.N, fReg, 13.5, rw-330), rx+100, yy+6, fReg, 13.5, cTxt)
			cost := r.card.FaceAt(0).C
			txt(g.dst, cost, rx+rw-230+max(0, 150-tw(cost, fMono, 11.5)), yy+8, fMono, 11.5, cMut)
			stl := 0
			if r.cmdr {
				stl = 1
			}
			g.button(rx+rw-68, yy+3, 50, 22, "Cmd", stl, func() { e.rows[k].cmdr = !e.rows[k].cmdr })
		case 2:
			m := e.miss[ln.k]
			k := ln.k
			lab := fmt.Sprintf("%d  %s", m.qty, m.name)
			txt(g.dst, fit(lab, fReg, 13.5, 230), rx+16, yy+6, fReg, 13.5, cBad)
			if !vis {
				break
			}
			bx := rx + 260
			for _, c := range m.sugg {
				w := min(tw(c.N, fReg, 12)+16, 210)
				if bx+w > rx+rw-110 {
					break
				}
				g.dst = old
				g.add(region{x: bx, y: yy + 3, w: w, h: 22, pv: c})
				g.dst = subImg
				g.button(bx, yy+3, w, 22, c.N, 0, func() {
					e.add(c, m.qty, false)
					e.miss = append(e.miss[:k], e.miss[k+1:]...)
					e.note = "Corrigido: " + m.name + " → " + c.N
				})
				bx += w + 4
			}
			if len(m.sugg) == 0 {
				txt(g.dst, "sem sugestão: use a busca ao lado", bx, yy+7, fReg, 12, cMut)
			}
			g.button(rx+rw-98, yy+3, 80, 22, "Remover", 2, func() { e.miss = append(e.miss[:k], e.miss[k+1:]...) })
		}
		g.dst = old
	}
	if tot := float64(len(lines)) * rh; tot > listH {
		bh := listH * listH / tot
		rrect(g.dst, rx+rw-7, listY+(listH-bh)*e.scroll/(tot-listH), 4, bh, 2, cLine)
	}
}

func baseName(p string) string { return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)) }
