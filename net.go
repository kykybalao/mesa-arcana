package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Multijogador em rede local. Cada jogador manda para a mesa a parte pública do próprio estado
// (campo, cemitério, exílio, comando, vida, fase) e o anfitrião repassa para os outros.
// Mão e grimório vão só como contagem; cartas viradas para baixo vão sem o nome.

const netPort = "7777"

type PubInst struct {
	ID       int            `json:"id"`
	Cid      string         `json:"c,omitempty"`
	Tapped   bool           `json:"t,omitempty"`
	Down     bool           `json:"d,omitempty"`
	Token    bool           `json:"k,omitempty"`
	Cmdr     bool           `json:"m,omitempty"`
	Face     int            `json:"f,omitempty"`
	Counters map[string]int `json:"n,omitempty"`
	X        float64        `json:"x,omitempty"`
	Y        float64        `json:"y,omitempty"`
}
type PubState struct {
	InGame  bool             `json:"g"`
	Deck    string           `json:"deck,omitempty"`
	Life    int              `json:"life"`
	Turn    int              `json:"turn"`
	Step    int              `json:"step"`
	Phase   string           `json:"phase"`
	Hand    int              `json:"hand"`
	Library int              `json:"lib"`
	BF      []PubInst        `json:"bf"`
	GY      []PubInst        `json:"gy"`
	EX      []PubInst        `json:"ex"`
	CMD     []PubInst        `json:"cmd"`
	Tax     map[string]int   `json:"tax,omitempty"`
	BW      float64          `json:"bw"`
	BH      float64          `json:"bh"`
	CW      float64          `json:"cw"`
	Customs map[string]*Card `json:"customs,omitempty"`
	Shown   []PubInst        `json:"shown,omitempty"` // mão revelada para a mesa
	Arrows  []Arrow          `json:"setas,omitempty"`
}
type Msg struct {
	T     string    `json:"t"` // hello, state, log, bye, full
	From  int       `json:"from"`
	Name  string    `json:"name,omitempty"`
	State *PubState `json:"state,omitempty"`
	Text  string    `json:"text,omitempty"`
}
type Peer struct {
	ID    int
	Name  string
	State *PubState
	Log   []string
}
type netEvent struct {
	from int
	name string
	text string
	chat bool
}

type Net struct {
	mu       sync.Mutex
	mode     string // "", "host", "client"
	name     string
	status   string
	ln       net.Listener
	conns    map[int]net.Conn // anfitrião: jogadores conectados
	conn     net.Conn         // cliente: ligação com o anfitrião
	nextID   int
	peers    map[int]*Peer
	events   []netEvent
	lastSent []byte
	tick     int
}

var nw = &Net{peers: map[int]*Peer{}, conns: map[int]net.Conn{}}

func (n *Net) active() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.mode != ""
}
func (n *Net) info() (mode, status string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.mode, n.status
}
func (n *Net) peerList() []*Peer {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]*Peer, 0, len(n.peers))
	for _, p := range n.peers {
		cp := *p
		out = append(out, &cp)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}
func (n *Net) peer(id int) *Peer {
	n.mu.Lock()
	defer n.mu.Unlock()
	if p := n.peers[id]; p != nil {
		cp := *p
		return &cp
	}
	return nil
}
func (n *Net) takeEvents() []netEvent {
	n.mu.Lock()
	defer n.mu.Unlock()
	ev := n.events
	n.events = nil
	return ev
}

func localIPs() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			out = append(out, ipn.IP.String())
		}
	}
	return out
}

// Toda escrita passa por uma fila única: as mensagens nunca se misturam nem chegam fora de ordem.
type outMsg struct {
	c net.Conn
	b []byte
}

var sendq = make(chan outMsg, 512)

func init() {
	go func() {
		for o := range sendq {
			o.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := o.c.Write(o.b); err != nil {
				o.c.Close()
			}
		}
	}()
}

const hostID = 1000 // número do anfitrião na mesa (0 fica reservado para "minha mesa")

