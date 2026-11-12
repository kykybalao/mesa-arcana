package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed dados/cartas.json.gz
var dbGz []byte

// Face é um lado de uma carta (as chaves curtas vêm do banco compactado).
type Face struct {
	N string `json:"n"`
	T string `json:"t,omitempty"`
	C string `json:"c,omitempty"`
	X string `json:"x,omitempty"`
	P string `json:"p,omitempty"`
	O string `json:"o,omitempty"`
	L string `json:"l,omitempty"`
}

type Card struct {
	ID string `json:"i"`
	Face
	F      []Face   `json:"f,omitempty"`
	K      []string `json:"k,omitempty"`
	U      string   `json:"u,omitempty"`
	NI     int      `json:"ni,omitempty"`
	TK     int      `json:"tk,omitempty"`
	Custom bool     `json:"custom,omitempty"`
	Mine   bool     `json:"minha,omitempty"` // carta criada pelo usuário
	Est    string   `json:"est,omitempty"`   // estilo da carta criada: "" playtest, moldura, estendida, cena
	Sub    string   `json:"sub,omitempty"`   // segunda linha de nome (estilo cena)
	Cor    string   `json:"cor,omitempty"`   // cor da moldura ("" = automática)
	Mar    string   `json:"mar,omitempty"`   // símbolo da carta criada (licao, sol, lua, modal1, modal2, fusao)
	Dica   string   `json:"dica,omitempty"`  // linha pequena no pé do texto (ex.: "Verso: 4/4")
	Alta   bool     `json:"alta,omitempty"`  // carta alta de fusão: o PNG sai dividido em duas metades
}

func (c *Card) Faces() []Face {
	if len(c.F) > 0 {
		return c.F
	}
	return []Face{c.Face}
}
func (c *Card) FaceAt(i int) Face {
	f := c.Faces()
	if i < 0 || i >= len(f) {
		i = 0
	}
	return f[i]
}

var (
	cardByID   = map[string]*Card{}
	cardByName = map[string]*Card{}
	dbDate     string
	dbCount    int
	baseDir    string
)

