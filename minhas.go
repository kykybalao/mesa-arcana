package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Cartas criadas pelo usuário: ficam em dados/minhas-cartas.json e entram no banco como qualquer outra carta
// (aparecem na busca do editor de decks e são reconhecidas pelo nome nas listas).

var myCards []*Card
var mySave func() // salvar da janela aberta (usado pelo roteiro de teste)

func myPath() string { return filepath.Join(baseDir, "dados", "minhas-cartas.json") }

func registerMine(c *Card) {
	c.Custom, c.Mine = true, true
	cardByID[c.ID] = c
	cardByName[strings.ToLower(c.N)] = c
}

func loadMine() {
	b, err := os.ReadFile(myPath())
	if err != nil {
		return
	}
	var cs []*Card
	if json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &cs) != nil {
		return
	}
	for _, c := range cs {
		if c != nil && c.ID != "" && c.N != "" {
			myCards = append(myCards, c)
			registerMine(c)
		}
	}
}

func saveMine() error {
	sort.Slice(myCards, func(a, b int) bool { return strings.ToLower(myCards[a].N) < strings.ToLower(myCards[b].N) })
	b, err := json.MarshalIndent(myCards, "", " ")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(myPath()), 0o755)
	return os.WriteFile(myPath(), b, 0o644)
}

// reindex refaz a busca por nome depois de criar, mudar ou apagar uma carta
func reindex() {
	allCards, allLower, allSq = nil, nil, nil
	buildIndex()
}

func (g *Game) forgetProxy(id string) {
	for k := range g.proxies {
		if strings.HasPrefix(k, id+":") {
			delete(g.proxies, k)
		}
	}
	imgMu.Lock()
	delete(imgCache, id+":0")
	imgMu.Unlock()
}

var myFields = []string{"mcn", "mcc", "mct", "mcx", "mcp", "mcl", "mcs"}

