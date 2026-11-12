package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) Draw(screen *ebiten.Image) {
	g.dst = screen
	g.drawn = true
	pW, pH := screen.Bounds().Dx(), screen.Bounds().Dy()
	g.W, g.H = int(float64(pW)/uiS), int(float64(pH)/uiS)
	g.regs = g.regs[:0]
	screen.Fill(cBg)
	if g.screen == "game" && g.S != nil {
		g.drawGame()
	} else if g.screen == "edit" && g.ed != nil {
		g.drawEditor()
	} else {
		g.drawHome()
	}
	if g.modal != nil {
		g.drawModal()
	}
	if g.ctx != nil {
		g.drawCtx()
	}
	if g.drag.on && g.drag.moved && g.S != nil {
		if i := g.S.Cards[g.drag.id]; i != nil {
			g.drawInst(i, float64(g.mx)-g.drag.ox, float64(g.my)-g.drag.oy, g.cw, i.Zone == "library", false, 0.9)
		}
	}
	if g.shot != "" {
		im := image.NewRGBA(image.Rect(0, 0, pW, pH))
		screen.ReadPixels(im.Pix)
		if f, err := os.Create(g.shot); err == nil {
			png.Encode(f, im)
			f.Close()
		}
		g.shot = ""
	}
}

func (g *Game) logo(x, y, s float64) {
	rrect(g.dst, x, y, s, s, s*0.2, cAcc)
	rrect(g.dst, x+2, y+2, s-4, s-4, s*0.15, cPanel)
	rrect(g.dst, x+s*0.3, y+s*0.3, s*0.4, s*0.4, s*0.2, cAcc2)
}

/* ====================== tela inicial ====================== */