func writeMsg(c net.Conn, m *Msg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	select {
	case sendq <- outMsg{c, append(b, '\n')}:
	default: // fila cheia: descarta (o próximo estado corrige)
	}
	return nil
}

func (n *Net) host(name string) {
	n.stop()
	ln, err := net.Listen("tcp", ":"+netPort)
	n.mu.Lock()
	defer n.mu.Unlock()
	if err != nil {
		n.status = "Não consegui criar a mesa: " + shortErr(err)
		return
	}
	n.mode, n.name, n.ln, n.nextID, n.lastSent = "host", name, ln, 1, nil
	ips := localIPs()
	if len(ips) == 0 {
		n.status = "Mesa criada, mas este computador não está em nenhuma rede."
	} else {
		n.status = "Mesa criada. Os outros entram com o endereço: " + strings.Join(ips, "  ou  ")
	}
	go n.acceptLoop(ln)
}
func (n *Net) acceptLoop(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		n.mu.Lock()
		if len(n.conns) >= 5 {
			n.mu.Unlock()
			c.Close()
			continue
		}
		id := n.nextID
		n.nextID++
		n.conns[id] = c
		n.lastSent = nil // força reenviar o estado para quem acabou de entrar
		// quem entra recebe o que já se sabe dos outros
		var known []Msg
		for _, p := range n.peers {
			known = append(known, Msg{T: "state", From: p.ID, Name: p.Name, State: p.State})
		}
		n.mu.Unlock()
		for k := range known {
			writeMsg(c, &known[k])
		}
		go n.readLoop(c, id)
	}
}

func (n *Net) join(name, addr string) {
	n.stop()
	addr = strings.TrimSpace(addr)
	if addr == "" {
		n.setStatus("Digite o endereço que aparece na tela de quem criou a mesa.")
		return
	}
	if !strings.Contains(addr, ":") {
		addr += ":" + netPort
	}
	n.setStatus("Conectando em " + addr + "…")
	go func() {
		c, err := net.DialTimeout("tcp", addr, 6*time.Second)
		if err != nil {
			n.setStatus("Não consegui entrar na mesa: " + shortErr(err))
			return
		}
		n.mu.Lock()
		n.mode, n.name, n.conn, n.lastSent = "client", name, c, nil
		n.status = "Conectado à mesa em " + addr
		n.mu.Unlock()
		writeMsg(c, &Msg{T: "hello", Name: name})
		n.readLoop(c, 0)
	}()
}
func (n *Net) setStatus(s string) {
	n.mu.Lock()
	n.status = s
	n.mu.Unlock()
}

// readLoop lê as mensagens de uma ligação. No anfitrião, id é o jogador daquela ligação.
func (n *Net) readLoop(c net.Conn, id int) {
	r := bufio.NewReaderSize(c, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil || len(line) > 4<<20 {
			break
		}
		var m Msg
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		n.mu.Lock()
		isHost := n.mode == "host"
		if isHost {
			m.From = id // o anfitrião decide de quem é a mensagem
		}
		n.handle(&m)
		var others []net.Conn
		if isHost {
			for oid, oc := range n.conns {
				if oid != id {
					others = append(others, oc)
				}
			}
		}
		n.mu.Unlock()
		for _, oc := range others {
			writeMsg(oc, &m)
		}
	}
	c.Close()
	n.mu.Lock()
	if n.mode == "host" {
		delete(n.conns, id)
		name := ""
		if p := n.peers[id]; p != nil {
			name = p.Name
		}
		delete(n.peers, id)
		bye := Msg{T: "bye", From: id, Name: name}
		var others []net.Conn
		for _, oc := range n.conns {
			others = append(others, oc)
		}
		if name != "" {
			n.events = append(n.events, netEvent{from: id, name: name, text: "saiu da mesa"})
		}
		n.mu.Unlock()
		for _, oc := range others {
			writeMsg(oc, &bye)
		}
		return
	}
	if n.mode == "client" && n.conn == c {
		n.mode, n.conn = "", nil
		n.peers = map[int]*Peer{}
		n.status = "A ligação com a mesa caiu."
	}
	n.mu.Unlock()
}