func loadDB() error {
	raw := dbGz
	if b, err := os.ReadFile(filepath.Join(baseDir, "dados", "cartas.json.gz")); err == nil {
		raw = b // um banco mais novo na pasta dados tem prioridade
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	var db struct {
		Data   string  `json:"data"`
		Cartas []*Card `json:"cartas"`
	}
	if err := json.NewDecoder(zr).Decode(&db); err != nil {
		return err
	}
	dbDate = db.Data
	for _, c := range db.Cartas {
		cardByID[c.ID] = c
		if c.TK != 0 {
			continue
		}
		dbCount++
		k := strings.ToLower(c.N)
		if _, ok := cardByName[k]; !ok {
			cardByName[k] = c
		}
		if i := strings.Index(k, " // "); i > 0 {
			if _, ok := cardByName[k[:i]]; !ok {
				cardByName[k[:i]] = c
			}
		}
	}
	return nil
}

func findCard(name string) *Card {
	k := strings.ToLower(strings.TrimSpace(name))
	if c := cardByName[k]; c != nil {
		return c
	}
	if i := strings.Index(k, " // "); i > 0 {
		if c := cardByName[k[:i]]; c != nil {
			return c
		}
	}
	if c := findPT(k); c != nil { // nome em português
		return c
	}
	// tolera diferenças de acento, apóstrofo e pontuação
	sq := squash(k)
	for n, c := range cardByName {
		if squash(n) == sq {
			return c
		}
	}
	return nil
}

var squashRe = regexp.MustCompile(`[^a-z0-9]+`)

func squash(s string) string {
	r := strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "ö", "o", "ú", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n")
	return squashRe.ReplaceAllString(r.Replace(s), "")
}

/* ---------------- decks ---------------- */

type Entry struct {
	Qty  int
	Card *Card
	Cmdr bool
}
type Deck struct {
	Name    string
	Path    string
	Entries []Entry
	Missing []string
	MissQty []int
	Cmd     []string
	Tokens  []string
	Total   int
}

var (
	lineRe = regexp.MustCompile(`(?i)^(\d+)\s*x?\s+(.+)$`)
	headRe = regexp.MustCompile(`(?i)^(commander|comandante|commanders|deck|main|mainboard|maindeck|sideboard|side|maybeboard|considering|companion|tokens?|lands?|terrenos?|creatures?|criaturas?|artifacts?|artefatos?|enchantments?|encantamentos?|instants?|sorcery|sorceries|feiti[cç]os?|planeswalkers?|battles?|spells?)\b[\s:()\d]*$`)
	skipRe = regexp.MustCompile(`^(sideboard|side|maybeboard|considering|tokens?)`)
	cmdRe  = regexp.MustCompile(`^(commander|comandante)`)
	tagRe  = regexp.MustCompile(`\*[^*]*\*`)
	setRe  = regexp.MustCompile(`\s+\([A-Za-z0-9]{2,6}\)(\s+\S+)?\s*$`)
	hashRe = regexp.MustCompile(`\s+#.*$`)
	brkRe  = regexp.MustCompile(`\[[^\]]*\]|\^[^^]*\^`)
	cmdTag = regexp.MustCompile(`(?i)\*CMDR\*|\[[^\]]*commander`)
	xEndRe = regexp.MustCompile(`(?i)^(.+?)\s+x\s*(\d+)$`)
	sbRe   = regexp.MustCompile(`(?i)^SB:\s*`)
	slRe   = regexp.MustCompile(`\s*/{1,2}\s*`)
)

func parseDeck(name, text string) *Deck {
	d := &Deck{Name: name}
	idx := map[string]int{}
	sec := ""
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		l := strings.TrimSpace(strings.TrimPrefix(raw, "\xef\xbb\xbf"))
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#") {
			sec = strings.ToLower(strings.TrimSpace(strings.TrimLeft(l, "/#")))
			continue
		}
		if sbRe.MatchString(l) { // "SB: 1 Carta" (MTGO) é reserva: fica de fora
			continue
		}
		l = strings.ReplaceAll(l, "\t", " ")
		qty, nm := 1, l
		if m := lineRe.FindStringSubmatch(l); m != nil {
			qty, _ = strconv.Atoi(m[1])
			nm = m[2]
		} else if headRe.MatchString(l) {
			sec = strings.ToLower(l)
			continue
		} else if m := xEndRe.FindStringSubmatch(l); m != nil { // "Sol Ring x2"
			qty, _ = strconv.Atoi(m[2])
			nm = m[1]
		}
		if skipRe.MatchString(sec) || qty <= 0 {
			continue
		}
		cmdr := cmdRe.MatchString(sec) || cmdTag.MatchString(nm)
		nm = brkRe.ReplaceAllString(nm, "") // etiquetas do Archidekt: [Ramp] ^Have^
		nm = tagRe.ReplaceAllString(nm, "")
		nm = strings.TrimSpace(nm)
		nm = setRe.ReplaceAllString(nm, "")
		nm = hashRe.ReplaceAllString(nm, "")
		nm = strings.TrimSpace(slRe.ReplaceAllString(nm, " // "))
		if nm == "" {
			continue
		}
		c := findCard(nm)
		if c == nil {
			d.Missing = append(d.Missing, nm)
			d.MissQty = append(d.MissQty, qty)
			continue
		}
		if i, ok := idx[c.ID]; ok {
			d.Entries[i].Qty += qty
			d.Entries[i].Cmdr = d.Entries[i].Cmdr || cmdr
		} else {
			idx[c.ID] = len(d.Entries)
			d.Entries = append(d.Entries, Entry{qty, c, cmdr})
		}
		d.Total += qty
	}
	for _, e := range d.Entries {
		if e.Cmdr && len(d.Cmd) < 2 {
			d.Cmd = append(d.Cmd, e.Card.ID)
		}
	}
	if len(d.Cmd) == 0 && len(d.Entries) > 0 && strings.Contains(d.Entries[0].Card.typeLine(), "Legendary") {
		d.Cmd = []string{d.Entries[0].Card.ID}
	}
	seen := map[string]bool{}
	for _, e := range d.Entries {
		for _, id := range e.Card.K {
			t := cardByID[id]
			if t == nil {
				continue
			}
			f := t.FaceAt(0)
			k := strings.Join([]string{t.N, f.T, f.P, f.O, f.X}, "|")
			if !seen[k] {
				seen[k] = true
				d.Tokens = append(d.Tokens, id)
			}
		}
	}
	return d
}

