package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// tester roda um roteiro automático (MESA_TEST=pasta) e grava capturas de tela.
type tester struct {
	dir         string
	n           int
	fake        bool
	mx, my      int
	ld, pld, rp bool
	id, cnt     int
	pxWait      int
}

// centro da região de uma carta em jogo
func (t *tester) at(g *Game, id int) (int, int) {
	for k := len(g.regs) - 1; k >= 0; k-- {
		if r := g.regs[k]; r.inst == id {
			return int(r.x + r.w/2), int(r.y + r.h/2)
		}
	}
	panic("carta sem região")
}

func (t *tester) step(g *Game) error {
	if !g.drawn {
		return nil
	}
	g.drawn = false
	if t.pxWait > 0 {
		t.pxWait++
		switch t.pxWait {
		case 60:
			g.shot = filepath.Join(t.dir, "px-janela.png")
		case 64:
			g.closeModal()
			g.myModal()
			pxTest = nil
		case 70:
			g.shot = filepath.Join(t.dir, "px-criador.png")
		case 74:
			fmt.Println("TESTE PX OK")
			return ebiten.Termination
		}
		return nil
	}
	t.n++
	shot := func(name string) { g.shot = filepath.Join(t.dir, name+".png") }
	chk := func(ok bool, what string) {
		if !ok {
			panic("TESTE FALHOU: " + what)
		}
	}
	s := g.S
	switch t.n {
	case 20:
		chk(len(g.decks) > 0, "sem decks")
		shot("1-inicio")
	case 22:
		g.pasteDeck()
		e := g.ed
		chk(e != nil && g.screen == "edit", "editor abre")
		if e.total() == 0 { // sem área de transferência (emulador): usa o arquivo de teste
			b, _ := os.ReadFile(os.Getenv("MESA_CLIP"))
			g.edAddText(string(b))
		}
		fmt.Println("editor:", e.note, "| nome:", g.fields["edname"], "| cartas:", e.total(), "| faltando:", len(e.miss))
		chk(e.total() == 10 && len(e.miss) == 2 && e.cmdCount() == 1, "leitura de formatos")
		for _, m := range e.miss {
			var s []string
			for _, c := range m.sugg {
				s = append(s, c.N)
			}
			fmt.Println("  sugestões para", m.name, "->", s)
		}
		chk(len(e.miss[0].sugg) > 0 && e.miss[0].sugg[0].N == "Sol Ring", "sugestão de nome")
		chk(len(searchCards("elenda", 9)) >= 2 && searchCards("sol ri", 9)[0].N == "Sol Ring", "busca")
		g.fields["edq"] = "blood art"
		{
			var cs, cm []*Card
			for _, n := range []string{"Judge Magister Gabranth", "Sanguine Bond", "Exquisite Blood", "Rhystic Study", "Time Warp", "Viscera Seer", "Sol Ring", "Armageddon", "Demonic Tutor", "Reassembling Skeleton", "Ashnod's Altar"} {
				cs = append(cs, findCard(n))
			}
			cm = cs[:1]
			a := analyze(cs, cm)
			fmt.Println("análise:", a.title(), "| GC", a.GC, "| MLD", a.MLD, "| ET", a.ET, "| combos", len(a.In), "| tpl", len(a.InTpl), "| quase", len(a.Almost))
			for _, r := range a.Reasons {
				fmt.Println("   ", r)
			}
			chk(a.Bracket == 4 && len(a.GC) == 2 && len(a.MLD) == 1 && len(a.ET) == 1 && len(a.In) >= 1, "análise de bracket")
			found := false
			for _, c := range a.In {
				if len(c.C) == 2 && strings.Contains(comboLine(c), "Sanguine Bond") && strings.Contains(comboLine(c), "Exquisite Blood") {
					found = true
				}
			}
			chk(found, "combo Sanguine Bond + Exquisite Blood")
			b := analyze(cs[:1], cm)
			chk(b.Bracket == 1 && len(b.In) == 0, "deck sem nada fica 1 ou 2")
		}
		shot("1b-editor")
	case 23:
		e := g.ed
		chk(len(e.results) > 0 && e.results[0].N == "Blood Artist", "resultado da busca")
		m := e.miss[0]
		e.add(m.sugg[0], m.qty, false)
		e.miss = e.miss[1:]
		g.edSave()
		fmt.Println("salvar:", g.msg)
		chk(g.screen == "home" && g.decks[g.sel].Total == 11 && len(g.decks[g.sel].Cmd) == 1, "salvar deck")
		g.openEditor(g.decks[g.sel], "")
		chk(g.ed.total() == 11 && g.ed.cmdCount() == 1 && len(g.ed.miss) == 1, "reabrir deck")
		g.screen, g.ed = "home", nil
	case 24:
		switch os.Getenv("MESA_NET") {
		case "host":
			nw.host("Anfitrião")
		case "join":
			nw.join("Visitante", "127.0.0.1")
		}
		g.reloadDecks("Gabranth")
		g.startGame(g.decks[g.sel])
	case 26:
		chk(len(s.Z["hand"]) == 7 && len(s.Z["library"])+len(s.Z["command"]) == g.deck.Total-7, "mão inicial")
		g.mulligan()
		g.mulligan()
		g.keep()
		chk(s.Phase == "bottom", "fase fundo")
		g.clickCard(s.Z["hand"][0])
		shot("2-mulligan")
	case 30:
		g.confirmBottom()
		chk(g.S.Phase == "play" && len(g.S.Z["hand"]) == 6, "mulligan 6")
		for k := 0; k < 4; k++ {
			g.snap()
			g.move(g.S.Z["hand"][0], "battlefield", moveOpt{})
		}
		g.clickCard(g.S.Z["battlefield"][0])
		chk(g.S.Cards[g.S.Z["battlefield"][0]].Tapped, "virar")
		h0 := len(g.S.Z["hand"])
		g.nextTurn()
		g.nextTurn()
		chk(len(g.S.Z["hand"]) == h0 && g.S.Turn == 3 && g.S.Step == 0, "turno não compra sozinho")
		g.nextStep()
		g.nextStep()
		chk(g.S.Step == 2, "fases")
		bf := g.S.Z["battlefield"]
		g.attach(bf[1], bf[2])
		g.attach(bf[3], bf[2])
		g.syncAtt()
		chk(g.S.Cards[bf[1]].Att == bf[2] && g.S.Cards[bf[1]].Y < g.S.Cards[bf[2]].Y, "equipar")
		if c := g.S.Z["command"]; len(c) > 0 {
			id := c[0]
			g.snap()
			g.move(id, "battlefield", moveOpt{})
			g.S.Cards[id].Counters["+1/+1"] = 3
			g.snap()
			g.move(id, "command", moveOpt{})
			g.move(id, "battlefield", moveOpt{})
			chk(g.S.Tax[g.S.Cards[id].Cid] == 0, "imposto não é automático")
			g.S.Tax[g.S.Cards[id].Cid] = 1
			g.S.Cards[id].Counters["+1/+1"] = 2
			g.S.Cards[id].Tapped = true
			g.pv = g.card(g.S.Cards[id])
		}
		if len(g.deck.Tokens) > 0 {
			g.addToken(g.deck.Tokens[0], 2)
		}
		g.snap()
		g.move(g.S.Z["hand"][0], "graveyard", moveOpt{})
		g.move(g.S.Z["library"][0], "exile", moveOpt{})
		n := len(g.S.Z["battlefield"])
		g.undoLast()
		chk(len(g.S.Z["exile"]) == 0 && len(g.S.Z["battlefield"]) == n, "desfazer")
		shot("3-mesa")
	case 32:
		cards, cmd := deckCards(g.deck)
		g.analysisModal(g.deck.Name, cards, cmd)
		shot("3b-analise")
	case 33:
		g.closeModal()
	case 34:
		g.viewZone("library", true)
		shot("4-buscar")
	case 38:
		g.closeModal()
		g.scry(3)
		shot("5-videncia")
	case 42:
		g.closeModal()
		g.tokenModal()
		shot("6-fichas")
	case 43:
		g.closeModal()
		g.myModal()
		g.fields["mcn"], g.fields["mcc"], g.fields["mct"] = "Gabranth, Juiz Sombrio", "{2}{w}{b}", "Criatura Lendária — Humano Cavaleiro"
		g.fields["mcx"], g.fields["mcp"] = "Ameaçar / Toda vez que outra criatura morre, compre um card.", "3/3"
		shot("6c-criar")
	case 44:
		chk(findCard("Gabranth, Juiz Sombrio") == nil, "carta criada ainda não existe")
		mySave()
		if c := findCard("gabranth, juiz sombrio"); true {
			chk(c != nil && c.Mine && c.C == "{2}{W}{B}" && c.O == "3" && strings.Contains(c.X, "\n"), "criar carta")
			chk(len(searchCards("juiz sombrio", 5)) == 1, "carta criada aparece na busca")
			d := parseDeck("x", "1 Gabranth, Juiz Sombrio\n2 Sol Ring\n")
			chk(d.Total == 3, "carta criada entra no deck")
		}
		if os.Getenv("MESA_PX") != "" {
			t.px(g)
			return nil
		}
		g.closeModal()
		cfg.Mat, cfg.Sleeve = "b:Nebulosa", "b:Rubi"
		g.back = nil
		chk(findPT("Anel Solar") != nil && findPT("anel solar").N == "Sol Ring", "nome em português")
		if f, o := loc(findCard("Judge Magister Gabranth"), 0); true {
			chk(o == 2 && strings.HasPrefix(f.X, "Ameaçar"), "tradução própria")
		}
		g.looksModal()
		shot("6b-aparencia")
	case 46:
		g.closeModal()
		g.mx, g.my = 500, 200
		g.cardMenu(g.S.Z["battlefield"][len(g.S.Z["battlefield"])-3])
		shot("7-menu")
	case 47:
		g.ctx = nil
		if os.Getenv("MESA_NET") != "" { // espera o outro jogador aparecer com cartas no campo
			ps := nw.peerList()
			if !g.showHand && g.S.Phase == "play" {
				g.toggleReveal()
				g.fields["chat"] = "oi, aqui é " + os.Getenv("MESA_NET")
				g.sendChat()
			}
			if len(ps) == 0 || ps[0].State == nil || len(ps[0].State.BF) == 0 || len(ps[0].State.Shown) == 0 || len(g.chat) < 2 {
				t.n--
				t.cnt++
				chk(t.cnt < 3000, "rede: o outro jogador não apareceu")
				return nil
			}
			fmt.Println("rede: vejo", ps[0].Name, "| campo", len(ps[0].State.BF), "| mão", ps[0].State.Hand, "| vida", ps[0].State.Life, "| registro", len(ps[0].Log))
			chk(ps[0].State.Hand > 0 && ps[0].State.Library > 0 && len(ps[0].Log) > 0, "rede: estado público")
			for _, l := range ps[0].Log {
				chk(!strings.Contains(l, "comprou ") || !strings.Contains(l, ":"), "rede: registro não revela compra")
			}
			fmt.Println("rede: mão revelada", len(ps[0].State.Shown), "| chat", g.chat)
			g.viewPeer = ps[0].ID
			shot("7b-rede")
		}
	case 48:
		if os.Getenv("MESA_ARTE") != "" {
			g.artModal(findCard("Sol Ring"))
			t.cnt = 0
		}
	case 49:
		if os.Getenv("MESA_ARTE") != "" {
			if t.cnt++; t.cnt < 90 {
				t.n--
				return nil
			}
			if t.cnt == 90 {
				shot("7c-arte")
				t.n--
				return nil
			}
			g.closeModal()
		}
		g.viewPeer = 0
	case 50: // daqui em diante, mouse simulado: arrastar da mão para o campo
		g.ctx = nil
		t.fake = true
		t.mx, t.my = 5, 300
	case 51:
		t.id = g.S.Z["hand"][0]
		t.cnt = len(g.S.Z["battlefield"])
		t.mx, t.my = t.at(g, t.id)
		t.ld = true
	case 52:
		t.mx, t.my = t.mx+40, t.my-60
	case 54:
		t.mx, t.my = g.bf.Min.X+600, g.bf.Min.Y+330
	case 56:
		t.ld = false
	case 58:
		i := g.S.Cards[t.id]
		chk(i.Zone == "battlefield" && len(g.S.Z["battlefield"]) == t.cnt+1, "arrastar para o campo")
		chk(i.X > 400 && i.Y > 150, "posição ao soltar")
		t.ld = true // clique simples vira a carta
	case 60:
		t.ld = false
	case 62:
		chk(g.S.Cards[t.id].Tapped, "clique vira")
		t.rp = true // botão direito abre o menu
	case 64:
		chk(g.ctx != nil, "menu de contexto")
		shot("8-menu-mouse")
	case 66:
		// acha a linha "Cemitério" pelas regiões do menu (25 px por linha)
		y := 0.0
		c := g.ctx
		h := 8.0
		for _, it := range c.items {
			if it.hd != "" {
				h += 22
			} else {
				h += 25
			}
		}
		cy := max(4, min(c.y, float64(g.H)-h-6)) + 4
		for _, it := range c.items {
			if it.hd != "" {
				cy += 22
				continue
			}
			if it.label == "Cemitério" {
				y = cy + 12
			}
			cy += 25
		}
		t.mx, t.my = int(max(4, min(c.x, float64(g.W)-276))+60), int(y)
		t.ld = true
	case 68:
		t.ld = false
	case 70:
		chk(g.ctx == nil && g.S.Cards[t.id].Zone == "graveyard", "menu moveu para o cemitério")
		t.cnt = len(g.S.Z["hand"])
		t.mx, t.my = t.at(g, g.S.Z["library"][0]) // clique no grimório compra
		t.ld = true
	case 72:
		t.ld = false
	case 74:
		chk(len(g.S.Z["hand"]) == t.cnt+1, "clique no grimório compra")
		t.id = g.S.Z["hand"][1] // arrastar da mão para o exílio
		t.mx, t.my = t.at(g, t.id)
		t.ld = true
	case 76:
		t.my -= 80
	case 78:
		for _, r := range g.regs {
			if r.zone == "exile" {
				t.mx, t.my = int(r.x+r.w/2), int(r.y+r.h/2)
			}
		}
	case 80:
		t.ld = false
	case 82:
		chk(g.S.Cards[t.id].Zone == "exile", "arrastar para o exílio")
		t.id = g.S.Z["hand"][0] // clique duplo joga da mão
		t.mx, t.my = t.at(g, t.id)
		t.ld = true
	case 83:
		t.ld = false
	case 85:
		t.ld = true
	case 86:
		t.ld = false
	case 88:
		chk(g.S.Cards[t.id].Zone == "battlefield", "clique duplo joga")
		t.mx, t.my = t.at(g, g.S.Z["graveyard"][len(g.S.Z["graveyard"])-1]) // clique no cemitério abre a lista
		t.ld = true
	case 89:
		t.ld = false
	case 91:
		chk(g.modal != nil, "cemitério abre janela")
		shot("9-cemiterio")
	case 95:
		fmt.Println("TESTE OK")
		return ebiten.Termination
	}
	return nil
}