// handle é chamado com o mutex preso
func (n *Net) handle(m *Msg) {
	p := n.peers[m.From]
	if p == nil && m.T != "bye" {
		p = &Peer{ID: m.From, Name: m.Name}
		n.peers[m.From] = p
		if m.Name != "" {
			n.events = append(n.events, netEvent{from: m.From, name: m.Name, text: "entrou na mesa"})
		}
	}
	switch m.T {
	case "hello":
		p.Name = m.Name
	case "state":
		if m.Name != "" {
			p.Name = m.Name
		}
		p.State = m.State
	case "log":
		p.Log = append(p.Log, m.Text)
		if len(p.Log) > 60 {
			p.Log = p.Log[1:]
		}
		n.events = append(n.events, netEvent{from: m.From, name: p.Name, text: m.Text})
	case "chat":
		if t := strings.TrimSpace(m.Text); t != "" {
			if len(t) > 400 {
				t = t[:400]
			}
			n.events = append(n.events, netEvent{from: m.From, name: p.Name, text: t, chat: true})
		}
	case "bye":
		if p != nil {
			n.events = append(n.events, netEvent{from: m.From, name: p.Name, text: "saiu da mesa"})
			delete(n.peers, m.From)
		}
	}
}

func (n *Net) send(m *Msg) {
	n.mu.Lock()
	var cs []net.Conn
	if n.mode == "host" {
		for _, c := range n.conns {
			cs = append(cs, c)
		}
	} else if n.mode == "client" && n.conn != nil {
		cs = append(cs, n.conn)
	}
	m.Name = n.name
	if n.mode == "host" {
		m.From = hostID
	}
	n.mu.Unlock()
	for _, c := range cs {
		writeMsg(c, m)
	}
}

func (n *Net) stop() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ln != nil {
		n.ln.Close()
		n.ln = nil
	}
	for id, c := range n.conns {
		c.Close()
		delete(n.conns, id)
	}
	if n.conn != nil {
		n.conn.Close()
		n.conn = nil
	}
	n.mode, n.status, n.lastSent = "", "", nil
	n.peers = map[int]*Peer{}
}

/* ---------- o que este jogador mostra para a mesa ---------- */

func (g *Game) pubState() *PubState {
	s := g.S
	if s == nil || g.screen != "game" {
		return &PubState{}
	}
	p := &PubState{InGame: true, Life: s.Life, Turn: s.Turn, Step: s.Step, Phase: s.Phase, Hand: len(s.Z["hand"]), Library: len(s.Z["library"]),
		Tax: s.Tax, BW: float64(g.bf.Dx()), BH: float64(g.bf.Dy()), CW: g.cw}
	if g.deck != nil {
		p.Deck = g.deck.Name
	}
	conv := func(zone string, pos bool) []PubInst {
		out := []PubInst{}
		for _, id := range s.Z[zone] {
			i := s.Cards[id]
			pi := PubInst{ID: i.ID, Cid: i.Cid, Tapped: i.Tapped, Down: i.Down, Token: i.Token, Cmdr: i.Cmdr, Face: i.Face}
			if len(i.Counters) > 0 {
				pi.Counters = i.Counters
			}
			if pos {
				pi.X, pi.Y = i.X, i.Y
			}
			if i.Down { // carta virada para baixo: os outros não ficam sabendo qual é
				pi.Cid, pi.Face = "", 0
			} else if c := g.customs[i.Cid]; c != nil || (cardByID[i.Cid] != nil && cardByID[i.Cid].Mine) {
				if c == nil {
					c = cardByID[i.Cid]
				}
				if p.Customs == nil {
					p.Customs = map[string]*Card{}
				}
				p.Customs[i.Cid] = c
			}
			out = append(out, pi)
		}
		return out
	}
	p.BF, p.GY, p.EX, p.CMD = conv("battlefield", true), conv("graveyard", false), conv("exile", false), conv("command", false)
	if g.showHand {
		p.Shown = conv("hand", false)
	}
	p.Arrows = g.arrows
	return p
}

