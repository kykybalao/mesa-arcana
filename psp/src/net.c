/* Rede entre PSPs (Wi-Fi ad hoc). Cada um manda a sua mesa pública algumas vezes por segundo para os
 * outros: vida, turno, fase, contagens e as cartas do campo (pelo nome). Quem recebe procura a carta pelo
 * nome nos decks que tem no Memory Stick para mostrar a imagem; se não achar, mostra um papel com o nome.
 * No PC (teste) o transporte é UDP local. */
#include "base.h"
#include <dirent.h>
#include <stdarg.h>

#ifdef __PSP__
#include <pspkernel.h>
#include <pspnet.h>
#include <pspnet_adhoc.h>
#include <pspnet_adhocctl.h>
#include <psputility.h>
#include <pspwlan.h>
#else
#include <arpa/inet.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>
#endif

#define NPEER 4
#define MAXPC 120
#define PORT 7790
#define PER_PART 22

typedef struct {
    char name[48], pt[12];
    unsigned char flags, row, cnt;
    signed char plus;
    short ref; /* carta conhecida (índice em refs) ou -1 */
} PCard;

typedef struct {
    unsigned id, last;
    char name[24];
    int life, poison, turn, step, phase, hand, lib, gy, ex, ncard;
    PCard c[MAXPC];
} Peer;

static Peer peers[NPEER];
static int state;  /* 0 desligado, 1 conectando, 2 na mesa */
static int room = 1, viewing; /* viewing: 0 = minha mesa, k = peers[k-1] */
static unsigned myId;
static char myName[24];
static unsigned short seq;
static unsigned connStart;
static char logLines[16][120];
static int nlog;

static void addLog(const char *s) {
    if (nlog == 16) {
        memmove(logLines, logLines + 1, sizeof logLines[0] * 15);
        nlog--;
    }
    snprintf(logLines[nlog++], 120, "%s", s);
}

/* ---------------- cartas conhecidas (de todos os decks do Memory Stick) ---------------- */

typedef struct {
    unsigned h;
    char *name, *cost;
    short deck, idx, face;
} Ref;
static Ref *refs;
static int nrefs;
static char deckPaths[40][128];
static int ndp;

static unsigned hname(const char *s) {
    unsigned h = 2166136261u;
    while (*s) h = (h ^ (unsigned char)*s++) * 16777619u;
    return h;
}

static void buildRefs(void) {
    if (refs) return;
    refs = calloc(4000, sizeof(Ref));
    DIR *d = opendir("decks");
    if (!d) return;
    struct dirent *e;
    char line[8192];
    while ((e = readdir(d)) && ndp < 40) {
        if (e->d_name[0] == '.') continue;
        char p[300];
        snprintf(p, sizeof p, "decks/%s/deck.txt", e->d_name);
        FILE *f = fopen(p, "r");
        if (!f) continue;
        snprintf(deckPaths[ndp], 128, "decks/%s", e->d_name);
        int idx = -1, face = 0;
        while (fgets(line, sizeof line, f) && nrefs < 4000) {
            if (!strncmp(line, "CARTA\t", 6)) {
                idx = atoi(line + 6);
                face = 0;
            } else if (!strncmp(line, "FACE\t", 5) && idx >= 0) {
                char *n = line + 5, *t = strchr(n, '\t'), *c = "";
                if (t) {
                    *t = 0;
                    c = t + 1;
                    char *e = strchr(c, '\t');
                    if (e) *e = 0;
                }
                refs[nrefs] = (Ref){hname(n), strdup(n), strdup(c), (short)ndp, (short)idx, (short)face};
                nrefs++;
                face++;
            }
        }
        fclose(f);
        ndp++;
    }
    closedir(d);
}
static int findRef(const char *name) {
    if (!*name) return -1;
    unsigned h = hname(name);
    for (int k = 0; k < nrefs; k++)
        if (refs[k].h == h && !strcmp(refs[k].name, name)) return k;
    return -1;
}