// myModal: lista das cartas criadas à esquerda, formulário e prévia à direita
func (g *Game) myModal() {
	var cur *Card // carta sendo editada (nil = nova)
	msg := ""
	est, cor := "", "" // estilo e cor da moldura da carta aberta
	var pv *ebiten.Image
	pvKey := ""
	var saving *pxExport
	fill := func(c *Card) {
		cur = c
		est, cor = "", ""
		for _, k := range myFields {
			g.fields[k] = ""
		}
		if c != nil {
			g.fields["mcn"], g.fields["mcc"], g.fields["mct"] = c.N, c.C, c.T
			g.fields["mcx"] = strings.ReplaceAll(c.X, "\n", " / ")
			if c.P != "" || c.O != "" {
				g.fields["mcp"] = c.P + "/" + c.O
			}
			g.fields["mcl"] = c.L
			g.fields["mcs"] = c.Sub
			est, cor = c.Est, c.Cor
		}
	}
	fill(nil)
	draft := func() *Card {
		p, o, _ := strings.Cut(strings.TrimSpace(g.fields["mcp"]), "/")
		c := &Card{ID: "rascunho", Custom: true, Mine: true, Face: Face{N: strings.TrimSpace(g.fields["mcn"]), C: strings.ToUpper(strings.TrimSpace(g.fields["mcc"])),
			T: strings.TrimSpace(g.fields["mct"]), X: strings.ReplaceAll(strings.TrimSpace(g.fields["mcx"]), " / ", "\n"), P: strings.TrimSpace(p), O: strings.TrimSpace(o), L: strings.TrimSpace(g.fields["mcl"])}}
		c.Est, c.Cor, c.Sub = est, cor, strings.TrimSpace(g.fields["mcs"])
		if cur != nil {
			c.ID = cur.ID
		}
		return c
	}
	save := func() *Card {
		d := draft()
		if d.N == "" {
			msg = "Dê um nome para a carta."
			return nil
		}
		if d.T == "" {
			d.T = "Criatura"
		}
		if old := cardByName[strings.ToLower(d.N)]; old != nil && old != cur {
			if old.Mine {
				msg = "Você já criou uma carta com esse nome."
			} else {
				msg = "Já existe uma carta de verdade com esse nome. Escolha outro."
			}
			return nil
		}
		if cur == nil {
			// o final aleatório evita confusão com a carta de mesmo nome de outro jogador na rede
			id := fmt.Sprintf("meu-%s-%04x", squash(d.N), time.Now().UnixNano()&0xffff)
			for cardByID[id] != nil {
				id += "x"
			}
			d.ID = id
			cur = d
			myCards = append(myCards, d)
		} else {
			delete(cardByName, strings.ToLower(cur.N))
			cur.Face = d.Face
		}
		cur.Est, cur.Cor, cur.Sub = d.Est, d.Cor, d.Sub
		registerMine(cur)
		g.forgetProxy(cur.ID)
		reindex()
		if err := saveMine(); err != nil {
			msg = "Não consegui gravar: " + shortErr(err)
		} else {
			msg = "“" + cur.N + "” salva. Use o nome dela nas listas de deck."
		}
		return cur
	}
	mySave = func() { save() }
	m := &Modal{title: "Minhas cartas", w: 1120, h: 720}
	m.onClose = func() {
		if pv != nil {
			pv.Deallocate()
		}
	}
	m.draw = func(x, y, w, h float64) {
		if saving != nil && !saving.step(g) {
			if saving.fail.Load() > 0 {
				msg = "Não consegui gravar o PNG na pasta proxies."
			} else {
				msg = "PNG gravado na pasta proxies, pronto para imprimir."
			}
			saving = nil
		}
		// imagem arrastada para a janela vira a arte da carta que está aberta
		if fsys := ebiten.DroppedFiles(); fsys != nil {
			if cur == nil {
				msg = "Salve a carta antes de arrastar a imagem."
			} else if es, err := fs.ReadDir(fsys, "."); err == nil && len(es) > 0 {
				if b, err := fs.ReadFile(fsys, es[0].Name()); err == nil {
					if _, _, err := image.Decode(bytes.NewReader(b)); err != nil {
						msg = "Esse arquivo não é uma imagem PNG ou JPG."
					} else {
						os.MkdirAll(filepath.Dir(imgPath(cur.ID+":0")), 0o755)
						os.WriteFile(imgPath(cur.ID+":0"), b, 0o644)
						g.forgetProxy(cur.ID)
						pvKey = ""
						msg = "Imagem colocada em “" + cur.N + "”."
					}
				}
			}
		}
		// lista
		lw := 250.0
		txt(g.dst, fmt.Sprintf("Criadas (%d)", len(myCards)), x, y, fBold, 14.5, cAcc)
		g.button(x+lw-96, y-4, 96, 26, "Nova carta", 0, func() { fill(nil); msg = "" })
		listY, listH := y+32, h-32-44
		m.scroll = max(0, min(m.scroll, float64(len(myCards))*34-listH))
		g.add(region{x: x, y: listY, w: lw, h: listH, wheel: func(dy float64) { m.scroll -= dy * 34 }})
		old := g.dst
		g.dst = clip(old, x, listY, lw, listH)
		for k, c := range myCards {
			cy := listY + float64(k)*34 - m.scroll
			if cy+34 < listY || cy > listY+listH {
				continue
			}
			bg := cPanel2
			if c == cur {
				bg = cLine
			}
			rrect(g.dst, x, cy, lw, 30, 6, bg)
			txt(g.dst, fit(c.N, fReg, 13.5, lw-20), x+10, cy+7, fReg, 13.5, cTxt)
			if cy >= listY && cy+30 <= listY+listH {
				g.add(region{x: x, y: cy, w: lw, h: 30, pv: c, click: func() { fill(c); msg = "" }})
			}
		}
		g.dst = old
		if len(myCards) == 0 {
			txt(g.dst, strings.Join(wrap("Nenhuma ainda. Preencha os campos ao lado e clique em Salvar.", fReg, 13, lw), "\n"), x, listY, fReg, 13, cMut)
		}
		// formulário
		fx := x + lw + 24
		pvW := 290.0
		fw := w - lw - 24 - pvW - 24
		title := "Nova carta"
		if cur != nil {
			title = "Editando: " + cur.N
		}
		txt(g.dst, fit(title, fBold, 14.5, fw), fx, y, fBold, 14.5, cAcc)
		row := func(lab, id, ph string, cy float64) {
			txt(g.dst, lab, fx, cy+7, fReg, 13, cMut)
			g.field(id, fx+92, cy, fw-92, ph)
		}
		row("Nome", "mcn", "Ex.: Gabranth, Juiz Sombrio", y+34)
		row("Custo", "mcc", "Ex.: {2}{W}{B}", y+72)
		row("Tipo", "mct", "Ex.: Criatura Lendária — Humano Cavaleiro", y+110)
		row("Texto", "mcx", "Use  /  (com espaços) para pular linha", y+148)
		row("P/R", "mcp", "Ex.: 3/3 (deixe vazio se não for criatura)", y+186)
		row("Lealdade", "mcl", "Só para planeswalker", y+224)
		row("Subtítulo", "mcs", "Segunda linha de nome (só no estilo Arte inteira)", y+262)
		txt(g.dst, "Estilo", fx, y+307, fReg, 13, cMut)
		sx := fx + 92
		for _, s := range styleNames {
			st := 0
			if s[0] == est {
				st = 1
			}
			sx += 6 + g.button(sx, y+300, 0, 30, s[1], st, func() { est = s[0] })
		}
		txt(g.dst, "Moldura", fx, y+345, fReg, 13, cMut)
		g.button(fx+92, y+338, 0, 30, "Cor: "+frameLabel(cor)+" (trocar)", 0, func() { cor = nextFrame(cor) })
		hy := y + 384
		imgHint := "Imagem (opcional): salve a carta e arraste um arquivo PNG ou JPG para dentro desta janela. No estilo Playtest a imagem é a carta inteira; nos outros ela é só a arte, e o programa desenha a moldura, o custo, o texto e a P/R."
		for _, l := range wrap(imgHint, fReg, 12.5, fw) {
			txt(g.dst, l, fx, hy, fReg, 12.5, cMut)
			hy += 16
		}
		for _, l := range wrap("Para usar: escreva o nome da carta na lista do deck, ou procure por ela no editor de decks.", fReg, 12.5, fw) {
			txt(g.dst, l, fx, hy, fReg, 12.5, cMut)
			hy += 16
		}
		if msg != "" {
			for _, l := range wrap(msg, fBold, 13.5, fw) {
				txt(g.dst, l, fx, hy+8, fBold, 13.5, cAcc2)
				hy += 17
			}
		}
		// prévia
		px := x + w - pvW
		d := draft()
		g.forgetProxyDraft()
		if est != "" {
			key := fmt.Sprint(d.Face, d.Est, d.Cor, d.Sub, d.ID)
			if key != pvKey {
				if pv != nil {
					pv.Deallocate()
				}
				pv, pvKey = renderCard(specOf(d, 0, est), 1), key
			}
			drawTex(g.dst, pv, px, y, pvW, false, 1)
		} else if cur != nil && hasImg(cur.ID+":0") {
			drawTex(g.dst, g.cardTex(cur, 0), px, y, pvW, false, 1)
		} else {
			d.ID = "rascunho"
			drawTex(g.dst, g.proxy(d, 0), px, y, pvW, false, 1)
		}
		// botões
		bx := fx
		bx += 8 + g.button(bx, y+h-32, 0, 30, "Salvar", 1, func() { save() })
		if cur != nil {
			if g.S != nil && g.S.Phase == "play" {
				bx += 8 + g.button(bx, y+h-32, 0, 30, "Pôr na minha mão", 0, func() {
					if c := save(); c != nil {
						g.snap()
						g.mk(c.ID, "hand")
						g.logP("", "pôs %s (carta criada) na mão", c.N)
						msg = "“" + c.N + "” foi para a sua mão."
					}
				})
			}
			bx += 8 + g.button(bx, y+h-32, 0, 30, "Gravar PNG", 0, func() {
				if c := save(); c != nil && saving == nil {
					saving = &pxExport{style: c.Est, dir: proxyDir(), total: 1, jobs: []pxJob{{c: c, path: filepath.Join(proxyDir(), safeName(c.N)+".png")}}}
				}
			})
			if hasImg(cur.ID + ":0") {
				bx += 8 + g.button(bx, y+h-32, 0, 30, "Tirar imagem", 0, func() {
					os.Remove(imgPath(cur.ID + ":0"))
					pvKey = ""
					g.forgetProxy(cur.ID)
					msg = "Imagem removida."
				})
			}
			g.button(bx, y+h-32, 0, 30, "Excluir", 2, func() {
				c := cur
				for k, x := range myCards {
					if x == c {
						myCards = append(myCards[:k], myCards[k+1:]...)
						break
					}
				}
				delete(cardByName, strings.ToLower(c.N))
				delete(cardByID, c.ID)
				g.customs[c.ID] = c // continua conhecida nesta sessão, para não quebrar uma partida em andamento
				reindex()
				saveMine()
				fill(nil)
				msg = "“" + c.N + "” excluída."
			})
		}
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}
	g.openModal(m)
}

func (g *Game) forgetProxyDraft() {
	for k := range g.proxies {
		if strings.HasPrefix(k, "rascunho:") {
			g.proxies[k].Deallocate()
			delete(g.proxies, k)
		}
	}
}
