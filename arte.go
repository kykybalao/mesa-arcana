package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Escolha de arte: lista todas as impressões da carta (ou da ficha) no Scryfall e baixa a escolhida
// no lugar da imagem padrão. Precisa de internet com o Scryfall liberado.

type artPrint struct {
	id, set, setName, num, year string
	small, normal, back         string
	raw                         *image.RGBA
	tex                         *ebiten.Image
	failed                      bool
}

type artPick struct {
	mu     sync.Mutex
	prints []*artPrint
	status string
	busy   bool
	done   bool
}

func httpGet(u string) ([]byte, error) {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "MesaArcana/1.0")
	req.Header.Set("Accept", "application/json, image/*")
	cl := &http.Client{Timeout: 30 * time.Second}
	res, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("resposta %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 8<<20))
}

func (a *artPick) set(s string) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
}

func (a *artPick) load(c *Card) {
	name := c.FaceAt(0).N
	q := fmt.Sprintf(`!"%s" game:paper`, name)
	if c.TK != 0 {
		q = fmt.Sprintf(`!"%s" t:token`, name)
	}
	u := "https://api.scryfall.com/cards/search?" + url.Values{"q": {q}, "unique": {"prints"}, "order": {"released"}, "include_extras": {"true"}, "include_variations": {"true"}}.Encode()
	b, err := httpGet(u)
	if err != nil {
		a.set("Não consegui falar com o Scryfall nesta rede. A troca de arte só funciona com internet liberada.")
		return
	}
	type uris struct {
		Small  string `json:"small"`
		Normal string `json:"normal"`
	}
	var res struct {
		Data []struct {
			ID    string `json:"id"`
			Set   string `json:"set"`
			SetN  string `json:"set_name"`
			Num   string `json:"collector_number"`
			Date  string `json:"released_at"`
			Type  string `json:"type_line"`
			Text  string `json:"oracle_text"`
			P     string `json:"power"`
			T     string `json:"toughness"`
			Imgs  *uris  `json:"image_uris"`
			Faces []struct {
				Imgs *uris `json:"image_uris"`
			} `json:"card_faces"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &res) != nil {
		a.set("O Scryfall respondeu algo que não entendi.")
		return
	}
	var out []*artPrint
	for _, d := range res.Data {
		if c.TK != 0 && (d.Type != c.T || d.P != c.P || d.T != c.O || strings.TrimSpace(d.Text) != strings.TrimSpace(c.X)) {
			continue // ficha com o mesmo nome mas outro tipo, texto ou tamanho
		}
		p := &artPrint{id: d.ID, set: strings.ToUpper(d.Set), setName: d.SetN, num: d.Num}
		if len(d.Date) >= 4 {
			p.year = d.Date[:4]
		}
		if d.Imgs != nil {
			p.small, p.normal = d.Imgs.Small, d.Imgs.Normal
		} else if len(d.Faces) > 0 && d.Faces[0].Imgs != nil {
			p.small, p.normal = d.Faces[0].Imgs.Small, d.Faces[0].Imgs.Normal
			if len(d.Faces) > 1 && d.Faces[1].Imgs != nil {
				p.back = d.Faces[1].Imgs.Normal
			}
		}
		if p.normal != "" {
			out = append(out, p)
		}
		if len(out) >= 120 {
			break
		}
	}
	a.mu.Lock()
	a.prints = out
	a.status = fmt.Sprintf("%d artes encontradas. Clique em uma para baixar.", len(out))
	if len(out) == 0 {
		a.status = "Não achei outras artes para esta carta."
	}
	a.mu.Unlock()
	// miniaturas, quatro por vez
	jobs := make(chan *artPrint)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				var raw *image.RGBA
				if b, err := httpGet(p.small); err == nil {
					if src, _, err := image.Decode(strings.NewReader(string(b))); err == nil {
						raw = image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
						draw.Draw(raw, raw.Bounds(), src, src.Bounds().Min, draw.Src)
					}
				}
				a.mu.Lock()
				p.raw, p.failed = raw, raw == nil
				a.mu.Unlock()
			}
		}()
	}
	for _, p := range out {
		a.mu.Lock()
		stop := a.done
		a.mu.Unlock()
		if stop {
			break
		}
		jobs <- p
	}
	close(jobs)
	wg.Wait()
}

// choose baixa a arte escolhida por cima da imagem atual da carta
func (a *artPick) choose(c *Card, p *artPrint) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return
	}
	a.busy, a.status = true, "Baixando a arte de "+p.setName+"…"
	a.mu.Unlock()
	go func() {
		msg := "Arte trocada: " + p.setName + " (" + p.year + ")."
		save := func(key, u string) bool {
			b, err := httpGet(u)
			if err != nil || len(b) < 1000 {
				return false
			}
			os.MkdirAll(filepath.Dir(imgPath(key)), 0o755)
			if os.WriteFile(imgPath(key)+".tmp", b, 0o644) != nil || os.Rename(imgPath(key)+".tmp", imgPath(key)) != nil {
				return false
			}
			imgMu.Lock()
			delete(imgCache, key)
			imgMu.Unlock()
			return true
		}
		if !save(c.ID+":0", p.normal) {
			msg = "Não consegui baixar essa imagem. Confira a internet e tente de novo."
		} else if p.back != "" && len(c.F) > 1 {
			save(c.ID+":1", p.back)
		}
		a.mu.Lock()
		a.busy, a.status = false, msg
		a.mu.Unlock()
	}()
}

func (g *Game) artModal(c *Card) {
	if c == nil || c.Custom {
		return
	}
	a := &artPick{status: "Procurando as artes no Scryfall…"}
	go a.load(c)
	m := &Modal{title: "Escolher a arte de " + c.FaceAt(0).N, w: 980, h: 740}
	m.onClose = func() {
		a.mu.Lock()
		a.done = true
		a.mu.Unlock()
	}
	m.draw = func(x, y, w, h float64) {
		a.mu.Lock()
		prints, status, busy := a.prints, a.status, a.busy
		for _, p := range prints {
			if p.raw != nil {
				p.tex, p.raw = ebiten.NewImageFromImage(p.raw), nil
			}
		}
		a.mu.Unlock()
		col := cAcc2
		if busy {
			col = cAcc
		}
		txt(g.dst, fit(status, fBold, 13.5, w), x, y, fBold, 13.5, col)
		const tw0, gap = 118.0, 14.0
		th := tw0 * ratio
		cols := int((w + gap) / (tw0 + gap))
		rowH := th + 40
		top, listH := y+28, h-28-44
		rows := (len(prints) + cols - 1) / cols
		m.scroll = max(0, min(m.scroll, float64(rows)*rowH-listH))
		g.add(region{x: x, y: top, w: w, h: listH, wheel: func(dy float64) { m.scroll -= dy * 70 }})
		old := g.dst
		g.dst = clip(old, x, top, w, listH)
		for k, p := range prints {
			cx := x + float64(k%cols)*(tw0+gap)
			cy := top + float64(k/cols)*rowH - m.scroll
			if cy+rowH < top || cy > top+listH {
				continue
			}
			if p.tex != nil {
				drawTex(g.dst, p.tex, cx, cy, tw0, false, 1)
			} else {
				rrect(g.dst, cx, cy, tw0, th, 6, cPanel2)
				lab := "carregando…"
				if p.failed {
					lab = "sem miniatura"
				}
				txtC(g.dst, lab, cx+tw0/2, cy+th/2-8, fReg, 11.5, cMut)
			}
			txtC(g.dst, fit(p.setName, fReg, 11.5, tw0+8), cx+tw0/2, cy+th+4, fReg, 11.5, cTxt)
			txtC(g.dst, p.set+" "+p.num+" · "+p.year, cx+tw0/2, cy+th+19, fReg, 10.5, cMut)
			if cy >= top-th && cy+th <= top+listH+th {
				g.add(region{x: cx, y: max(cy, top), w: tw0, h: min(th, top+listH-max(cy, top)), click: func() { a.choose(c, p) }})
			}
		}
		g.dst = old
		txt(g.dst, "A imagem escolhida fica guardada em dados/imagens e vale para todos os decks.", x, y+h-26, fReg, 12.5, cMut)
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}
	g.openModal(m)
}