/* texturas pequenas das cartas dos outros */
typedef struct {
    int ref, tried; /* tried: já tentou abrir (mesmo sem imagem, não tenta de novo a cada quadro) */
    SDL_Texture *t;
    unsigned used;
} PTex;
static PTex ptex[64];
static SDL_Texture *refTex(int ref) {
    PTex *old = &ptex[0];
    for (int k = 0; k < 64; k++) {
        if (ptex[k].tried && ptex[k].ref == ref) {
            ptex[k].used = frameN;
            return ptex[k].t;
        }
        if (!ptex[k].tried) {
            old = &ptex[k];
            break;
        }
        if (ptex[k].used < old->used) old = &ptex[k];
    }
    if (old->t) SDL_DestroyTexture(old->t);
    char p[300];
    snprintf(p, sizeof p, "%s/img/%d_%d.jpg", deckPaths[refs[ref].deck], refs[ref].idx, refs[ref].face);
    old->t = IMG_LoadTexture(R, p), nOpen++;
    old->tried = 1;
    old->ref = ref;
    old->used = frameN;
    return old->t;
}

/* ---------------- transporte ---------------- */

#ifdef __PSP__
static int pdp = -1;
static unsigned char myMac[6];
static int modsLoaded, netInit;

static int trOpen(void) {
    if (sceWlanGetSwitchState() == 0) {
        announce("Ligue o botão do Wi-Fi do PSP");
        return -1;
    }
    if (!modsLoaded) {
        if (sceUtilityLoadNetModule(PSP_NET_MODULE_COMMON) < 0 || sceUtilityLoadNetModule(PSP_NET_MODULE_ADHOC) < 0) {
            announce("Não consegui carregar a rede do PSP");
            return -1;
        }
        modsLoaded = 1;
    }
    if (!netInit) {
        if (sceNetInit(128 * 1024, 42, 4 * 1024, 42, 4 * 1024) < 0 || sceNetAdhocInit() < 0) {
            announce("Não consegui iniciar a rede");
            return -1;
        }
        struct productStruct prod;
        memset(&prod, 0, sizeof prod);
        strcpy(prod.product, "ULUS99999");
        if (sceNetAdhocctlInit(0x2000, 0x20, &prod) < 0) {
            announce("Não consegui iniciar o ad hoc");
            return -1;
        }
        netInit = 1;
    }
    char grp[9];
    snprintf(grp, sizeof grp, "MesaAr%d", room);
    if (sceNetAdhocctlConnect(grp) < 0) {
        announce("Não consegui entrar na sala");
        return -1;
    }
    return 1; /* conectando */
}
static int trReady(void) {
    int st = 0;
    if (sceNetAdhocctlGetState(&st) < 0) return -1;
    if (st != 1) return 0;
    sceNetGetLocalEtherAddr(myMac);
    pdp = sceNetAdhocPdpCreate(myMac, PORT, 0x4000, 0);
    return pdp >= 0 ? 1 : -1;
}
static unsigned char peerMacs[8][6];
static int npm;
static void refreshMacs(void) {
    static unsigned char buf[8 * sizeof(SceNetAdhocctlPeerInfo)];
    int len = sizeof buf;
    npm = 0;
    if (sceNetAdhocctlGetPeerList(&len, buf) < 0) return;
    for (SceNetAdhocctlPeerInfo *p = len > 0 ? (SceNetAdhocctlPeerInfo *)buf : NULL; p && npm < 8; p = p->next) {
        memcpy(peerMacs[npm++], p->mac, 6);
        if ((unsigned char *)p->next < buf || (unsigned char *)p->next >= buf + len) break;
    }
}
static void trSend(const void *b, int n) {
    if (pdp < 0) return;
    if (frameN % 60 == 0 || !npm) refreshMacs();
    for (int k = 0; k < npm; k++) sceNetAdhocPdpSend(pdp, peerMacs[k], PORT, (void *)b, n, 0, 1);
}
static int trRecv(void *b, int cap) {
    if (pdp < 0) return -1;
    unsigned char mac[6];
    unsigned short port;
    int len = cap;
    if (sceNetAdhocPdpRecv(pdp, mac, &port, b, &len, 0, 1) < 0) return -1;
    return len;
}
static void trClose(void) {
    if (pdp >= 0) sceNetAdhocPdpDelete(pdp, 0);
    pdp = -1;
    sceNetAdhocctlDisconnect();
}
static void defaultName(void) {
    if (sceUtilityGetSystemParamString(PSP_SYSTEMPARAM_ID_STRING_NICKNAME, myName, sizeof myName) < 0 || !*myName) strcpy(myName, "PSP");
}
#else
static int sock = -1, myPort;
static int trOpen(void) {
    sock = socket(AF_INET, SOCK_DGRAM, 0);
    struct sockaddr_in a = {0};
    a.sin_family = AF_INET;
    a.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    for (myPort = PORT + room * 10; myPort < PORT + room * 10 + 4; myPort++) {
        a.sin_port = htons(myPort);
        if (bind(sock, (struct sockaddr *)&a, sizeof a) == 0) break;
    }
    fcntl(sock, F_SETFL, O_NONBLOCK);
    return 1;
}
static int trReady(void) { return connStart + 30 < frameN ? 1 : 0; }
static void trSend(const void *b, int n) {
    struct sockaddr_in a = {0};
    a.sin_family = AF_INET;
    a.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    for (int p = PORT + room * 10; p < PORT + room * 10 + 4; p++)
        if (p != myPort) {
            a.sin_port = htons(p);
            sendto(sock, b, n, 0, (struct sockaddr *)&a, sizeof a);
        }
}
static int trRecv(void *b, int cap) {
    if (sock < 0) return -1;
    int n = recv(sock, b, cap, 0);
    return n > 0 ? n : -1;
}
static void trClose(void) {
    if (sock >= 0) close(sock);
    sock = -1;
}
static void defaultName(void) { snprintf(myName, sizeof myName, "%s", getenv("MESA_NOME") ? getenv("MESA_NOME") : "PC"); }
#endif