func (c *Card) typeLine() string {
	if c.T != "" {
		return c.T
	}
	return c.FaceAt(0).T
}

func (d *Deck) Legends() []*Card {
	var out []*Card
	for _, e := range d.Entries {
		if strings.Contains(e.Card.typeLine(), "Legendary") {
			out = append(out, e.Card)
		}
	}
	return out
}

// todas as cartas (e fichas) do deck que têm imagem no Scryfall
func (d *Deck) imageKeys() []string {
	var out []string
	add := func(c *Card) {
		if c == nil || c.NI != 0 || c.Custom {
			return
		}
		out = append(out, c.ID+":0")
		if len(c.F) > 1 {
			out = append(out, c.ID+":1")
		}
	}
	for _, e := range d.Entries {
		add(e.Card)
	}
	for _, id := range d.Tokens {
		add(cardByID[id])
	}
	return out
}

type Config struct {
	Cmd      map[string][]string `json:"comandantes"`
	Size     float64             `json:"tamanho"`
	Name     string              `json:"nome,omitempty"`
	Addr     string              `json:"mesa,omitempty"`
	Lang     string              `json:"idioma,omitempty"` // "pt" (padrão) ou "en"
	Mat      string              `json:"playmat,omitempty"`
	Sleeve   string              `json:"sleeve,omitempty"`
	HideLeft bool                `json:"sem_painel,omitempty"` // painel da esquerda escondido
	Field    int                 `json:"campo,omitempty"`      // 0 faixas, 1 zonas, 2 limpo
	NoSnap   bool                `json:"sem_encaixe,omitempty"`
	NoBands  bool                `json:"sem_faixas,omitempty"`
	Proxy    string              `json:"proxy,omitempty"` // estilo das cartas sem imagem na mesa ("" = playtest)
}

var cfg = Config{Cmd: map[string][]string{}, Size: 112}

func cfgPath() string { return filepath.Join(baseDir, "dados", "config.json") }
func loadCfg() {
	if b, err := os.ReadFile(cfgPath()); err == nil {
		json.Unmarshal(b, &cfg)
	}
	if cfg.Cmd == nil {
		cfg.Cmd = map[string][]string{}
	}
	if cfg.Size < 60 || cfg.Size > 220 {
		cfg.Size = 112
	}
}
func saveCfg() {
	if b, err := json.MarshalIndent(cfg, "", " "); err == nil {
		os.WriteFile(cfgPath(), b, 0o644)
	}
}

func loadDecks() []*Deck {
	var out []*Deck
	files, _ := filepath.Glob(filepath.Join(baseDir, "decks", "*.txt"))
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		d := parseDeck(strings.TrimSuffix(filepath.Base(f), ".txt"), string(b))
		d.Path = f
		if saved, ok := cfg.Cmd[d.Name]; ok {
			var cmd []string
			for _, id := range saved {
				for _, e := range d.Entries {
					if e.Card.ID == id {
						cmd = append(cmd, id)
					}
				}
			}
			d.Cmd = cmd
		}
		out = append(out, d)
	}
	return out
}

var fileSafe = regexp.MustCompile(`[^\p{L}\p{N} _\-]+`)

func saveDeckText(name, text string) (string, error) {
	name = strings.TrimSpace(fileSafe.ReplaceAllString(name, ""))
	if name == "" {
		name = "deck"
	}
	dir := filepath.Join(baseDir, "decks")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, name+".txt")
	for i := 2; ; i++ {
		if _, err := os.Stat(p); err != nil {
			break
		}
		p = filepath.Join(dir, fmt.Sprintf("%s %d.txt", name, i))
	}
	return strings.TrimSuffix(filepath.Base(p), ".txt"), os.WriteFile(p, []byte(text), 0o644)
}