// netSync manda o estado público quando ele muda e traz para o registro o que os outros fizeram
func (g *Game) netSync() {
	if !nw.active() {
		return
	}
	nw.tick++
	if nw.tick%8 == 0 {
		st := g.pubState()
		b, _ := json.Marshal(st)
		nw.mu.Lock()
		changed := !bytes.Equal(b, nw.lastSent)
		if changed {
			nw.lastSent = b
		}
		nw.mu.Unlock()
		if changed {
			nw.send(&Msg{T: "state", State: st})
		}
	}
	for _, ev := range nw.takeEvents() {
		if ev.chat {
			g.chatLine(ev.name, ev.text)
			continue
		}
		line := "[" + ev.name + "] " + ev.text
		if g.S != nil {
			g.S.Log = append(g.S.Log, line)
			if strings.HasPrefix(ev.text, "fase: ") || strings.HasPrefix(ev.text, "— turno") || strings.HasSuffix(ev.text, " da mesa") || strings.Contains(ev.text, "revelou a mão") {
				g.announce(ev.name + ": " + strings.TrimPrefix(ev.text, "fase: "))
			}
		} else {
			g.msg = line
		}
	}
	if g.viewPeer != 0 && nw.peer(g.viewPeer) == nil {
		g.viewPeer = 0
	}
}

// chatLine guarda uma mensagem do chat e mostra no registro
func (g *Game) chatLine(who, text string) {
	line := who + " diz: " + text
	g.chat = append(g.chat, line)
	if len(g.chat) > 200 {
		g.chat = g.chat[1:]
	}
	if g.S != nil {
		g.S.Log = append(g.S.Log, "» "+line)
	}
	if who != "Você" {
		g.announce(fit(line, fBold, 20, 900))
	}
}

func (g *Game) sendChat() {
	t := strings.TrimSpace(g.fields["chat"])
	g.fields["chat"] = ""
	if t == "" || !nw.active() {
		return
	}
	nw.send(&Msg{T: "chat", Text: t})
	g.chatLine("Você", t)
}

// chatBox desenha o campo de mensagem; Enter envia
func (g *Game) chatBox(x, y, w float64) {
	g.field("chat", x, y, w, "Mensagem para a mesa (Enter envia)")
	if g.focus == "chat" && g.modal == nil {
		g.onEnter = g.sendChat
	}
}

func (g *Game) toggleReveal() {
	g.showHand = !g.showHand
	if g.showHand {
		g.logP("revelou a mão para a mesa", "revelou a mão para a mesa")
	} else {
		g.logP("escondeu a mão de novo", "escondeu a mão de novo")
	}
}

// pubLog manda para a mesa uma linha do registro (já sem informação secreta)
func (g *Game) pubLog(text string) {
	if nw.active() {
		nw.send(&Msg{T: "log", Text: text})
	}
}

/* ---------- telas ---------- */