/* ---------------- pacotes ---------------- */

enum { P_STATE = 1, P_LOG = 2, P_BYE = 3 };

typedef struct {
    unsigned char *p, *end;
} Buf;
static void put8(Buf *b, int v) {
    if (b->p < b->end) *b->p++ = (unsigned char)v;
}
static void put16(Buf *b, int v) { put8(b, v & 255), put8(b, (v >> 8) & 255); }
static void put32(Buf *b, unsigned v) { put16(b, v & 0xffff), put16(b, v >> 16); }
static void putS(Buf *b, const char *s, int max) {
    int n = strlen(s);
    if (n > max) n = max;
    while (n > 0 && ((unsigned char)s[n] & 0xC0) == 0x80) n--; /* não corta no meio de uma letra */
    put8(b, n);
    for (int k = 0; k < n; k++) put8(b, s[k]);
}
static int get8(Buf *b) { return b->p < b->end ? *b->p++ : 0; }
static int get16(Buf *b) {
    int a = get8(b);
    return a | get8(b) << 8;
}
static unsigned get32(Buf *b) {
    unsigned a = get16(b);
    return a | (unsigned)get16(b) << 16;
}
static void getS(Buf *b, char *out, int cap) {
    int n = get8(b), k;
    for (k = 0; k < n; k++) {
        int c = get8(b);
        if (k < cap - 1) out[k] = c;
    }
    out[n < cap - 1 ? n : cap - 1] = 0;
}

static int rowOfFace(const Face *f, int down) {
    if (down) return 0;
    if (strstr(f->t, "Criatura") || strstr(f->t, "Creature")) return 0;
    if (strstr(f->t, "Terreno") || strstr(f->t, "Land")) return 2;
    return 1;
}

static void header(Buf *b, int type) {
    put8(b, 'M'), put8(b, 'A'), put8(b, 1), put8(b, type);
    put32(b, myId);
    put16(b, seq);
}