func (g *Game) drawHome() {
	W, H := float64(g.W), float64(g.H)
	g.logo(28, 24, 30)
	txt(g.dst, "Mesa Arcana", 68, 20, fBold, 28, cAcc)
	txt(g.dst, "Simulador offline de Commander. Tudo roda do pendrive, sem internet.", 30, 62, fReg, 14, cMut)

	{
		mode, _ := nw.info()
		lab, st := "Multijogador (rede local)", 0
		if mode != "" {
			lab, st = fmt.Sprintf("Mesa em rede: %d jogador(es) além de você", len(nw.peerList())), 1
		}
		bwid := tw(lab, fReg, 14) + 26
		g.button(W-28-bwid, 28, bwid, 34, lab, st, g.netModal)
		x2 := W - 28 - bwid - 8
		for _, b := range []struct {
			lab string
			fn  func()
		}{{"Aparência", g.looksModal}, {langLabel(), g.toggleLang}, {"Criar carta", g.myModal}} {
			w2 := tw(b.lab, fReg, 14) + 26
			x2 -= w2
			g.button(x2, 28, w2, 34, b.lab, 0, b.fn)
			x2 -= 8
		}
	}
	// coluna da esquerda: decks
	lx, ly, lw := 28.0, 100.0, 400.0
	lh := H - ly - 176
	rrect(g.dst, lx, ly, lw, lh, 10, cPanel)
	txt(g.dst, "Meus decks", lx+14, ly+12, fBold, 16, cAcc)
	listY, listH := ly+44, lh-54
	rows := float64(len(g.decks)) * 46
	g.listScr = max(0, min(g.listScr, rows-listH))
	g.add(region{x: lx, y: listY, w: lw, h: listH, wheel: func(dy float64) { g.listScr -= dy * 46 }})
	if len(g.decks) == 0 {
		txt(g.dst, strings.Join(wrap("Nenhum deck ainda. Salve uma lista como arquivo .txt na pasta decks, ao lado do programa, ou use os botões abaixo.", fReg, 14, lw-28), "\n"), lx+14, listY, fReg, 14, cMut)
	}
	sub := clip(g.dst, lx, listY, lw, listH)
	old := g.dst
	for k, d := range g.decks {
		y := listY + float64(k)*46 - g.listScr
		if y+42 < listY || y > listY+listH {
			continue
		}
		g.dst = sub
		bg := cPanel2
		if k == g.sel {
			rrect(g.dst, lx+10, y, lw-20, 40, 7, cAcc)
		} else {
			rrect(g.dst, lx+10, y, lw-20, 40, 7, cLine)
		}
		rrect(g.dst, lx+11, y+1, lw-22, 38, 6, bg)
		txt(g.dst, fit(d.Name, fBold, 14.5, lw-140), lx+22, y+10, fBold, 14.5, cTxt)
		info := fmt.Sprintf("%d cartas", d.Total)
		txt(g.dst, info, lx+lw-24-tw(info, fReg, 12.5), y+12, fReg, 12.5, cMut)
		g.dst = old
		if y >= listY-20 && y+40 <= listY+listH+20 {
			g.add(region{x: lx + 10, y: max(y, listY), w: lw - 20, h: 40, click: func() { g.sel = k; g.imgTick = 0; g.msg = "" }})
		}
	}
	by := ly + lh + 12
	x := lx
	x += g.button(x, by, 0, 32, "Novo deck / importar lista", 1, g.pasteDeck) + 8
	x += g.button(x, by, 0, 32, "Recarregar pasta", 0, func() { g.reloadDecks(""); g.msg = "Pasta decks recarregada." }) + 8
	_ = x
	help := "“Novo deck” abre o editor: cole a lista copiada (Moxfield, Archidekt, Arena, bloco de notas), confira o que foi reconhecido, corrija nomes e busque cartas no banco local. Também dá para arrastar um .txt para esta janela."
	txt(g.dst, strings.Join(wrap(help, fReg, 12.5, lw), "\n"), lx, by+44, fReg, 12.5, cMut)

	// coluna da direita: deck escolhido
	rx := lx + lw + 24
	rw := W - rx - 28
	rrect(g.dst, rx, ly, rw, lh, 10, cPanel)
	foot := fmt.Sprintf("Banco de cartas embutido: %d cartas (dados do Scryfall de %s). Projeto pessoal, não oficial.", dbCount, dbDate)
	txt(g.dst, fit(foot, fReg, 12, W-56), lx, H-28, fReg, 12, cMut)
	if g.msg != "" {
		txt(g.dst, strings.Join(wrap(g.msg, fReg, 13.5, rw), "\n"), rx, by+4, fReg, 13.5, cAcc2)
	}
	if g.sel >= len(g.decks) {
		txt(g.dst, "Escolha ou adicione um deck.", rx+18, ly+16, fReg, 15, cMut)
		return
	}
	d := g.decks[g.sel]
	cardW := min(260.0, (lh-40)/ratio)
	tw0 := rw - cardW - 54
	y := ly + 14
	txt(g.dst, fit(d.Name, fBold, 22, tw0), rx+18, y, fBold, 22, cTxt)
	y += 34
	txt(g.dst, fmt.Sprintf("%d cartas · %d fichas diferentes", d.Total, len(d.Tokens)), rx+18, y, fReg, 14, cMut)
	y += 26
	if d.Total != 100 {
		txt(g.dst, "Atenção: um deck de Commander tem 100 cartas contando o comandante.", rx+18, y, fReg, 13, cAcc)
		y += 22
	}
	if len(d.Missing) > 0 {
		ls := wrap("Não achei no banco (confira o nome em inglês): "+strings.Join(d.Missing, "; "), fReg, 13, tw0)
		if len(ls) > 4 {
			ls = append(ls[:4], "…")
		}
		txt(g.dst, strings.Join(ls, "\n"), rx+18, y, fReg, 13, cBad)
		y += float64(len(ls))*16.5 + 8
	}
	y += 8
	legs := d.Legends()
	cyc := func(slot int) func() {
		return func() {
			cur := ""
			if slot < len(d.Cmd) {
				cur = d.Cmd[slot]
			}
			opts := []string{""}
			for _, c := range legs {
				if !(slot == 1 && len(d.Cmd) > 0 && c.ID == d.Cmd[0]) && !(slot == 0 && len(d.Cmd) > 1 && c.ID == d.Cmd[1]) {
					opts = append(opts, c.ID)
				}
			}
			n := 0
			for k, o := range opts {
				if o == cur {
					n = (k + 1) % len(opts)
				}
			}
			var cmd []string
			for k := 0; k < 2; k++ {
				v := ""
				if k < len(d.Cmd) {
					v = d.Cmd[k]
				}
				if k == slot {
					v = opts[n]
				}
				if v != "" {
					cmd = append(cmd, v)
				}
			}
			d.Cmd = cmd
			cfg.Cmd[d.Name] = cmd
			saveCfg()
		}
	}
	nameOf := func(slot int, none string) string {
		if slot < len(d.Cmd) {
			return cardByID[d.Cmd[slot]].N
		}
		return none
	}
	txt(g.dst, "Comandante", rx+18, y+7, fReg, 13.5, cMut)
	g.button(rx+110, y, min(tw0-92, 330), 32, nameOf(0, "(sem comandante)"), 0, cyc(0))
	y += 40
	if len(d.Cmd) > 0 {
		txt(g.dst, "Parceiro", rx+18, y+7, fReg, 13.5, cMut)
		g.button(rx+110, y, min(tw0-92, 330), 32, nameOf(1, "(sem parceiro)"), 0, cyc(1))
		y += 40
	}
	txt(g.dst, "Clique no nome para trocar entre as lendárias do deck.", rx+18, y, fReg, 12, cMut)
	y += 34

	// imagens
	if g.imgTick <= 0 {
		keys := d.imageKeys()
		g.imgTot, g.imgHave = len(keys), 0
		for _, k := range keys {
			if hasImg(k) {
				g.imgHave++
			}
		}
		g.imgTick = 90
	}
	g.imgTick--
	st := fmt.Sprintf("Imagens no pendrive: %d de %d.", g.imgHave, g.imgTot)
	if g.imgHave < g.imgTot {
		st += " As cartas sem imagem aparecem como proxy de texto, e o jogo funciona igual."
	}
	ls := wrap(st, fReg, 13.5, tw0)
	txt(g.dst, strings.Join(ls, "\n"), rx+18, y, fReg, 13.5, cTxt)
	y += float64(len(ls))*17 + 10
	if dl.running.Load() {
		g.button(rx+18, y, 0, 32, fmt.Sprintf("Baixando… %d/%d (clique para parar)", dl.done.Load(), dl.total.Load()), 0, func() { dl.stop.Store(true) })
	} else if g.imgHave < g.imgTot {
		g.button(rx+18, y, 0, 32, "Baixar imagens deste deck (em casa, com internet)", 0, func() { dl.start(d.imageKeys()) })
	}
	y += 40
	if m, _ := dl.msg.Load().(string); m != "" {
		ls := wrap(m, fReg, 13, tw0)
		txt(g.dst, strings.Join(ls, "\n"), rx+18, y, fReg, 13, cAcc)
		y += float64(len(ls))*16.5 + 8
	}
	if len(d.Entries) > 0 {
		g.button(rx+18, ly+lh-60, 180, 44, "Jogar", 1, func() { g.startGame(d) })
		g.button(rx+208, ly+lh-60, 130, 44, "Editar deck", 0, func() { g.openEditor(d, "") })
		cards, cmd := deckCards(d)
		a := g.cachedAnalysis(cards, cmd)
		g.button(rx+348, ly+lh-60, 190, 44, "Bracket e combos", 0, func() { g.analysisModal(d.Name, cards, cmd) })
		bcol := color.Color(cAcc2)
		if a.Bracket >= 4 {
			bcol = cBad
		} else if a.Bracket == 3 {
			bcol = cAcc
		}
		txt(g.dst, fmt.Sprintf("%s · %d Game Changers · %d combos no deck", a.title(), len(a.GC), len(a.In)), rx+18, ly+lh-88, fBold, 14, bcol)
	}
	// carta do comandante (ou a primeira do deck) como ilustração
	var show *Card
	if len(d.Cmd) > 0 {
		show = cardByID[d.Cmd[0]]
	} else if len(d.Entries) > 0 {
		show = d.Entries[0].Card
	}
	if show != nil {
		drawTex(g.dst, g.cardTex(show, 0), rx+rw-cardW-18, ly+18, cardW, false, 1)
	}
}