func (g *Game) netModal() {
	if g.fields["netname"] == "" {
		g.fields["netname"] = cfg.Name
	}
	if g.fields["netaddr"] == "" {
		g.fields["netaddr"] = cfg.Addr
	}
	save := func() string {
		name := strings.TrimSpace(g.fields["netname"])
		if name == "" {
			name = "Jogador"
		}
		cfg.Name, cfg.Addr = name, strings.TrimSpace(g.fields["netaddr"])
		saveCfg()
		return name
	}
	g.openModal(&Modal{title: "Multijogador em rede local", w: 640, h: 470, draw: func(x, y, w, h float64) {
		mode, status := nw.info()
		txt(g.dst, "Seu nome na mesa", x, y+6, fReg, 13.5, cMut)
		g.field("netname", x+150, y, 240, "Seu nome")
		y += 46
		if mode == "" {
			txt(g.dst, "Criar uma mesa", x, y, fBold, 14.5, cAcc)
			ls := wrap("Um jogador cria a mesa e passa o endereço para os outros. Todos precisam estar na mesma rede (o mesmo Wi-Fi ou o roteador de um celular). Não precisa de internet.", fReg, 13, w)
			txt(g.dst, strings.Join(ls, "\n"), x, y+24, fReg, 13, cMut)
			y += 30 + float64(len(ls))*16.5
			g.button(x, y, 0, 34, "Criar mesa neste computador", 1, func() { nw.host(save()) })
			y += 56
			txt(g.dst, "Entrar em uma mesa", x, y, fBold, 14.5, cAcc)
			g.field("netaddr", x, y+26, 300, "Endereço do anfitrião (ex.: 192.168.0.12)")
			g.button(x+310, y+26, 0, 30, "Entrar", 1, func() { nw.join(save(), g.fields["netaddr"]) })
			y += 70
		} else {
			lab := "Você criou esta mesa."
			if mode == "client" {
				lab = "Você entrou em uma mesa."
			}
			txt(g.dst, lab, x, y, fBold, 14.5, cAcc)
			y += 26
			ps := nw.peerList()
			if len(ps) == 0 {
				txt(g.dst, "Ninguém mais na mesa ainda.", x, y, fReg, 13.5, cMut)
				y += 22
			}
			for _, p := range ps {
				st := "escolhendo o deck"
				if p.State != nil && p.State.InGame {
					st = fmt.Sprintf("jogando com %s · vida %d", p.State.Deck, p.State.Life)
				}
				txt(g.dst, fit("• "+p.Name+" — "+st, fReg, 13.5, w), x, y, fReg, 13.5, cTxt)
				y += 22
			}
			y += 10
			g.button(x, y, 0, 32, "Sair da mesa", 2, func() { nw.stop() })
			y += 46
			ls := wrap("Agora cada um escolhe o seu deck e clica em Jogar. Na mesa, os nomes dos outros jogadores aparecem em cima do campo: clique em um nome para ver a mesa dele.", fReg, 13, w)
			txt(g.dst, strings.Join(ls, "\n"), x, y, fReg, 13, cMut)
			y += float64(len(ls))*16.5 + 8
		}
		if status != "" {
			ls := wrap(status, fBold, 13.5, w)
			txt(g.dst, strings.Join(ls, "\n"), x, y, fBold, 13.5, cAcc2)
			y += float64(len(ls))*17 + 6
		}
		if mode == "host" {
			ls := wrap("Se o Windows perguntar sobre o firewall, clique em “Permitir acesso”. Sem isso os outros não conseguem entrar.", fReg, 12.5, w)
			txt(g.dst, strings.Join(ls, "\n"), x, y, fReg, 12.5, cMut)
		}
	}})
}

// drawPlayers desenha a faixa com os jogadores da mesa; clicar em um nome mostra a mesa dele
func (g *Game) drawPlayers(x, y, w float64) {
	rect(g.dst, x, y, w, 28, cPanel)
	rect(g.dst, x, y+27, w, 1, cLine)
	px := x + 8
	chipName := ""
	chip := func(label string, on bool, fn func()) {
		cw := tw(label, fReg, 12.5) + 20
		g.au.chips[chipName] = [2]float64{px + cw/2, y + 24}
		if on {
			rrect(g.dst, px, y+3, cw, 21, 10, cAcc2)
			txtC(g.dst, label, px+cw/2, y+6, fBold, 12.5, color.RGBA{0x08, 0x1b, 0x16, 0xff})
		} else {
			rrect(g.dst, px, y+3, cw, 21, 10, cPanel2)
			txtC(g.dst, label, px+cw/2, y+6, fReg, 12.5, cTxt)
		}
		g.add(region{x: px, y: y + 3, w: cw, h: 21, click: fn})
		px += cw + 6
	}
	chip("Minha mesa", g.viewPeer == 0, func() { g.viewPeer = 0 })
	for _, p := range nw.peerList() {
		lab := p.Name + " (sem deck)"
		if p.State != nil && p.State.InGame {
			lab = fmt.Sprintf("%s · vida %d · mão %d · turno %d", p.Name, p.State.Life, p.State.Hand, p.State.Turn)
		}
		id := p.ID
		chipName = p.Name
		chip(lab, g.viewPeer == id, func() { g.viewPeer = id })
	}
}