static void sendState(void) {
    int n = S.zn[Z_BF];
    if (n > MAXPC) n = MAXPC;
    int parts = n ? (n + PER_PART - 1) / PER_PART : 1;
    seq++;
    for (int part = 0; part < parts; part++) {
        unsigned char raw[1400];
        Buf b = {raw, raw + sizeof raw};
        header(&b, P_STATE);
        putS(&b, myName, 20);
        put16(&b, S.life + 1000);
        put8(&b, S.poison), put8(&b, S.step), put16(&b, S.turn), put8(&b, S.phase);
        put8(&b, S.zn[Z_HAND]), put16(&b, S.zn[Z_LIB]), put8(&b, S.zn[Z_GY]), put8(&b, S.zn[Z_EX]);
        put8(&b, n), put8(&b, part * PER_PART);
        int cnt = n - part * PER_PART;
        if (cnt > PER_PART) cnt = PER_PART;
        put8(&b, cnt);
        for (int k = part * PER_PART; k < part * PER_PART + cnt; k++) {
            Inst *i = &S.in[S.z[Z_BF][k]];
            Def *d = &defs[i->def];
            const Face *f = &d->f[i->face < d->nf ? i->face : 0];
            put8(&b, (i->tapped ? 1 : 0) | (i->down ? 2 : 0) | (i->token ? 4 : 0) | (i->cmdr ? 8 : 0));
            if (i->att > 0 && S.in[i->att].used && S.in[i->att].zone == Z_BF) { /* presa: vai na faixa da carta dona */
                Inst *h = &S.in[i->att];
                const Face *hf = &defs[h->def].f[h->face < defs[h->def].nf ? h->face : 0];
                put8(&b, rowOfFace(hf, h->down));
            } else
                put8(&b, rowOfFace(f, i->down));
            put8(&b, i->plus), put8(&b, i->cnt > 255 ? 255 : i->cnt);
            putS(&b, i->down ? "" : f->n, 46);
            char pt[16] = "";
            if (!i->down && (*f->p || *f->o)) snprintf(pt, sizeof pt, "%s/%s", f->p, f->o);
            putS(&b, pt, 10);
        }
        trSend(raw, b.p - raw);
    }
}

static Peer *peerFor(unsigned id, int make) {
    Peer *free = NULL;
    for (int k = 0; k < NPEER; k++) {
        if (peers[k].id == id) return &peers[k];
        if (!peers[k].id && !free) free = &peers[k];
    }
    if (make && free) {
        memset(free, 0, sizeof *free);
        free->id = id;
    }
    return make ? free : NULL;
}

static void handle(unsigned char *raw, int len) {
    Buf b = {raw, raw + len};
    if (get8(&b) != 'M' || get8(&b) != 'A' || get8(&b) != 1) return;
    int type = get8(&b);
    unsigned id = get32(&b);
    get16(&b);
    if (id == myId) return;
    int isNew = peerFor(id, 0) == NULL;
    Peer *p = peerFor(id, type != P_BYE);
    if (!p) return;
    p->last = frameN;
    if (type == P_BYE) {
        char m[80];
        snprintf(m, sizeof m, "%s saiu da mesa", p->name);
        announce("%s", m), addLog(m);
        if (viewing && &peers[viewing - 1] == p) viewing = 0;
        memset(p, 0, sizeof *p);
        return;
    }
    if (type == P_LOG) {
        char name[24], t[110], m[140];
        getS(&b, name, sizeof name);
        getS(&b, t, sizeof t);
        snprintf(p->name, sizeof p->name, "%s", name);
        snprintf(m, sizeof m, "%s: %s", name, t);
        addLog(m);
        if (!strncmp(t, "fase: ", 6) || !strncmp(t, "turno", 5) || !strncmp(t, "começou", 7)) announce("%s: %s", name, !strncmp(t, "fase: ", 6) ? t + 6 : t);
        return;
    }
    if (type != P_STATE) return;
    getS(&b, p->name, sizeof p->name);
    if (isNew) {
        char m[80];
        snprintf(m, sizeof m, "%s entrou na mesa", p->name);
        announce("%s", m), addLog(m);
    }
    p->life = get16(&b) - 1000;
    p->poison = get8(&b), p->step = get8(&b), p->turn = get16(&b), p->phase = get8(&b);
    p->hand = get8(&b), p->lib = get16(&b), p->gy = get8(&b), p->ex = get8(&b);
    int total = get8(&b), first = get8(&b), cnt = get8(&b);
    if (total > MAXPC) total = MAXPC;
    p->ncard = total;
    for (int k = first; k < first + cnt && k < MAXPC; k++) {
        PCard *c = &p->c[k];
        c->flags = get8(&b), c->row = get8(&b);
        c->plus = (signed char)get8(&b), c->cnt = get8(&b);
        getS(&b, c->name, sizeof c->name);
        getS(&b, c->pt, sizeof c->pt);
        c->ref = findRef(c->name);
        if (c->row > 2) c->row = 1;
    }
}