// pasteDeck abre o editor; se já houver uma lista copiada, ela entra direto para conferência
func (g *Game) pasteDeck() {
	text := ""
	if s, ok := clipboardText(); ok {
		if p := parseDeck("", s); len(p.Entries) >= 2 {
			text = s
		}
	}
	g.openEditor(nil, text)
}

/* ====================== mesa ====================== */

func (g *Game) drawGame() {
	s := g.S
	W, H := float64(g.W), float64(g.H)
	const top, leftW = 46.0, 250.0
	cw := g.cw
	ch := cw * ratio
	pileW := min(cw, ((H-top-10)/4-30)/ratio)
	rightW := pileW + 28
	handH := ch + 18
	strip := 30.0 // faixa das fases do turno
	var peer *Peer
	netOn := nw.active()
	if netOn {
		strip = 58 // mais a faixa dos jogadores da mesa
		if g.viewPeer != 0 {
			peer = nw.peer(g.viewPeer)
		}
	}
	bx, by, bw, bh := leftW, top+strip, W-leftW-rightW, H-top-strip-handH
	g.bf = image.Rect(int(bx), int(by), int(bx+bw), int(by+bh))

	if peer == nil {
		// campo
		rect(g.dst, bx, by, bw, bh, cFelt)
		if mt := g.matTex(); mt != nil {
			drawCover(g.dst, mt, bx, by, bw, bh, 0.82)
		}
		rect(g.dst, bx, by, bw, 6, color.RGBA{0, 0, 0, 0x30})
		g.add(region{x: bx, y: by, w: bw, h: bh, zone: "battlefield", rclick: func() {
			if s.Phase == "play" {
				g.openCtx(g.tableMenu())
			}
		}})
		if len(s.Z["battlefield"]) == 0 {
			txtC(g.dst, "CAMPO DE BATALHA", bx+bw/2, by+bh/2-12, fBold, 22, color.RGBA{0xff, 0xff, 0xff, 0x22})
		}
		bfImg := clip(g.dst, bx, by, bw, bh)
		whole := g.dst
		g.dst = bfImg
		for _, id := range s.Z["battlefield"] {
			i := s.Cards[id]
			x, y := bx+i.X, by+i.Y
			a := float32(1)
			if g.drag.on && g.drag.moved && g.drag.id == id {
				a = 0.25
			}
			g.drawInst(i, x, y, cw, false, false, a)
			r := region{x: x, y: y, w: cw, h: ch, inst: id, pv: g.card(i), pvFace: i.Face, rclick: func() { g.cardMenu(id) }}
			if i.Tapped {
				r.x, r.y, r.w, r.h = x+cw/2-ch/2, y+ch/2-cw/2, ch, cw
			}
			g.add(r)
		}
		g.dst = whole

		// faixa das fases: clique numa fase para anunciá-la
		rect(g.dst, bx, top, bw, 30, cPanel)
		rect(g.dst, bx, top+29, bw, 1, cLine)
		if s.Phase == "play" {
			tot := 0.0
			for _, n := range stepNames {
				tot += tw(n, fReg, 12.5) + 22 + 6
			}
			px := bx + (bw-tot)/2
			for k, n := range stepNames {
				w := tw(n, fReg, 12.5) + 22
				if k == s.Step {
					rrect(g.dst, px, top+4, w, 22, 11, cAcc)
					txtC(g.dst, n, px+w/2, top+7, fBold, 12.5, color.RGBA{0x1b, 0x16, 0x08, 0xff})
				} else {
					rrect(g.dst, px, top+4, w, 22, 11, cPanel2)
					txtC(g.dst, n, px+w/2, top+7, fReg, 12.5, cMut)
				}
				g.add(region{x: px, y: top + 4, w: w, h: 22, click: func() { g.setStep(k) }})
				px += w + 6
			}
		}
		if g.bannerT > 0 && s.Phase == "play" { // anúncio grande da fase, some sozinho
			a := min(1, float64(g.bannerT)/25)
			w := tw(g.banner, fBold, 30) + 60
			rrect(g.dst, bx+(bw-w)/2, by+bh*0.38, w, 58, 12, color.RGBA{0, 0, 0, uint8(190 * a)})
			txtC(g.dst, g.banner, bx+bw/2, by+bh*0.38+10, fBold, 30, color.RGBA{uint8(0xe0 * a), uint8(0xb3 * a), uint8(0x4a * a), uint8(255 * a)})
		}
		if g.equip != 0 {
			nm := "carta"
			if i := s.Cards[g.equip]; i != nil {
				nm = g.card(i).N
			} else {
				g.equip = 0
			}
			msg := "Clique na carta que vai receber " + nm
			w := tw(msg, fReg, 14) + 140
			ex := bx + (bw-w)/2
			rrect(g.dst, ex-1, by+bh-55, w+2, 44, 10, cAcc2)
			rrect(g.dst, ex, by+bh-54, w, 42, 9, color.RGBA{0x10, 0x14, 0x17, 0xf2})
			txt(g.dst, msg, ex+14, by+bh-42, fReg, 14, cTxt)
			g.button(ex+w-112, by+bh-48, 100, 30, "Cancelar", 0, func() { g.equip = 0 })
		}

		// mulligan
		if s.Phase != "play" {
			n := g.bottomCount()
			msg := "Mão inicial"
			if s.Phase == "mull" && s.Mulls > 0 {
				msg = fmt.Sprintf("Mulligan nº %d (grátis)", s.Mulls)
				if n > 0 {
					msg = fmt.Sprintf("Mulligan nº %d (vai devolver %d ao fundo)", s.Mulls, n)
				}
			} else if s.Phase == "bottom" {
				msg = fmt.Sprintf("Clique em %d carta(s) da mão para pôr no fundo do grimório (%d/%d)", n, len(s.Pick), n)
			}
			mw := tw(msg, fReg, 14) + 250
			mx := bx + (bw-mw)/2
			rrect(g.dst, mx-1, by+13, mw+2, 50, 10, cAcc)
			rrect(g.dst, mx, by+14, mw, 48, 9, color.RGBA{0x10, 0x14, 0x17, 0xf2})
			g.add(region{x: mx, y: by + 14, w: mw, h: 48, block: true, click: func() {}})
			txt(g.dst, msg, mx+14, by+29, fReg, 14, cTxt)
			if s.Phase == "mull" {
				g.button(mx+mw-222, by+23, 100, 30, "Manter", 1, g.keep)
				g.button(mx+mw-114, by+23, 100, 30, "Mulligan", 0, g.mulligan)
			} else {
				st := 1
				if len(s.Pick) != n {
					st = 3
				}
				g.button(mx+mw-124, by+23, 110, 30, "Confirmar", st, g.confirmBottom)
			}
		}

		// mão
		hy := H - handH
		rect(g.dst, bx, hy, bw, handH, cPanel)
		rect(g.dst, bx, hy, bw, 1, cLine)
		g.add(region{x: bx, y: hy, w: bw, h: handH, zone: "hand"})
		hand := s.Z["hand"]
		g.handPos = g.handPos[:0]
		if n := len(hand); n > 0 {
			step := cw + 6
			if n > 1 {
				step = min(step, (bw-20-cw)/float64(n-1))
			}
			for k, id := range hand {
				i := s.Cards[id]
				x, y := bx+10+float64(k)*step, hy+10
				g.handPos = append(g.handPos, x)
				a := float32(1)
				if g.drag.on && g.drag.moved && g.drag.id == id {
					a = 0.25
				}
				if contains(s.Pick, id) {
					y -= 8
					rrect(g.dst, x-3, y-3, cw+6, ch+6, cw*0.06, cBad)
				}
				g.drawInst(i, x, y, cw, false, true, a)
				g.add(region{x: x, y: y, w: cw, h: ch, inst: id, pv: g.card(i), pvFace: i.Face, rclick: func() { g.cardMenu(id) }})
			}
		}
		if netOn && s.Phase == "play" {
			lab, st := "Revelar mão", 0
			if g.showHand {
				lab, st = "Esconder mão (a mesa está vendo)", 2
			}
			w2 := tw(lab, fReg, 12) + 22
			g.button(bx+bw-w2-8, hy+4, w2, 22, lab, st, g.toggleReveal)
		}

	} else {
		g.drawPeerBoard(peer, bx, by, bw, bh, top, handH, H)
	}
	if netOn {
		g.drawPlayers(bx, top+30, bw)
	}

	// painel esquerdo: prévia, texto e registro
	rect(g.dst, 0, top, leftW, H-top, cPanel)
	rect(g.dst, leftW-1, top, 1, H-top, cLine)
	pvW := leftW - 16
	pvH := pvW * ratio
	logH := 150.0
	if netOn {
		logH = 210 // sobra espaço para o chat
	}
	if g.pv != nil && !g.pvHide {
		drawTex(g.dst, g.cardTex(g.pv, g.pvFace), 8, top+8, pvW, false, 1)
		f, _ := loc(g.pv, g.pvFace)
		body := f.N
		if f.C != "" {
			body += "  " + f.C
		}
		body += "\n" + f.T + "\n\n" + f.X
		if f.P != "" || f.O != "" {
			body += "\n\n" + f.P + "/" + f.O
		}
		if f.L != "" {
			body += "\n\nLealdade: " + f.L
		}
		if nt := locNote(g.pv, g.pvFace); nt != "" {
			body += "\n\n" + nt
		}
		ls := wrap(body, fReg, 12.5, pvW)
		maxL := int((H - logH - (top + pvH + 20)) / 16)
		if len(ls) > maxL && maxL > 0 {
			ls = append(ls[:maxL-1], "…")
		}
		if maxL > 0 {
			txt(g.dst, strings.Join(ls, "\n"), 8, top+pvH+18, fReg, 12.5, color.RGBA{0xcf, 0xd8, 0xdc, 0xff})
		}
	} else {
		rstroke(g.dst, 8, top+8, pvW, pvH, 11, 1, cLine)
		msg := "Passe o mouse sobre uma carta"
		if g.pvHide {
			msg = "Carta oculta"
		}
		txtC(g.dst, msg, leftW/2, top+pvH/2, fReg, 12.5, cMut)
	}
	ly := H - logH
	rect(g.dst, 8, ly, pvW, 1, cLine)
	var lines []string
	for k := len(s.Log) - 1; k >= 0 && len(lines) < 12; k-- {
		w := wrap(s.Log[k], fReg, 11.5, pvW)
		lines = append(append([]string{}, w...), lines...)
	}
	if netOn {
		g.chatBox(8, H-38, pvW)
		logH -= 40
	}
	maxL := int((logH - 12) / 14.4)
	if len(lines) > maxL {
		lines = lines[len(lines)-maxL:]
	}
	txt(g.dst, strings.Join(lines, "\n"), 8, ly+6, fReg, 11.5, cMut)

	if peer == nil {
		// painel direito: pilhas
		rx := W - rightW
		rect(g.dst, rx, top, rightW, H-top, cPanel)
		rect(g.dst, rx, top, 1, H-top, cLine)
		slotH := (H - top - 10) / 4
		for k, z := range []string{"command", "library", "graveyard", "exile"} {
			y := top + 6 + float64(k)*slotH
			ids := s.Z[z]
			label := map[string]string{"command": "COMANDO", "library": "GRIMÓRIO", "graveyard": "CEMITÉRIO", "exile": "EXÍLIO"}[z]
			txt(g.dst, label, rx+10, y+3, fBold, 10.5, cMut)
			if z != "command" {
				c := fmt.Sprint(len(ids))
				txt(g.dst, c, rx+rightW-10-tw(c, fBold, 11.5), y+2, fBold, 11.5, cTxt)
			}
			cy := y + 20
			rstroke(g.dst, rx+14, cy, pileW, pileW*ratio, pileW*0.05, 1, cLine)
			reg := region{x: rx + 4, y: y, w: rightW - 8, h: slotH - 4, zone: z}
			if z == "library" {
				reg.rclick = func() {
					if s.Phase == "play" {
						g.openCtx(g.libMenu())
					}
				}
			} else if z != "command" {
				reg.rclick = func() {
					if s.Phase == "play" {
						g.viewZone(z, false)
					}
				}
			}
			g.add(reg)
			show := ids
			if z == "library" && len(ids) > 0 {
				show = ids[:1]
			} else if z != "command" && len(ids) > 0 {
				show = ids[len(ids)-1:]
			}
			for n, id := range show {
				i := s.Cards[id]
				yy := cy + float64(n)*24
				a := float32(1)
				if g.drag.on && g.drag.moved && g.drag.id == id {
					a = 0.25
				}
				g.drawInst(i, rx+14, yy, pileW, z == "library", true, a)
				g.add(region{x: rx + 14, y: yy, w: pileW, h: pileW * ratio, inst: id, pv: g.card(i), pvFace: i.Face, pvHidden: z == "library", rclick: func() { g.cardMenu(id) }})
			}
		}

	} else {
		g.drawPeerPiles(peer, W-rightW, top, rightW, pileW, H)
	}

	// barra de cima
	rect(g.dst, 0, 0, W, top, cPanel)
	rect(g.dst, 0, top-1, W, 1, cLine)
	g.logo(10, 11, 24)
	x := 46.0
	shift := func() int {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			return 5
		}
		return 1
	}
	stat := func(label string, val int, col color.Color, dec, inc func()) {
		if label != "" {
			txt(g.dst, label, x, 15, fReg, 12.5, cMut)
			x += tw(label, fReg, 12.5) + 6
		}
		x += g.button(x, 11, 22, 24, "-", 0, dec) + 2
		v := fmt.Sprint(val)
		txtC(g.dst, v, x+17, 10, fBold, 19, col)
		x += 36
		x += g.button(x, 11, 22, 24, "+", 0, inc) + 10
	}
	stat("Vida", s.Life, cAcc2, func() { g.snap(); g.S.Life -= shift() }, func() { g.snap(); g.S.Life += shift() })
	for k := range s.Opp {
		lab := ""
		if k == 0 {
			lab = "Oponentes"
		}
		col := color.Color(cTxt)
		if s.Opp[k] <= 0 {
			col = cBad
		}
		stat(lab, s.Opp[k], col, func() { g.snap(); g.S.Opp[k] -= shift() }, func() { g.snap(); g.S.Opp[k] += shift() })
	}
	txt(g.dst, "Turno", x, 15, fReg, 12.5, cMut)
	x += 40
	txtC(g.dst, fmt.Sprint(s.Turn), x+12, 10, fBold, 19, cTxt)
	x += 32
	on := func(fn func()) func() {
		return func() {
			if g.playing() {
				fn()
			}
		}
	}
	x += g.button(x, 8, 0, 30, "Próxima fase", 1, on(g.nextStep)) + 6
	x += g.button(x, 8, 0, 30, "Próximo turno", 0, on(g.nextTurn)) + 6
	x += g.button(x, 8, 0, 30, "Desvirar", 0, on(g.untapAll)) + 6
	x += g.button(x, 8, 0, 30, "Fichas", 0, on(g.tokenModal)) + 6
	x += g.button(x, 8, 0, 30, "Dado", 0, g.diceModal) + 6
	// lado direito da barra
	rxb := W - 10
	rb := func(label string, style int, fn func()) {
		w := tw(label, fReg, 14) + 22
		rxb -= w
		g.button(rxb, 8, w, 30, label, style, fn)
		rxb -= 6
	}
	rb("Sair", 2, func() {
		g.confirm("Sair da partida e voltar para a lista de decks?", func() { g.screen, g.S = "home", nil; g.pv = nil })
	})
	rb("Reiniciar", 0, func() { g.confirm("Começar uma partida nova com este deck?", func() { g.startGame(g.deck) }) })
	st := 0
	if len(g.undo) == 0 {
		st = 3
	}
	rb("Desfazer", st, g.undoLast)
	size := func(d float64) func() {
		return func() { g.cw = max(70, min(190, g.cw+d)); cfg.Size = g.cw; saveCfg() }
	}
	rb("+", 0, size(10))
	rb("-", 0, size(-10))
	if rxb-46 > x {
		txt(g.dst, "Cartas", rxb-42, 15, fReg, 12.5, cMut)
	}
}

/* ====================== início ====================== */

func main() {
	baseDir = exeDir()
	os.MkdirAll(filepath.Join(baseDir, "decks"), 0o755)
	os.MkdirAll(filepath.Join(baseDir, "dados", "imagens"), 0o755)
	if f, err := os.OpenFile(filepath.Join(baseDir, "dados", "erros.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(f)
	}
	loadCfg()
	initFonts()
	if err := loadDB(); err != nil {
		log.Fatal("banco de cartas: ", err)
	}
	g := &Game{screen: "home", customs: map[string]*Card{}, proxies: map[string]*ebiten.Image{}, fields: map[string]string{}, cw: cfg.Size}
	dl.msg.Store("")
	loadMine()
	buildIndex()
	go loadAnalysis() // o banco de combos carrega em segundo plano
	g.reloadDecks("")
	if os.Getenv("MESA_TEST") != "" {
		g.test = &tester{dir: os.Getenv("MESA_TEST")}
	}
	ebiten.SetWindowSize(1400, 860)
	ebiten.SetWindowSizeLimits(1000, 640, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("Mesa Arcana")
	ebiten.SetWindowIcon(windowIcons())
	if err := ebiten.RunGame(g); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}