/* ---------------- imagens ---------------- */

func imgPath(key string) string {
	id, f, _ := strings.Cut(key, ":")
	if f == "1" {
		id += "_b"
	}
	return filepath.Join(baseDir, "dados", "imagens", id+".jpg")
}
func imgURL(key string) string {
	id, f, _ := strings.Cut(key, ":")
	c := cardByID[id]
	if f == "0" && c != nil && c.U != "" {
		return c.U
	}
	side := "front"
	if f == "1" {
		side = "back"
	}
	return fmt.Sprintf("https://cards.scryfall.io/normal/%s/%s/%s/%s.jpg", side, id[0:1], id[1:2], id)
}
func hasImg(key string) bool {
	st, err := os.Stat(imgPath(key))
	return err == nil && st.Size() > 1000
}

type imgSlot struct {
	state int // 0 carregando, 1 pronto para virar textura, 2 textura pronta, 3 não existe
	raw   *image.RGBA
	tex   *ebiten.Image
}

var (
	imgMu    sync.Mutex
	imgCache = map[string]*imgSlot{}
)

// photo devolve a imagem real da carta se ela estiver no pendrive (carrega em segundo plano).
func photo(key string) *ebiten.Image {
	imgMu.Lock()
	defer imgMu.Unlock()
	s := imgCache[key]
	if s == nil {
		s = &imgSlot{}
		imgCache[key] = s
		go func() {
			raw := decodeRounded(imgPath(key))
			imgMu.Lock()
			if raw == nil {
				s.state = 3
			} else {
				s.raw, s.state = raw, 1
			}
			imgMu.Unlock()
		}()
		return nil
	}
	if s.state == 1 {
		s.tex = ebiten.NewImageFromImage(s.raw)
		s.raw, s.state = nil, 2
	}
	return s.tex
}
func forgetMissing() {
	imgMu.Lock()
	for k, s := range imgCache {
		if s.state == 3 {
			delete(imgCache, k)
		}
	}
	imgMu.Unlock()
}

func decodeRounded(path string) *image.RGBA {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	r := float64(w) * 0.048
	for y := 0; y < int(r)+1; y++ {
		for x := 0; x < int(r)+1; x++ {
			dx, dy := r-float64(x)-0.5, r-float64(y)-0.5
			if dx*dx+dy*dy > r*r {
				for _, p := range [][2]int{{x, y}, {w - 1 - x, y}, {x, h - 1 - y}, {w - 1 - x, h - 1 - y}} {
					o := dst.PixOffset(p[0], p[1])
					dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = 0, 0, 0, 0
				}
			}
		}
	}
	return dst
}

// baixador de imagens (para usar em casa, onde o Scryfall não é bloqueado)
type Downloader struct {
	total, done, ok, fail atomic.Int32
	running               atomic.Bool
	stop                  atomic.Bool
	msg                   atomic.Value
}

var dl Downloader