/* ---------------- interface ---------------- */

void net_init(void) {
    myId = (unsigned)rand() ^ (unsigned)SDL_GetTicks() << 12 ^ 0x5bd1e995u;
    if (!myId) myId = 1;
    defaultName();
}
int net_on(void) { return state == 2; }
int net_npeers(void) {
    int n = 0;
    for (int k = 0; k < NPEER; k++) n += peers[k].id != 0;
    return n;
}
int net_view(void) { return state == 2 ? viewing : 0; }
void net_cycle(void) {
    for (int k = 1; k <= NPEER + 1; k++) {
        int v = (viewing + k) % (NPEER + 1);
        if (v == 0 || peers[v - 1].id) {
            viewing = v;
            announce(v ? "Mesa de %s" : "Sua mesa", v ? peers[v - 1].name : "");
            return;
        }
    }
}

/* ---------------- partida contra a IA: ela ocupa o lugar de um jogador da rede ---------------- */
static int aiMode;
void net_ai_begin(const char *name) {
    if (state && !aiMode) net_shutdown();
    memset(peers, 0, sizeof peers);
    buildRefs();
    state = 2, aiMode = 1, viewing = 0, nlog = 0;
    peers[0].id = 1;
    peers[0].phase = 2;
    snprintf(peers[0].name, sizeof peers[0].name, "%s", name);
}
void net_ai_end(void) {
    if (!aiMode) return;
    aiMode = 0, state = 0, viewing = 0;
    memset(peers, 0, sizeof peers);
}
void net_ai_say(const char *s) {
    char t[120];
    snprintf(t, sizeof t, "[IA] %s", s);
    addLog(t);
    net_dirty = 1;
}
void net_ai_card(int k, const char *name, const char *pt, int tapped, int row) {
    if (k >= MAXPC) return;
    PCard *c = &peers[0].c[k];
    memset(c, 0, sizeof *c);
    snprintf(c->name, sizeof c->name, "%s", name);
    snprintf(c->pt, sizeof c->pt, "%s", pt);
    c->flags = tapped ? 1 : 0;
    c->row = row;
    c->ref = findRef(name);
}
void net_ai_stats(int life, int turn, int hand, int lib, int gy, int ex, int ncard) {
    Peer *p = &peers[0];
    p->life = life, p->turn = turn, p->hand = hand, p->lib = lib, p->gy = gy, p->ex = ex;
    p->ncard = ncard < MAXPC ? ncard : MAXPC;
    p->phase = 2, p->step = 0;
    net_dirty = 1;
}

void net_log(const char *fmt, ...) {
    if (state != 2 || aiMode) return;
    char t[110];
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(t, sizeof t, fmt, ap);
    va_end(ap);
    unsigned char raw[200];
    Buf b = {raw, raw + sizeof raw};
    header(&b, P_LOG);
    putS(&b, myName, 20);
    putS(&b, t, 100);
    trSend(raw, b.p - raw);
}