func (g *Game) pubCard(p *Peer, i PubInst) *Card {
	if c := cardByID[i.Cid]; c != nil {
		return c
	}
	if p.State.Customs != nil {
		if c := p.State.Customs[i.Cid]; c != nil {
			return c
		}
	}
	return nil
}

// drawPubInst desenha uma carta de outro jogador (só leitura)
func (g *Game) drawPubInst(p *Peer, pi PubInst, x, y, w float64, flat bool) {
	c := g.pubCard(p, pi)
	i := &Inst{ID: 0, Cid: pi.Cid, Zone: "battlefield", Tapped: pi.Tapped && !flat, Down: pi.Down || c == nil, Token: pi.Token, Face: pi.Face, Counters: pi.Counters}
	if c != nil && g.customs[pi.Cid] == nil && cardByID[pi.Cid] == nil {
		g.customs[pi.Cid] = c // ficha personalizada do outro jogador
	}
	g.drawInst(i, x, y, w, false, false, 1)
	h := w * ratio
	r := region{x: x, y: y, w: w, h: h, pv: c, pvFace: pi.Face, pvHidden: c == nil || pi.Down}
	if c == nil {
		r.pv = &Card{Face: Face{N: "Carta oculta"}}
	}
	if i.Tapped {
		r.x, r.y, r.w, r.h = x+w/2-h/2, y+h/2-w/2, h, w
	}
	if !flat { // carta no campo do outro jogador: pode ser alvo de seta
		g.au.pub[pi.ID] = [4]float64{r.x, r.y, r.w, r.h}
		r.click = func() { g.pubClick(p, pi, false) }
		r.rclick = func() { g.pubClick(p, pi, true) }
	}
	g.add(r)
	if pi.Cmdr && p.State.Tax[pi.Cid] > 0 {
		s := fmt.Sprintf("imposto +%d", p.State.Tax[pi.Cid]*2)
		bw := tw(s, fBold, 11) + 10
		rrect(g.dst, x+3, y+3, bw, 17, 4, color.RGBA{0, 0, 0, 0xe6})
		txt(g.dst, s, x+8, y+4.5, fBold, 11, cAcc)
	}
}

// drawPeerBoard mostra a mesa de outro jogador no lugar da sua
func (g *Game) drawPeerBoard(p *Peer, bx, by, bw, bh, top, handH, H float64) {
	st := p.State
	rect(g.dst, bx, by, bw, bh, color.RGBA{0x1c, 0x2f, 0x3d, 0xff})
	rect(g.dst, bx, top, bw, 30, cPanel)
	rect(g.dst, bx, top+29, bw, 1, cLine)
	g.add(region{x: bx, y: by, w: bw, h: bh + handH, block: true, click: func() {}, rclick: func() {}})
	if st == nil || !st.InGame {
		txtC(g.dst, p.Name+" ainda não começou a partida", bx+bw/2, by+bh/2-10, fBold, 18, color.RGBA{0xff, 0xff, 0xff, 0x55})
	} else {
		head := fmt.Sprintf("Mesa de %s  ·  %s  ·  vida %d  ·  turno %d", p.Name, st.Deck, st.Life, st.Turn)
		if st.Phase == "play" && st.Step >= 0 && st.Step < len(stepNames) {
			head += "  ·  " + stepNames[st.Step]
		} else {
			head += "  ·  escolhendo a mão"
		}
		txtC(g.dst, fit(head, fBold, 13.5, bw-20), bx+bw/2, top+7, fBold, 13.5, cAcc2)
		f := 1.0
		if st.BW > 0 && st.BH > 0 {
			f = min(bw/st.BW, bh/st.BH)
		}
		old := g.dst
		g.dst = clip(old, bx, by, bw, bh)
		for _, pi := range st.BF {
			g.drawPubInst(p, pi, bx+pi.X*f, by+pi.Y*f, st.CW*f, false)
		}
		g.dst = old
		if len(st.BF) == 0 {
			txtC(g.dst, "campo vazio", bx+bw/2, by+bh/2-10, fBold, 18, color.RGBA{0xff, 0xff, 0xff, 0x33})
		}
	}
	// mão do outro jogador: só o verso das cartas
	hy := H - handH
	rect(g.dst, bx, hy, bw, handH, cPanel)
	rect(g.dst, bx, hy, bw, 1, cLine)
	if st != nil && st.InGame {
		n := st.Hand
		step := g.cw*0.5 + 4
		if n > 1 {
			step = min(step, (bw-20-g.cw)/float64(n-1))
		}
		if len(st.Shown) > 0 { // mão revelada: mostra as cartas
			n = len(st.Shown)
			step = g.cw + 6
			if n > 1 {
				step = min(step, (bw-20-g.cw)/float64(n-1))
			}
			for k, pi := range st.Shown {
				g.drawPubInst(p, pi, bx+10+float64(k)*step, hy+10, g.cw, true)
			}
		} else {
			for k := 0; k < n; k++ {
				drawTex(g.dst, g.backTex(), bx+10+float64(k)*step, hy+10, g.cw, false, 1)
			}
		}
		lab := fmt.Sprintf("%d carta(s) na mão de %s", n, p.Name)
		if len(st.Shown) > 0 {
			lab = fmt.Sprintf("%s está revelando a mão (%d)", p.Name, n)
		}
		txt(g.dst, lab, bx+bw-16-tw(lab, fReg, 13), hy+12, fReg, 13, cMut)
	}
}