func (d *Downloader) start(keys []string) {
	if d.running.Load() {
		return
	}
	var need []string
	for _, k := range keys {
		if !hasImg(k) {
			need = append(need, k)
		}
	}
	d.total.Store(int32(len(need)))
	d.done.Store(0)
	d.ok.Store(0)
	d.fail.Store(0)
	d.stop.Store(false)
	if len(need) == 0 {
		d.msg.Store("Todas as imagens deste deck já estão no pendrive.")
		return
	}
	d.msg.Store("")
	d.running.Store(true)
	os.MkdirAll(filepath.Join(baseDir, "dados", "imagens"), 0o755)
	go func() {
		defer d.running.Store(false)
		cl := &http.Client{Timeout: 30 * time.Second}
		streak := 0
		for _, k := range need {
			if d.stop.Load() {
				break
			}
			if err := fetchImg(cl, k); err != nil {
				d.fail.Add(1)
				streak++
				if streak >= 4 && d.ok.Load() == 0 {
					d.msg.Store("Não consegui falar com o Scryfall (" + shortErr(err) + "). Nesta rede ele deve estar bloqueado; baixe as imagens em casa.")
					return
				}
			} else {
				d.ok.Add(1)
				streak = 0
			}
			d.done.Add(1)
			time.Sleep(100 * time.Millisecond)
		}
		forgetMissing()
		if f := d.fail.Load(); f > 0 {
			d.msg.Store(fmt.Sprintf("Baixei %d imagens; %d falharam. Clique de novo para tentar as que faltam.", d.ok.Load(), f))
		} else {
			d.msg.Store(fmt.Sprintf("Pronto: %d imagens novas guardadas em dados/imagens.", d.ok.Load()))
		}
	}()
}
func shortErr(err error) string {
	s := err.Error()
	if len(s) > 70 {
		s = s[:70] + "…"
	}
	return s
}
func fetchImg(cl *http.Client, key string) error {
	req, _ := http.NewRequest("GET", imgURL(key), nil)
	req.Header.Set("User-Agent", "MesaArcana/1.0")
	req.Header.Set("Accept", "image/jpeg,*/*")
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(b)); err != nil {
		return fmt.Errorf("resposta não é imagem")
	}
	tmp := imgPath(key) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, imgPath(key))
}

/* ---------------- busca e sugestões de nomes ---------------- */

var (
	allCards []*Card // cartas (sem fichas) em ordem alfabética
	allLower []string
	allSq    []string
)

func buildIndex() {
	for _, c := range cardByID {
		if c.TK == 0 {
			allCards = append(allCards, c)
		}
	}
	sort.Slice(allCards, func(a, b int) bool { return allCards[a].N < allCards[b].N })
	for _, c := range allCards {
		l := strings.ToLower(c.N)
		allLower = append(allLower, l)
		allSq = append(allSq, squash(l))
	}
}

// searchCards: nomes que começam com o texto primeiro, depois os que contêm
func searchCards(q string, limit int) []*Card {
	q = strings.ToLower(strings.TrimSpace(q))
	if len(q) < 2 {
		return nil
	}
	sq := squash(q)
	var pre, mid []*Card
	for i, l := range allLower {
		if strings.HasPrefix(l, q) || (sq != "" && strings.HasPrefix(allSq[i], sq)) {
			pre = append(pre, allCards[i])
		} else if len(mid) < limit && (strings.Contains(l, q) || (len(sq) > 2 && strings.Contains(allSq[i], sq))) {
			mid = append(mid, allCards[i])
		}
		if len(pre) >= limit {
			break
		}
	}
	out := append(pre, mid...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func lev(a, b string, maxD int) int {
	if d := len(a) - len(b); d > maxD || -d > maxD {
		return maxD + 1
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= len(b); j++ {
			c := prev[j-1]
			if a[i-1] != b[j-1] {
				c++
			}
			cur[j] = min(c, prev[j]+1, cur[j-1]+1)
			best = min(best, cur[j])
		}
		if best > maxD {
			return maxD + 1
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// suggest devolve até n cartas com nome parecido (erro de digitação ou nome incompleto)
func suggest(name string, n int) []*Card {
	sq := squash(strings.ToLower(name))
	if len(sq) < 3 {
		return nil
	}
	maxD := max(2, len(sq)/4)
	type hit struct {
		c *Card
		d int
	}
	var hits []hit
	for i, s := range allSq {
		d := lev(sq, s, maxD)
		if d > maxD && len(sq) >= 4 && strings.Contains(s, sq) {
			d = maxD // nome incompleto: "Elenda" -> "Elenda, the Dusk Rose"
		}
		if d <= maxD {
			hits = append(hits, hit{allCards[i], d})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].d < hits[b].d })
	var out []*Card
	for _, h := range hits {
		if len(out) >= n {
			break
		}
		out = append(out, h.c)
	}
	return out
}