int net_dirty; /* chegou algo pela rede: a tela precisa ser redesenhada */
void net_tick(void) {
    if (state == 1) {
        int r = trReady();
        if (r > 0) {
            state = 2;
            buildRefs();
            announce("Na sala %d. Esperando os outros…", room);
        } else if (r < 0 || frameN - connStart > 60 * 25) {
            announce("Não achei a rede. Tente de novo.");
            trClose();
            state = 0;
        }
        return;
    }
    if (state != 2) return;
    if (aiMode) {
        peers[0].last = frameN;
        return;
    }
    unsigned char raw[1500];
    int n, guard = 0;
    while (guard++ < 32 && (n = trRecv(raw, sizeof raw)) > 0) handle(raw, n), net_dirty = 1;
    if (frameN % 20 == 0) sendState();
    for (int k = 0; k < NPEER; k++)
        if (peers[k].id && frameN - peers[k].last > 60 * 8) {
            char m[80];
            snprintf(m, sizeof m, "%s sumiu da rede", peers[k].name);
            announce("%s", m), addLog(m);
            if (viewing == k + 1) viewing = 0;
            memset(&peers[k], 0, sizeof peers[k]);
        }
}

void net_shutdown(void) {
    if (aiMode) {
        net_ai_end();
        return;
    }
    if (state == 2) {
        unsigned char raw[16];
        Buf b = {raw, raw + sizeof raw};
        header(&b, P_BYE);
        trSend(raw, b.p - raw);
    }
    if (state) trClose();
    state = 0;
    viewing = 0;
    memset(peers, 0, sizeof peers);
}

/* faixa com os jogadores, no alto do campo */
void net_draw_peers(int x, int y, int w, int h) {
    fill(x, y, w, h, (SDL_Color){10, 26, 32, 255});
    char b[96];
    int px = x + 4;
    for (int v = 0; v <= NPEER; v++) {
        if (v && !peers[v - 1].id) continue;
        if (v == 0)
            snprintf(b, sizeof b, "Você %d", S.life);
        else {
            Peer *p = &peers[v - 1];
            snprintf(b, sizeof b, "%s %d · T%d %s", p->name, p->life, p->turn, p->phase == 2 && p->step < 6 ? stepNames[p->step] : "mão inicial");
        }
        int tw = textW(b, 9, v == viewing) + 10;
        if (px + tw > x + w) break;
        if (v == viewing) fill(px, y + 1, tw, h - 2, cAcc);
        text(px + 5, y + 1, b, 9, v == viewing, v == viewing ? cInk : cTxt);
        px += tw + 4;
    }
    if (!net_npeers()) text(px + 4, y + 1, "esperando outros PSPs na sala…", 9, 0, cMut);
    else text(x + w - textW("R+L troca", 8, 0) - 4, y + 2, "R+L troca", 8, 0, cMut);
}

