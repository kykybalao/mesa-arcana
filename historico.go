package main

// Histórico de partidas (ideia do EDHPlay): o jogador registra o resultado quando quiser,
// e a janela mostra as últimas partidas e o resumo por deck. Nada é gravado sozinho.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Played struct {
	When    string   `json:"quando"`
	Deck    string   `json:"deck"`
	Cmdrs   []string `json:"comandantes,omitempty"`
	Result  string   `json:"resultado"` // venci, perdi, empate, teste
	Turn    int      `json:"turno"`
	Minutes int      `json:"minutos"`
	Players int      `json:"jogadores"`
}

func histPath() string { return filepath.Join(baseDir, "dados", "historico.json") }

func loadHist() []Played {
	var h []Played
	if b, err := os.ReadFile(histPath()); err == nil {
		json.Unmarshal(b, &h)
	}
	return h
}

func saveHist(h []Played) {
	os.MkdirAll(filepath.Dir(histPath()), 0o755)
	if b, err := json.MarshalIndent(h, "", " "); err == nil {
		os.WriteFile(histPath(), b, 0o644)
	}
}

var resultLabel = map[string]string{"venci": "Venci", "perdi": "Perdi", "empate": "Empate", "teste": "Só teste"}

func (g *Game) recordGame(res string) {
	if g.S == nil || g.deck == nil {
		return
	}
	p := Played{When: time.Now().Format("02/01/2006 15:04"), Deck: g.deck.Name, Result: res, Turn: g.S.Turn, Players: 1}
	if !g.started.IsZero() {
		p.Minutes = int(time.Since(g.started).Minutes() + 0.5)
	}
	if nw.active() {
		p.Players += len(nw.peerList())
	}
	seen := map[string]bool{}
	for _, i := range g.S.Cards {
		if i.Cmdr {
			if n := g.card(i).N; !seen[n] {
				seen[n] = true
				p.Cmdrs = append(p.Cmdrs, n)
			}
		}
	}
	sort.Strings(p.Cmdrs)
	saveHist(append(loadHist(), p))
	g.saved = true
	g.announce("Partida registrada: " + resultLabel[res])
}

func (g *Game) historyModal() {
	g.openModal(&Modal{title: "Resultado e histórico", w: 720, h: 520, draw: func(x, y, w, h float64) {
		hist := loadHist()
		yy := y
		if g.S != nil && g.screen == "game" {
			if g.saved {
				txt(g.dst, "Esta partida já foi registrada.", x, yy+4, fReg, 13, cMut)
			} else {
				txt(g.dst, fmt.Sprintf("Registrar esta partida (turno %d):", g.S.Turn), x, yy+4, fReg, 13, cTxt)
				bx := x + 250
				for _, r := range []string{"venci", "perdi", "empate", "teste"} {
					r := r
					st := 0
					if r == "venci" {
						st = 1
					}
					bx += g.button(bx, yy, 0, 28, resultLabel[r], st, func() { g.recordGame(r) }) + 6
				}
			}
			yy += 44
		}
		// resumo por deck
		type sum struct {
			n, win, turns int
		}
		per := map[string]*sum{}
		var names []string
		for _, p := range hist {
			if p.Result == "teste" {
				continue
			}
			s := per[p.Deck]
			if s == nil {
				s = &sum{}
				per[p.Deck] = s
				names = append(names, p.Deck)
			}
			s.n++
			if p.Result == "venci" {
				s.win++
				s.turns += p.Turn
			}
		}
		sort.Strings(names)
		txt(g.dst, "Por deck", x, yy, fBold, 13, cAcc)
		yy += 22
		if len(names) == 0 {
			txt(g.dst, "Nenhuma partida registrada ainda.", x, yy, fReg, 12.5, cMut)
			yy += 20
		}
		for k, n := range names {
			if k >= 5 {
				break
			}
			s := per[n]
			line := fmt.Sprintf("%s: %d partidas, %d vitórias (%d%%)", n, s.n, s.win, s.win*100/s.n)
			if s.win > 0 {
				line += fmt.Sprintf(", vence em média no turno %.1f", float64(s.turns)/float64(s.win))
			}
			txt(g.dst, fit(line, fReg, 12.5, w), x, yy, fReg, 12.5, cTxt)
			yy += 19
		}
		yy += 10
		txt(g.dst, "Últimas partidas", x, yy, fBold, 13, cAcc)
		yy += 22
		for k := len(hist) - 1; k >= 0 && yy < y+h-20; k-- {
			p := hist[k]
			cm := strings.Join(p.Cmdrs, " + ")
			if cm == "" {
				cm = p.Deck
			}
			line := fmt.Sprintf("%s  ·  %s  ·  %s  ·  turno %d  ·  %d min", p.When, resultLabel[p.Result], cm, p.Turn, p.Minutes)
			if p.Players > 1 {
				line += fmt.Sprintf("  ·  %d jogadores", p.Players)
			}
			txt(g.dst, fit(line, fReg, 12, w), x, yy, fReg, 12, cTxt)
			yy += 19
		}
	}})
}