// drawPeerPiles mostra comando, grimório, cemitério e exílio do outro jogador
func (g *Game) drawPeerPiles(p *Peer, rx, top, rightW, pileW, H float64) {
	rect(g.dst, rx, top, rightW, H-top, cPanel)
	rect(g.dst, rx, top, 1, H-top, cLine)
	st := p.State
	slotH := (H - top - 10) / 4
	for k, z := range []string{"COMANDO", "GRIMÓRIO", "CEMITÉRIO", "EXÍLIO"} {
		y := top + 6 + float64(k)*slotH
		txt(g.dst, z, rx+10, y+3, fBold, 10.5, cMut)
		cy := y + 20
		rstroke(g.dst, rx+14, cy, pileW, pileW*ratio, pileW*0.05, 1, cLine)
		if st == nil || !st.InGame {
			continue
		}
		var list []PubInst
		switch k {
		case 0:
			list = st.CMD
		case 2:
			list = st.GY
		case 3:
			list = st.EX
		}
		if k == 1 {
			c := fmt.Sprint(st.Library)
			txt(g.dst, c, rx+rightW-10-tw(c, fBold, 11.5), y+2, fBold, 11.5, cTxt)
			if st.Library > 0 {
				drawTex(g.dst, g.backTex(), rx+14, cy, pileW, false, 1)
			}
			continue
		}
		if k > 0 {
			c := fmt.Sprint(len(list))
			txt(g.dst, c, rx+rightW-10-tw(c, fBold, 11.5), y+2, fBold, 11.5, cTxt)
		}
		show := list
		if k > 0 && len(list) > 0 {
			show = list[len(list)-1:]
		}
		for n, pi := range show {
			g.drawPubInst(p, pi, rx+14, cy+float64(n)*24, pileW, true)
		}
		if k > 0 && len(list) > 0 {
			title, l := z, list
			g.add(region{x: rx + 4, y: y, w: rightW - 8, h: slotH - 4, click: func() { g.peerZoneModal(p, title, l) }})
		}
	}
}

func (g *Game) peerZoneModal(p *Peer, title string, list []PubInst) {
	m := &Modal{title: strings.Title(strings.ToLower(title)) + " de " + p.Name, w: 1100, h: 700}
	m.draw = func(x, y, w, h float64) {
		g.grid(m, x, y, w-8, h-44, len(list), 0, func(k int, cx, cy, cw float64) []cellBtn {
			g.drawPubInst(p, list[len(list)-1-k], cx, cy, cw, true)
			return nil
		})
		g.button(x+w-90, y+h-32, 90, 30, "Fechar", 1, g.closeModal)
	}
	g.openModal(m)
}