/* mesa de outro jogador, só para ver */
void net_draw_table(int x, int y, int w, int h) {
    Peer *p = &peers[viewing - 1];
    fill(x, y, w, h, (SDL_Color){28, 40, 52, 255});
    int top, rowH, cw, ch;
    bfGeom(&top, &rowH, &cw, &ch);
    for (int r = 1; r < 3; r++) fill(x + 6, top + r * rowH, w - 12, 1, (SDL_Color){232, 185, 74, 60});
    char b[120];
    snprintf(b, sizeof b, "Mesa de %s · mão %d · grimório %d · cemitério %d · exílio %d%s", p->name, p->hand, p->lib, p->gy, p->ex, p->poison ? " · veneno" : "");
    text(x + w - textW(b, 8, 0) - 4, y + h - 11, b, 8, 0, cMut);
    for (int r = 0; r < 3; r++) {
        int idx[MAXPC], n = 0, tot = 0;
        for (int k = 0; k < p->ncard; k++)
            if (p->c[k].row == r) {
                idx[n++] = k;
                tot += ((p->c[k].flags & 1) ? ch : cw) + 3;
            }
        int yy = top + r * rowH + 2;
        if (!n) continue;
        float kf = 1;
        if (tot > w - 8) kf = (float)(w - 8 - ch) / (tot - ((p->c[idx[n - 1]].flags & 1) ? ch : cw) - 3);
        float xx = x + 4;
        for (int j = 0; j < n; j++) {
            PCard *c = &p->c[idx[j]];
            int t = c->flags & 1, fw = t ? ch : cw, fh = t ? cw : ch;
            int cy = t ? yy + (ch - cw) : yy;
            SDL_Texture *tex = (c->flags & 2) || c->ref < 0 ? NULL : refTex(c->ref);
            if (c->flags & 2)
                drawBack((int)xx, cy, fw, fh);
            else if (tex) {
                SDL_Rect d = {(int)xx, cy, cw, ch};
                if (t) d.x = (int)xx + (ch - cw) / 2, d.y = cy + (cw - ch) / 2;
                SDL_RenderCopyEx(R, tex, NULL, &d, t ? 90 : 0, NULL, SDL_FLIP_NONE);
            } else {
                Face f = {c->name, "", "", "", "", "", ""};
                drawPaper(&f, (int)xx, cy, fw, fh);
                if (*c->pt) badge((int)xx + fw - textW(c->pt, 9, 1) - 7, cy + fh - 13, c->pt, (SDL_Color){0, 0, 0, 200}, cTxt);
            }
            if (!(c->flags & 2) && showBar) {
                Face nf = {c->name, c->ref >= 0 ? refs[c->ref].cost : "", "", "", "", "", ""};
                nameBar(&nf, (int)xx, cy, fw);
            }
            int by = cy + fh - 13;
            if (c->plus) {
                snprintf(b, sizeof b, "%+d", c->plus);
                badge((int)xx + 1, by, b, (SDL_Color){0, 0, 0, 220}, c->plus > 0 ? cAcc : cBad);
                by -= 13;
            }
            if (c->cnt) {
                snprintf(b, sizeof b, "●%d", c->cnt);
                badge((int)xx + 1, by, b, (SDL_Color){0, 0, 0, 220}, cAcc2);
            }
            if (isSel(K_PEER, viewing, idx[j])) {
                frame((int)xx - 2, cy - 2, fw + 4, fh + 4, cAcc);
                frame((int)xx - 3, cy - 3, fw + 6, fh + 6, cAcc);
            }
            addSel((int)xx, cy, fw, fh, K_PEER, viewing, idx[j]);
            xx += (fw + 3) * kf;
        }
    }
    if (!p->ncard) textC(x + w / 2, top + rowH, p->phase == 2 ? "campo vazio" : "escolhendo a mão inicial", 12, 1, (SDL_Color){255, 255, 255, 60});
}

/* carta grande de outro jogador: o texto vem do deck que tiver essa carta no Memory Stick */
void net_zoom(int k) {
    if (!viewing || k < 0 || k >= peers[viewing - 1].ncard) return;
    PCard *c = &peers[viewing - 1].c[k];
    if (c->flags & 2) {
        announce("Carta virada para baixo");
        return;
    }
    static char fld[7][1200];
    Face f = {fld[0], fld[1], fld[2], fld[3], fld[4], fld[5], fld[6]};
    for (int i = 0; i < 7; i++) fld[i][0] = 0;
    snprintf(fld[0], sizeof fld[0], "%s", c->name);
    char big[300] = "", extra[160] = "";
    if (c->ref >= 0) {
        Ref *r = &refs[c->ref];
        char p[300], line[8192];
        snprintf(p, sizeof p, "%s/deck.txt", deckPaths[r->deck]);
        snprintf(big, sizeof big, "%s/img/%d_%dg.jpg", deckPaths[r->deck], r->idx, r->face);
        FILE *fp = fopen(p, "r");
        int idx = -1, face = 0;
        while (fp && fgets(line, sizeof line, fp)) {
            line[strcspn(line, "\r\n")] = 0;
            if (!strncmp(line, "CARTA\t", 6))
                idx = atoi(line + 6), face = 0;
            else if (!strncmp(line, "FACE\t", 5)) {
                if (idx == r->idx && face == r->face) {
                    char *s = line + 5;
                    for (int i = 0; i < 7 && s; i++) {
                        char *t = strchr(s, '\t');
                        if (t) *t = 0;
                        char *o = fld[i];
                        for (; *s && o < fld[i] + 1199; s++) { /* \n vira quebra de linha */
                            if (s[0] == '\\' && s[1] == 'n') *o++ = '\n', s++;
                            else *o++ = *s;
                        }
                        *o = 0;
                        s = t ? t + 1 : NULL;
                    }
                    break;
                }
                face++;
            }
        }
        if (fp) fclose(fp);
    } else {
        snprintf(fld[2], sizeof fld[2], "(esta carta não está em nenhum deck do seu Memory Stick)");
        if (*c->pt) {
            char *sl = strchr(c->pt, '/');
            if (sl) {
                *sl = 0;
                snprintf(fld[4], sizeof fld[4], "%s", c->pt);
                snprintf(fld[5], sizeof fld[5], "%s", sl + 1);
                *sl = '/';
            }
        }
    }
    char tmp[60] = "";
    if (c->plus) snprintf(tmp, sizeof tmp, "Marcadores %+d/%+d: %d. ", c->plus > 0 ? 1 : -1, c->plus > 0 ? 1 : -1, abs(c->plus));
    snprintf(extra, sizeof extra, "%s%s%sMesa de %s", tmp, c->cnt ? "Tem outros marcadores. " : "", (c->flags & 1) ? "Virada. " : "", peers[viewing - 1].name);
    zoomExt(&f, big, extra);
}