// px: roteiro só do fazedor de proxies (MESA_PX=1); grava as cartas em tamanho real na pasta de capturas
func (t *tester) px(g *Game) {
	g.closeModal()
	dump := func(name string, c *Card, st string) {
		im := g.printable(c, 0, st)
		writePNG(filepath.Join(t.dir, name+".png"), grab(im))
		im.Deallocate()
	}
	art := os.Getenv("MESA_PXART")
	for _, n := range []string{"Judge Magister Gabranth", "Sol Ring", "Blood Artist", "Counterspell", "Atraxa, Praetors' Voice", "Swords to Plowshares", "Forest", "Fire // Ice", "Command Tower", "Kitchen Finks", "Liliana of the Veil", "Rhystic Study"} {
		c := findCard(n)
		if c == nil {
			fmt.Println("px: sem", n)
			continue
		}
		if art != "" {
			b, _ := os.ReadFile(art)
			os.MkdirAll(filepath.Dir(artPath(c.ID+":0")), 0o755)
			os.WriteFile(artPath(c.ID+":0"), b, 0o644)
		}
		for _, st := range []string{"moldura", "estendida", "cena"} {
			dump(safeName(n)+"-"+st, c, st)
		}
	}
	mine := findCard("gabranth, juiz sombrio")
	mine.Est, mine.Sub = "cena", "Judge Magister Gabranth"
	dump("minha-cena", mine, "cena")
	dump("playtest", findCard("Sol Ring"), "")
	g.proxyModal()
	pxTest("folhas")
	t.pxWait = 1
}