/* ---------------- menu da rede ---------------- */

static int logOn;

static void nJoin(int x) {
    (void)x;
    if (state) return;
    connStart = frameN;
    int r = trOpen();
    if (r < 0) return;
    state = 1;
    announce("Procurando a sala %d…", room);
}
static void nLeave(int x) {
    (void)x;
    net_shutdown();
    announce("Saiu da rede");
}
static void nView(int v) {
    viewing = v;
    announce(v ? "Mesa de %s" : "Sua mesa", v ? peers[v - 1].name : "");
}

/* menus vêm de main.c */
void *net_open_menu(const char *title);
void net_menu_add(void *m, const char *label, void (*fn)(int), int arg, void (*adj)(int, int));
void net_menu_relabel(void *m, void (*fn)(void));
void net_menu_set(int k, const char *label);

static void adjRoom(int x, int d) {
    (void)x;
    if (state) return;
    room += d;
    if (room < 1) room = 4;
    if (room > 4) room = 1;
}
static void relabelNet(void);
static void nLog(int x) {
    (void)x;
    logOn = 1;
}

void net_menu(void) {
    void *m = net_open_menu("Jogar em rede (Wi-Fi ad hoc)");
    if (state == 0) {
        net_menu_add(m, "", NULL, 0, adjRoom);
        net_menu_add(m, "Entrar na sala", nJoin, 0, NULL);
    } else {
        net_menu_add(m, "Ver a sua mesa", nView, 0, NULL);
        for (int k = 0; k < NPEER; k++)
            if (peers[k].id) {
                char b[80];
                snprintf(b, sizeof b, "Ver a mesa de %s (vida %d)", peers[k].name, peers[k].life);
                net_menu_add(m, b, nView, k + 1, NULL);
            }
        net_menu_add(m, "Registro do que os outros fizeram", nLog, 0, NULL);
        net_menu_add(m, "Sair da rede", nLeave, 0, NULL);
    }
    net_menu_relabel(m, relabelNet);
}
static void relabelNet(void) {
    char b[80];
    snprintf(b, sizeof b, "Sala: < %d >  (todos na mesma sala)", room);
    if (state == 0) net_menu_set(0, b);
}

int net_log_on(void) { return logOn; }
void net_log_input(int any) {
    if (any) logOn = 0;
}
void net_draw_log(void) {
    fill(0, 0, SW, SH, (SDL_Color){6, 9, 11, 245});
    text(14, 8, "Registro da mesa", 15, 1, cAcc);
    int y = 32;
    if (!nlog) text(14, y, "Nada ainda.", 11, 0, cMut);
    for (int k = 0; k < nlog; k++) {
        char b[130];
        fit(b, sizeof b, logLines[k], 11, 0, SW - 28);
        text(14, y, b, 11, 0, cTxt);
        y += 14;
    }
    textC(SW / 2, SH - 16, "Qualquer botão fecha", 9, 0, cMut);
}
