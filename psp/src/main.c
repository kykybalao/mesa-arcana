/* Mesa Arcana para PSP
 * Mesa de Commander para jogar sozinho (e em rede entre PSPs). Os decks vêm prontos do programa do PC
 * (botão "PSP"): texto das cartas e imagens já reduzidas, na pasta decks/<deck>.
 * O mesmo código compila no PC com SDL2 para teste (teclado no lugar dos botões). */
#include "base.h"
#include <dirent.h>
#include <stdarg.h>
#include <sys/stat.h>
#include <time.h>
#ifdef __PSP__
#include <psppower.h>
#endif

/* no PSP, o SDL2main já declara o módulo e a memória */

const SDL_Color cBg = {14, 19, 22, 255}, cPanel = {27, 36, 42, 255}, cPanel2 = {38, 50, 58, 255}, cLine = {56, 76, 86, 255},
                cTxt = {230, 236, 240, 255}, cMut = {140, 156, 166, 255}, cAcc = {232, 185, 74, 255}, cAcc2 = {79, 195, 161, 255},
                cBad = {235, 96, 96, 255}, cFelt = {22, 50, 58, 255}, cInk = {30, 26, 22, 255}, cPaper = {236, 228, 210, 255};

const char *stepNames[6] = {"Manutenção", "Compra", "Principal 1", "Combate", "Principal 2", "Final"};

SDL_Renderer *R;
static SDL_Window *win;
Def defs[MAXDEF];
int ndef;
State S;
static State undoS[UNDO];
static int nundo;
char deckDir[256], deckName[96];
static int running = 1;
unsigned frameN;
int nOpen; /* quantos arquivos o programa abriu (aparece na tela de atalhos, para conferir o cartão) */

/* processador: 333 MHz só enquanto carrega; na partida fica no modo escolhido (padrão 222, gasta menos bateria) */
static int cpuMax; /* 1 = sempre 333 (menu START) */
static void cpuSpeed(int fast) {
#ifdef __PSP__
    if (fast || cpuMax)
        scePowerSetClockFrequency(333, 333, 166);
    else
        scePowerSetClockFrequency(222, 222, 111);
#else
    (void)fast;
#endif
}

/* ===================== texto ===================== */

static TTF_Font *fonts[2][40];
static void *fontData[2];
static size_t fontLen[2];
/* as duas fontes são lidas do Memory Stick uma vez só; cada tamanho novo abre da memória (sem travadinha) */
static TTF_Font *font(int size, int bold) {
    if (size < 6) size = 6;
    if (size > 39) size = 39;
    if (fonts[bold][size]) return fonts[bold][size];
    static int failed[2];
    if (failed[bold]) return bold ? font(size, 0) : NULL; /* sem uib.ttf: usa a normal, sem tentar abrir de novo */
    if (!fontData[bold]) {
        nOpen++;
        FILE *f = fopen(bold ? "uib.ttf" : "ui.ttf", "rb");
        if (!f) {
            failed[bold] = 1;
            return bold ? font(size, 0) : NULL;
        }
        fseek(f, 0, SEEK_END);
        fontLen[bold] = ftell(f);
        fseek(f, 0, SEEK_SET);
        fontData[bold] = malloc(fontLen[bold]);
        if (fread(fontData[bold], 1, fontLen[bold], f) != fontLen[bold]) {
            free(fontData[bold]);
            fontData[bold] = NULL;
        }
        fclose(f);
        if (!fontData[bold]) {
            failed[bold] = 1;
            return bold ? font(size, 0) : NULL;
        }
    }
    fonts[bold][size] = TTF_OpenFontRW(SDL_RWFromConstMem(fontData[bold], (int)fontLen[bold]), 1, size);
    return fonts[bold][size];
}

typedef struct {
    unsigned h;
    char *s;
    int size, bold, wrap;
    SDL_Color c;
    SDL_Texture *t;
    int w, hh;
    unsigned used;
} TC;
#define NTC 512 /* 64 baldes de 8: a busca olha só 8 entradas e quase nunca precisa redesenhar texto */
static TC tcache[NTC];

static unsigned hstr(const char *s, int a, int b, int c, SDL_Color col) {
    unsigned h = 2166136261u ^ (unsigned)(a * 31 + b * 7 + c * 131) ^ ((unsigned)col.r << 16 | col.g << 8 | col.b);
    while (*s) h = (h ^ (unsigned char)*s++) * 16777619u;
    return h;
}

static TC *textTex(const char *s, int size, int bold, SDL_Color c, int wrap) {
    if (!s || !*s) return NULL;
    unsigned h = hstr(s, size, bold, wrap, c);
    TC *base = &tcache[(h % (NTC / 8)) * 8], *old = base;
    for (int i = 0; i < 8; i++) {
        TC *t = &base[i];
        if (t->t && t->h == h && t->size == size && t->bold == bold && t->wrap == wrap && !strcmp(t->s, s)) {
            t->used = frameN;
            return t;
        }
        if (!t->t) { old = t; break; }
        if (t->used < old->used) old = t;
    }
    TTF_Font *f = font(size, bold);
    if (!f) return NULL;
    SDL_Surface *sf = wrap > 0 ? TTF_RenderUTF8_Blended_Wrapped(f, s, c, wrap) : TTF_RenderUTF8_Blended(f, s, c);
    if (!sf) return NULL;
    if (sf->h > 512) { /* textura grande demais para o PSP: corta */
        SDL_Surface *cut = SDL_CreateRGBSurfaceWithFormat(0, sf->w, 512, 32, sf->format->format);
        SDL_Rect r = {0, 0, sf->w, 512};
        SDL_SetSurfaceBlendMode(sf, SDL_BLENDMODE_NONE);
        SDL_BlitSurface(sf, &r, cut, NULL);
        SDL_FreeSurface(sf);
        sf = cut;
    }
    if (old->t) {
        SDL_DestroyTexture(old->t);
        free(old->s);
    }
    old->t = SDL_CreateTextureFromSurface(R, sf);
    old->w = sf->w;
    old->hh = sf->h;
    SDL_FreeSurface(sf);
    old->h = h;
    old->s = strdup(s);
    old->size = size;
    old->bold = bold;
    old->wrap = wrap;
    old->c = c;
    old->used = frameN;
    return old->t ? old : NULL;
}

int text(int x, int y, const char *s, int size, int bold, SDL_Color c) {
    TC *t = textTex(s, size, bold, c, 0);
    if (!t) return 0;
    SDL_Rect d = {x, y, t->w, t->hh};
    SDL_RenderCopy(R, t->t, NULL, &d);
    return t->w;
}
int textC(int cx, int y, const char *s, int size, int bold, SDL_Color c) {
    TC *t = textTex(s, size, bold, c, 0);
    if (!t) return 0;
    SDL_Rect d = {cx - t->w / 2, y, t->w, t->hh};
    SDL_RenderCopy(R, t->t, NULL, &d);
    return t->w;
}
int textWrap(int x, int y, int w, const char *s, int size, int bold, SDL_Color c) {
    TC *t = textTex(s, size, bold, c, w);
    if (!t) return 0;
    SDL_Rect d = {x, y, t->w, t->hh};
    SDL_RenderCopy(R, t->t, NULL, &d);
    return t->hh;
}
int textW(const char *s, int size, int bold) {
    int w = 0, h = 0;
    if (s && *s && font(size, bold)) TTF_SizeUTF8(font(size, bold), s, &w, &h);
    return w;
}
/* fit: corta o texto (sem quebrar letras acentuadas) para caber em maxw */
void fit(char *out, int cap, const char *s, int size, int bold, int maxw) {
    snprintf(out, cap, "%s", s ? s : "");
    if (textW(out, size, bold) <= maxw) return;
    int n = strlen(out);
    while (n > 0) {
        n--;
        while (n > 0 && (out[n] & 0xC0) == 0x80) n--;
        out[n] = 0;
        char tmp[256];
        snprintf(tmp, sizeof tmp, "%s…", out);
        if (textW(tmp, size, bold) <= maxw) {
            snprintf(out, cap, "%s", tmp);
            return;
        }
    }
}

void fill(int x, int y, int w, int h, SDL_Color c) {
    SDL_SetRenderDrawColor(R, c.r, c.g, c.b, c.a);
    SDL_Rect r = {x, y, w, h};
    SDL_RenderFillRect(R, &r);
}
void frame(int x, int y, int w, int h, SDL_Color c) {
    SDL_SetRenderDrawColor(R, c.r, c.g, c.b, c.a);
    SDL_Rect r = {x, y, w, h};
    SDL_RenderDrawRect(R, &r);
}

static char toast[120];
static int toastT;
void announce(const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(toast, sizeof toast, fmt, ap);
    va_end(ap);
    toastT = 75;
}

/* ===================== botões ===================== */

static int held[NB], pressed[NB], rep[NB];
static int stickY;       /* alavanca analógica, eixo vertical (-32768..32767) */
static int stickScrolls; /* na mesa com o painel: a alavanca (cima/baixo) rola o texto do painel */
#ifdef __PSP__
static int padHeld[NB], stick[NB]; /* botões e alavanca analógica, juntos em held[] */
#endif
static const char *btnNames[NB] = {"U", "D", "L", "R", "X", "O", "T", "Q", "LT", "RT", "ST", "SE"};

/* roteiro de teste (arquivo teste.txt ao lado do programa) */
static char *script;
static char *scriptPos;
static int scriptWait;
static char shotName[64];
static int scriptHeld[NB];

static void scriptStep(void) {
    if (!scriptPos) return;
    if (scriptWait > 0) {
        scriptWait--;
        return;
    }
    while (*scriptPos == ' ' || *scriptPos == '\n' || *scriptPos == '\r' || *scriptPos == '\t') scriptPos++;
    if (!*scriptPos) {
        scriptPos = NULL;
        running = 0;
        return;
    }
    char tok[64];
    int n = 0;
    while (*scriptPos && *scriptPos != ' ' && *scriptPos != '\n' && *scriptPos != '\r' && n < 63) tok[n++] = *scriptPos++;
    tok[n] = 0;
    if (tok[0] == 'W' && tok[1] >= '0' && tok[1] <= '9') {
        scriptWait = atoi(tok + 1);
        return;
    }
    if (tok[0] == 'P' && tok[1] == ':') {
        snprintf(shotName, sizeof shotName, "%s", tok + 2);
        return;
    }
    if (!strcmp(tok, "QUIT")) {
        running = 0;
        return;
    }
    if (tok[0] == 'A' && tok[1] == ':') { /* alavanca: A:20000 ... A:0 */
        stickY = atoi(tok + 2);
        return;
    }
    if (tok[0] == 'H' && tok[1] == ':') { /* segura um botão: H:RT ... H:-RT solta */
        int off = tok[2] == '-';
        for (int b = 0; b < NB; b++)
            if (!strcmp(tok + 2 + off, btnNames[b])) scriptHeld[b] = !off;
        return;
    }
    for (char *p = strtok(tok, "+"); p; p = strtok(NULL, "+"))
        for (int b = 0; b < NB; b++)
            if (!strcmp(p, btnNames[b])) pressed[b] = 1;
    scriptWait = 2;
}

static void readInput(void) {
    SDL_Event e;
    int down[NB] = {0};
    memset(pressed, 0, sizeof pressed);
    while (SDL_PollEvent(&e)) {
        if (e.type == SDL_QUIT) running = 0;
#ifdef __PSP__
        if (e.type == SDL_JOYBUTTONDOWN || e.type == SDL_JOYBUTTONUP) {
            /* botões do PSP no SDL2: 0 triângulo, 1 círculo, 2 xis, 3 quadrado, 4 L, 5 R, 6 baixo, 7 esquerda, 8 cima, 9 direita, 10 select, 11 start */
            static const int map[12] = {B_TRI, B_O, B_X, B_SQ, B_L, B_R, B_DOWN, B_LEFT, B_UP, B_RIGHT, B_SELECT, B_START};
            int b = e.jbutton.button;
            if (b < 12) {
                if (e.type == SDL_JOYBUTTONDOWN) {
                    if (!held[map[b]]) down[map[b]] = 1;
                    padHeld[map[b]] = 1;
                } else
                    padHeld[map[b]] = 0;
            }
        }
        if (e.type == SDL_JOYAXISMOTION && e.jaxis.axis < 2) { /* alavanca: zona morta grande contra o "drift" */
            int v = e.jaxis.value, lo = e.jaxis.axis ? B_UP : B_LEFT, hi = e.jaxis.axis ? B_DOWN : B_RIGHT;
            if (e.jaxis.axis) stickY = v;
            stick[lo] = v < -22000;
            stick[hi] = v > 22000;
        }
#else
        if (e.type == SDL_KEYDOWN || e.type == SDL_KEYUP) {
            int b = -1;
            switch (e.key.keysym.sym) {
            case SDLK_UP: b = B_UP; break;
            case SDLK_DOWN: b = B_DOWN; break;
            case SDLK_LEFT: b = B_LEFT; break;
            case SDLK_RIGHT: b = B_RIGHT; break;
            case SDLK_z: b = B_X; break;
            case SDLK_x: b = B_O; break;
            case SDLK_s: b = B_TRI; break;
            case SDLK_a: b = B_SQ; break;
            case SDLK_q: b = B_L; break;
            case SDLK_w: b = B_R; break;
            case SDLK_RETURN: b = B_START; break;
            case SDLK_BACKSPACE: b = B_SELECT; break;
            case SDLK_ESCAPE: running = 0; break;
            }
            if (b >= 0) {
                if (e.type == SDL_KEYDOWN && !e.key.repeat) down[b] = 1;
                held[b] = e.type == SDL_KEYDOWN;
            }
        }
#endif
    }
#ifdef __PSP__
    for (int b = 0; b < NB; b++) {
        int now = padHeld[b] || (stick[b] && !(stickScrolls && (b == B_UP || b == B_DOWN)));
        if (now && !held[b]) down[b] = 1;
        held[b] = now;
    }
#endif
    for (int b = 0; b < NB; b++) {
        pressed[b] = down[b];
        if (b <= B_RIGHT && held[b]) { /* direcionais repetem quando seguradas */
            rep[b]++;
            if (rep[b] > 18 && rep[b] % 5 == 0) pressed[b] = 1;
        } else
            rep[b] = 0;
    }
    if (scriptPos || scriptWait) {
        for (int b = 0; b < NB; b++) held[b] = scriptHeld[b];
        scriptStep();
    }
}

/* ===================== deck ===================== */

static char *unesc(const char *s) {
    char *o = malloc(strlen(s) + 1), *p = o;
    for (; *s; s++) {
        if (s[0] == '\\' && s[1] == 'n') {
            *p++ = '\n';
            s++;
        } else
            *p++ = *s;
    }
    *p = 0;
    return o;
}

static void freeDeck(void) {
    for (int i = 0; i < ndef; i++) {
        for (int f = 0; f < defs[i].nf; f++) {
            Face *fc = &defs[i].f[f];
            free(fc->n), free(fc->c), free(fc->t), free(fc->x), free(fc->p), free(fc->o), free(fc->l);
            if (defs[i].sm[f]) SDL_DestroyTexture(defs[i].sm[f]);
        }
    }
    memset(defs, 0, sizeof defs);
    ndef = 0;
}

/* tela de "carregando" enquanto as imagens saem do Memory Stick */
static void loadingFrame(const char *name, int n) {
    SDL_SetRenderDrawColor(R, cBg.r, cBg.g, cBg.b, 255);
    SDL_RenderClear(R);
    char b[140];
    snprintf(b, sizeof b, "Abrindo %s", name);
    textC(SW / 2, SH / 2 - 30, b, 13, 1, cAcc);
    snprintf(b, sizeof b, "%d cartas", n);
    textC(SW / 2, SH / 2 + 14, b, 10, 0, cMut);
    int w = 220, x = (SW - w) / 2, y = SH / 2 - 4;
    fill(x, y, w, 8, cPanel2);
    int p = n * w / 110;
    fill(x, y, p < w ? p : w, 8, cAcc2);
    SDL_RenderPresent(R);
}
static int loadDeck(const char *dir) {
    char path[320], line[8192];
    cpuSpeed(1);
    snprintf(path, sizeof path, "%s/deck.txt", dir);
    FILE *f = fopen(path, "r");
    if (!f) return 0;
    freeDeck();
    snprintf(deckDir, sizeof deckDir, "%s", dir);
    Def *cur = NULL;
    int fi = 0;
    while (fgets(line, sizeof line, f)) {
        line[strcspn(line, "\r\n")] = 0;
        char *fld[10] = {0};
        int nf = 0;
        char *p = line;
        while (nf < 10) {
            fld[nf++] = p;
            char *t = strchr(p, '\t');
            if (!t) break;
            *t = 0;
            p = t + 1;
        }
        if (!strcmp(fld[0], "NOME") && nf > 1)
            snprintf(deckName, sizeof deckName, "%s", fld[1]);
        else if (!strcmp(fld[0], "CARTA") && nf >= 6 && ndef < MAXDEF) {
            cur = &defs[ndef++];
            cur->qty = atoi(fld[2]);
            cur->cmdr = atoi(fld[3]);
            cur->token = atoi(fld[4]);
            cur->nf = atoi(fld[5]);
            if (cur->nf < 1) cur->nf = 1;
            if (cur->nf > 2) cur->nf = 2;
            fi = 0;
        } else if (!strcmp(fld[0], "FACE") && cur && fi < cur->nf) {
            Face *fc = &cur->f[fi];
            const char *v[7];
            for (int k = 0; k < 7; k++) v[k] = k + 1 < nf ? fld[k + 1] : "";
            fc->n = unesc(v[0]), fc->c = unesc(v[1]), fc->t = unesc(v[2]), fc->x = unesc(v[3]);
            fc->p = unesc(v[4]), fc->o = unesc(v[5]), fc->l = unesc(v[6]);
            snprintf(path, sizeof path, "%s/img/%d_%d.jpg", dir, (int)(cur - defs), fi);
            cur->sm[fi] = IMG_LoadTexture(R, path), nOpen++;
            fi++;
            if (fi == 1 && ndef % 8 == 1) loadingFrame(deckName, ndef);
        }
    }
    fclose(f);
    for (int i = 0; i < ndef; i++) /* carta com uma face a menos que o anunciado */
        for (int k = 0; k < defs[i].nf; k++)
            if (!defs[i].f[k].n) defs[i].nf = k;
    cpuSpeed(0);
    return ndef > 0;
}

/* ===================== jogo ===================== */

static int iSel = -1; /* carta escolhida para menus */

static void snap(void) {
    if (nundo == UNDO) {
        memmove(undoS, undoS + 1, sizeof(State) * (UNDO - 1));
        nundo--;
    }
    undoS[nundo++] = S;
}
static void undoLast(void) {
    if (!nundo) {
        announce("Nada para desfazer");
        return;
    }
    S = undoS[--nundo];
    announce("Desfeito");
}

Face *faceOf(int id) {
    Inst *i = &S.in[id];
    Def *d = &defs[i->def];
    return &d->f[i->face < d->nf ? i->face : 0];
}

static void zrem(int id) {
    int z = S.in[id].zone;
    for (int k = 0; k < S.zn[z]; k++)
        if (S.z[z][k] == id) {
            memmove(&S.z[z][k], &S.z[z][k + 1], sizeof(short) * (S.zn[z] - k - 1));
            S.zn[z]--;
            return;
        }
}
static void zadd(int z, int id, int top) {
    if (S.zn[z] >= MAXI) return;
    if (top) {
        memmove(&S.z[z][1], &S.z[z][0], sizeof(short) * S.zn[z]);
        S.z[z][0] = id;
    } else
        S.z[z][S.zn[z]] = id;
    S.zn[z]++;
    S.in[id].zone = z;
}
static int newInst(int def, int zone, int token) {
    for (int i = 1; i < MAXI; i++)
        if (!S.in[i].used) {
            memset(&S.in[i], 0, sizeof(Inst));
            S.in[i].used = 1;
            S.in[i].def = def;
            S.in[i].token = token;
            zadd(zone, i, 0);
            return i;
        }
    announce("Cartas demais na mesa");
    return 0;
}

static const char *zoneNames[NZ] = {"grimório", "mão", "campo", "cemitério", "exílio", "zona de comando"};

/* move a carta; top vale para o grimório (1 = em cima, 0 = embaixo) */
static void moveTo(int id, int z, int top) {
    Inst *i = &S.in[id];
    if (i->zone == z && z != Z_LIB) return;
    zrem(id);
    if (z != Z_BF) {
        /* saiu do campo: solta o que estava preso nela (fica no campo, sem ir sozinho ao cemitério) e se solta da dona */
        i->att = 0;
        for (int k = 1; k < MAXI; k++)
            if (S.in[k].used && S.in[k].att == id) S.in[k].att = 0;
        i->tapped = 0;
        i->plus = i->cnt = 0;
        i->face = 0;
        if (i->token) { /* ficha fora do campo deixa de existir */
            i->used = 0;
            return;
        }
        i->down = 0;
    }
    zadd(z, id, z == Z_LIB ? top : 0);
}

static void shuffleLib(void) {
    for (int k = S.zn[Z_LIB] - 1; k > 0; k--) {
        int j = rand() % (k + 1);
        short t = S.z[Z_LIB][k];
        S.z[Z_LIB][k] = S.z[Z_LIB][j];
        S.z[Z_LIB][j] = t;
    }
}
static int drawOne(void) {
    if (!S.zn[Z_LIB]) return 0;
    moveTo(S.z[Z_LIB][0], Z_HAND, 0);
    return 1;
}
static void drawCmd(int n) {
    snap();
    int k = 0;
    while (k < n && drawOne()) k++;
    announce(k ? "Comprou %d" : "O grimório acabou", k);
}

static time_t gameStart;
static int gameSaved;
static void newGame(void) {
    memset(&S, 0, sizeof S);
    gameStart = time(NULL), gameSaved = 0;
    nundo = 0;
    S.life = 40;
    S.turn = 1;
    for (int d = 0; d < ndef; d++) {
        if (defs[d].token) continue;
        for (int k = 0; k < defs[d].qty; k++) {
            int id = newInst(d, (defs[d].cmdr && k == 0) ? Z_CMD : Z_LIB, 0);
            if (defs[d].cmdr && k == 0) S.in[id].cmdr = 1;
        }
    }
    if (!S.zn[Z_CMD]) S.life = 20; /* deck sem comandante: vida normal de 20 */
    shuffleLib();
    for (int k = 0; k < 7; k++) drawOne();
    S.phase = PH_MULL;
    net_log("começou uma partida com %s", deckName);
}

static void mulligan(void) {
    snap();
    while (S.zn[Z_HAND]) moveTo(S.z[Z_HAND][0], Z_LIB, 0);
    shuffleLib();
    for (int k = 0; k < 7; k++) drawOne();
    S.mulls++;
    announce("Mulligan %d", S.mulls);
}
static int bottomCount(void) { return S.mulls > 1 ? S.mulls - 1 : 0; }
static void keepHand(void) {
    if (bottomCount() > 0) {
        S.phase = PH_BOTTOM;
        S.npick = 0;
        announce("Escolha %d para o fundo", bottomCount());
    } else {
        S.phase = PH_PLAY;
        announce("Turno 1 · %s", stepNames[0]);
    }
}
static void confirmBottom(void) {
    if (S.npick != bottomCount()) {
        announce("Marque %d carta(s) com X", bottomCount());
        return;
    }
    for (int k = 0; k < S.npick; k++) moveTo(S.pick[k], Z_LIB, 0);
    S.npick = 0;
    S.phase = PH_PLAY;
    announce("Turno 1 · %s", stepNames[0]);
}

static void untapAll(void) {
    for (int k = 0; k < S.zn[Z_BF]; k++) S.in[S.z[Z_BF][k]].tapped = 0;
}
static void nextTurn(void) {
    snap();
    untapAll();
    S.turn++;
    S.step = 0;
    announce("Turno %d · %s", S.turn, stepNames[0]);
    net_log("turno %d", S.turn);
}
static void nextStep(void) {
    if (S.phase != PH_PLAY) return;
    if (S.step >= 5) {
        nextTurn();
        return;
    }
    S.step++;
    announce("%s", stepNames[S.step]);
    net_log("fase: %s", stepNames[S.step]);
}

/* dona válida de uma carta presa (0 se solta) */
static int hostOf(int id) {
    int h = S.in[id].att;
    if (h > 0 && h < MAXI && S.in[h].used && S.in[h].zone == Z_BF && S.in[id].zone == Z_BF) return h;
    return 0;
}
static int rowOf(int id) {
    Face *f = faceOf(id);
    if (S.in[id].down) return 0;
    if (strstr(f->t, "Criatura") || strstr(f->t, "Creature")) return 0;
    if (strstr(f->t, "Terreno") || strstr(f->t, "Land")) return 2;
    return 1;
}

/* ===================== imagens ===================== */

void drawBack(int x, int y, int w, int h) {
    fill(x, y, w, h, (SDL_Color){11, 15, 18, 255});
    fill(x + 2, y + 2, w - 4, h - 4, (SDL_Color){34, 48, 56, 255});
    frame(x + 4, y + 4, w - 8, h - 8, (SDL_Color){52, 72, 82, 255});
    int cx = x + w / 2, cy = y + h / 2, r = w / 4;
    SDL_SetRenderDrawColor(R, cAcc.r, cAcc.g, cAcc.b, 255);
    SDL_Point p[5] = {{cx, cy - r}, {cx + r, cy}, {cx, cy + r}, {cx - r, cy}, {cx, cy - r}};
    SDL_RenderDrawLines(R, p, 5);
    fill(cx - r / 3, cy - r / 3, r / 3 * 2 + 1, r / 3 * 2 + 1, cAcc2);
}

int showBar = 1;
static int infoPanel = 1; /* painel da carta escolhida, à esquerda (menu START) */
static int infoScroll;    /* rolagem do texto do painel (alavanca analógica) */
/* carta sem imagem: retângulo de papel com o nome */
void drawPaper(const Face *f, int x, int y, int w, int h) {
    fill(x, y, w, h, (SDL_Color){125, 117, 102, 255});
    fill(x + 1, y + 1, w - 2, h - 2, cPaper);
    char buf[96];
    if (showBar && w >= 28 && h < 200) {
        if (f->t && *f->t) textWrap(x + 2, y + (w >= 56 ? 13 : 12), w - 4, f->t, 7, 0, cInk);
        return;
    }
    snprintf(buf, sizeof buf, "%s", f->n);
    textWrap(x + 2, y + 2, w - 4, buf, w > 50 ? 9 : 7, 1, cInk);
}

/* custo de mana em bolinhas coloridas, alinhado à direita em xr; devolve a largura */
static SDL_Color pipColor(const char *sym) {
    switch (sym[0]) {
    case 'W': return (SDL_Color){240, 230, 196, 255};
    case 'U': return (SDL_Color){92, 152, 214, 255};
    case 'B': return (SDL_Color){88, 78, 92, 255};
    case 'R': return (SDL_Color){214, 92, 70, 255};
    case 'G': return (SDL_Color){86, 168, 104, 255};
    }
    return (SDL_Color){178, 172, 160, 255};
}
/* bolinha: desenhada uma vez por raio numa textura branca e tingida na hora (1 desenho em vez de ~15) */
static SDL_Texture *discTex[16];
static void disc(int cx, int cy, int r, SDL_Color c) {
    if (r < 1) return;
    if (r > 15) r = 15;
    if (!discTex[r]) {
        int d = r * 2 + 1;
        SDL_Surface *sf = SDL_CreateRGBSurfaceWithFormat(0, d, d, 32, SDL_PIXELFORMAT_RGBA32);
        if (!sf) return;
        Uint32 *px = sf->pixels;
        for (int y = 0; y < d; y++)
            for (int x = 0; x < d; x++) {
                int dx = x - r, dy = y - r;
                px[y * (sf->pitch / 4) + x] = dx * dx + dy * dy <= r * r + r ? 0xffffffffu : 0;
            }
        discTex[r] = SDL_CreateTextureFromSurface(R, sf);
        SDL_FreeSurface(sf);
        if (!discTex[r]) return;
        SDL_SetTextureBlendMode(discTex[r], SDL_BLENDMODE_BLEND);
    }
    SDL_SetTextureColorMod(discTex[r], c.r, c.g, c.b);
    SDL_SetTextureAlphaMod(discTex[r], c.a);
    SDL_Rect d = {cx - r, cy - r, r * 2 + 1, r * 2 + 1};
    SDL_RenderCopy(R, discTex[r], NULL, &d);
}
int drawCost(int xr, int y, const char *cost, int d) {
    char sym[16][8];
    int n = 0;
    for (const char *p = cost; *p && n < 16; p++) {
        if (*p != '{') continue;
        const char *e = strchr(p, '}');
        if (!e) break;
        int k = 0;
        for (const char *q = p + 1; q < e && k < 7; q++)
            if (*q != '/') sym[n][k++] = *q;
        sym[n][k] = 0;
        n++;
        p = e;
    }
    if (!n) return 0;
    int step = d + 1, w = n * step, x = xr - w;
    int fs = d >= 12 ? d - 3 : d;
    for (int k = 0; k < n; k++) {
        int cx = x + k * step + d / 2, cy = y + d / 2;
        disc(cx, cy, d / 2, (SDL_Color){20, 18, 16, 255});
        disc(cx, cy, d / 2 - 1, pipColor(sym[k]));
        /* número, X ou letra só quando há espaço; em carta pequena a cor basta */
        int isNum = (sym[k][0] >= '0' && sym[k][0] <= '9') || sym[k][0] == 'X';
        if (isNum || d >= 12) {
            char t[4] = {sym[k][0], isNum ? sym[k][1] : 0, 0, 0};
            int tw = textW(t, fs, 1);
            text(cx - tw / 2, cy - fs / 2 - 2, t, fs, 1, (SDL_Color){20, 18, 16, 255});
        }
    }
    return w;
}
/* faixa com nome e custo no topo da carta */
void nameBar(const Face *f, int x, int y, int w) {
    if (w < 28 || !f || !f->n) return;
    char buf[96];
    int hasCost = f->c && *f->c;
    if (w >= 120) { /* carta grande: nome e custo na mesma linha */
        int bh = 18;
        fill(x, y, w, bh, (SDL_Color){14, 16, 18, 215});
        int cw = hasCost ? drawCost(x + w - 2, y + 1, f->c, bh - 2) : 0;
        fit(buf, sizeof buf, f->n, 12, 1, w - cw - 8);
        text(x + 3, y + 1, buf, 12, 1, cTxt);
        return;
    }
    /* carta pequena: nome na largura toda e custo numa linha embaixo */
    int fs = w >= 56 ? 8 : 7, nh = fs + 3, d = 7;
    fill(x, y, w, nh, (SDL_Color){14, 16, 18, 215});
    fit(buf, sizeof buf, f->n, fs, 1, w - 3);
    text(x + 2, y + (nh - fs) / 2 - 2, buf, fs, 1, cTxt);
    if (hasCost) {
        int cw = drawCost(x + w - 1, y + nh + 1, f->c, d);
        (void)cw;
    }
}

static void drawCardTex(int def, int face, int down, int x, int y, int w, int h, int tapped) {
    Def *d = &defs[def];
    SDL_Rect dst = {x, y, w, h};
    if (tapped) { /* deitada: ocupa h x w e gira em volta do centro */
        dst.x = x + (h - w) / 2;
        dst.y = y + (w - h) / 2;
    }
    if (down) {
        if (tapped)
            drawBack(x, y, h, w);
        else
            drawBack(x, y, w, h);
        return;
    }
    SDL_Texture *t = d->sm[face < d->nf ? face : 0];
    const Face *f = &d->f[face < d->nf ? face : 0];
    if (t)
        SDL_RenderCopyEx(R, t, NULL, &dst, tapped ? 90 : 0, NULL, SDL_FLIP_NONE);
    else if (tapped)
        drawPaper(f, x, y, h, w);
    else
        drawPaper(f, x, y, w, h);
    if (showBar) nameBar(f, x, y, tapped ? h : w);
}

void badge(int x, int y, const char *s, SDL_Color bg, SDL_Color fg) {
    int w = textW(s, 9, 1) + 6;
    fill(x, y, w, 12, bg);
    text(x + 3, y, s, 9, 1, fg);
}

/* ===================== seleção ===================== */

typedef struct {
    int x, y, w, h, kind, zone, id;
} Sel;
static Sel items[MAXI + 32];
static int nitems;
int selKind, selZone, selId;
static int selX = 200, selY = 230;

void addSel(int x, int y, int w, int h, int kind, int zone, int id) {
    if (nitems < MAXI + 32) items[nitems++] = (Sel){x, y, w, h, kind, zone, id};
}
int isSel(int kind, int zone, int id) { return selKind == kind && selZone == zone && selId == id; }

static void resolveSel(void) {
    for (int k = 0; k < nitems; k++)
        if (isSel(items[k].kind, items[k].zone, items[k].id)) {
            selX = items[k].x + items[k].w / 2, selY = items[k].y + items[k].h / 2;
            return;
        }
    int best = -1, bd = 1 << 30;
    for (int pass = 0; pass < 2 && best < 0; pass++) /* primeiro na mesma zona (ex.: a próxima carta da mão) */
        for (int k = 0; k < nitems; k++) {
            if (pass == 0 && (items[k].kind != selKind || items[k].zone != selZone)) continue;
            int dx = items[k].x + items[k].w / 2 - selX, dy = items[k].y + items[k].h / 2 - selY;
            if (dx * dx + dy * dy < bd) bd = dx * dx + dy * dy, best = k;
        }
    if (best >= 0) {
        selKind = items[best].kind, selZone = items[best].zone, selId = items[best].id;
        selX = items[best].x + items[best].w / 2, selY = items[best].y + items[best].h / 2;
    } else
        selKind = 0;
}

static void moveSel(int dir) {
    int best = -1;
    long long bs = 1LL << 40;
    for (int k = 0; k < nitems; k++) {
        if (isSel(items[k].kind, items[k].zone, items[k].id)) continue;
        int dx = items[k].x + items[k].w / 2 - selX, dy = items[k].y + items[k].h / 2 - selY;
        int along = dir == B_RIGHT ? dx : dir == B_LEFT ? -dx : dir == B_DOWN ? dy : -dy;
        int across = (dir == B_LEFT || dir == B_RIGHT) ? abs(dy) : abs(dx);
        if (along <= 2) continue;
        long long s = (long long)along + (long long)across * 3;
        if (s < bs) bs = s, best = k;
    }
    if (best >= 0) {
        selKind = items[best].kind, selZone = items[best].zone, selId = items[best].id;
        selX = items[best].x + items[best].w / 2, selY = items[best].y + items[best].h / 2;
    }
}

/* ===================== menus ===================== */

typedef struct {
    char label[80];
    void (*fn)(int);
    int arg;
    void (*adj)(int arg, int d);
    int keep;
} MItem;
typedef struct {
    char title[80];
    MItem it[26];
    int n, cur;
    void (*relabel)(void);
} Menu;
static Menu menus[4];
static int nmenu;

static Menu *openMenu(const char *title) {
    if (nmenu >= 4) nmenu = 3;
    Menu *m = &menus[nmenu++];
    memset(m, 0, sizeof *m);
    snprintf(m->title, sizeof m->title, "%s", title);
    return m;
}
static MItem *mAdd(Menu *m, const char *label, void (*fn)(int), int arg) {
    if (m->n >= 26) return &m->it[25];
    MItem *it = &m->it[m->n++];
    snprintf(it->label, sizeof it->label, "%s", label);
    it->fn = fn;
    it->arg = arg;
    return it;
}
static void closeMenus(void) { nmenu = 0; }

void *net_open_menu(const char *t) { return openMenu(t); }
void net_menu_add(void *m, const char *label, void (*fn)(int), int arg, void (*adj)(int, int)) {
    MItem *it = mAdd((Menu *)m, label, fn, arg);
    it->adj = adj;
    it->keep = adj != NULL;
}
void net_menu_relabel(void *m, void (*fn)(void)) {
    ((Menu *)m)->relabel = fn;
    if (fn) fn();
}
void net_menu_set(int k, const char *label) {
    Menu *m = &menus[nmenu - 1];
    if (nmenu && k < m->n) snprintf(m->it[k].label, sizeof m->it[k].label, "%s", label);
}
static void menuInput(void) {
    Menu *m = &menus[nmenu - 1];
    if (pressed[B_UP]) m->cur = (m->cur + m->n - 1) % m->n;
    if (pressed[B_DOWN]) m->cur = (m->cur + 1) % m->n;
    MItem *it = &m->it[m->cur];
    if (it->adj && (pressed[B_LEFT] || pressed[B_RIGHT])) {
        it->adj(it->arg, pressed[B_RIGHT] ? 1 : -1);
        if (m->relabel) m->relabel();
        return;
    }
    if (pressed[B_O]) {
        nmenu--;
        return;
    }
    if (pressed[B_X] && it->fn) {
        void (*fn)(int) = it->fn;
        int arg = it->arg;
        if (!it->keep) nmenu--;
        fn(arg);
        if (nmenu && menus[nmenu - 1].relabel) menus[nmenu - 1].relabel();
    }
}

static void drawMenu(void) {
    Menu *m = &menus[nmenu - 1];
    int rowH = 17, w = 300;
    for (int k = 0; k < m->n; k++) {
        int tw = textW(m->it[k].label, 11, 0) + 24;
        if (tw > w) w = tw;
    }
    if (w > SW - 20) w = SW - 20;
    int vis = m->n < 12 ? m->n : 12;
    int h = 26 + vis * rowH + 18;
    int x = (SW - w) / 2, y = (SH - h) / 2;
    fill(0, 0, SW, SH, (SDL_Color){0, 0, 0, 120});
    fill(x - 1, y - 1, w + 2, h + 2, cAcc);
    fill(x, y, w, h, cPanel);
    char buf[120];
    fit(buf, sizeof buf, m->title, 12, 1, w - 16);
    text(x + 8, y + 5, buf, 12, 1, cAcc);
    int first = m->cur - vis + 1 > 0 ? m->cur - vis + 1 : 0;
    for (int k = first; k < first + vis && k < m->n; k++) {
        int yy = y + 24 + (k - first) * rowH;
        if (k == m->cur) fill(x + 4, yy, w - 8, rowH - 1, cPanel2);
        fit(buf, sizeof buf, m->it[k].label, 11, k == m->cur, w - 20);
        text(x + 10, yy + 2, buf, 11, k == m->cur, k == m->cur ? cTxt : cMut);
    }
    textC(x + w / 2, y + h - 15, m->it[m->cur].adj ? "← → muda · X escolhe · O volta" : "X escolhe · O volta", 9, 0, cMut);
}

/* ===================== zoom ===================== */

static struct {
    int on, def, face, id, scroll;
    SDL_Texture *big;
} zoom;
static Face zext; /* carta de outro jogador (def = -1) */
static char zextExtra[160];

static void openZoom(int def, int face, int id) {
    if (def < 0 || def >= ndef) return;
    if (zoom.big) SDL_DestroyTexture(zoom.big);
    zoom.big = NULL;
    zoom.on = 1, zoom.def = def, zoom.face = face < defs[def].nf ? face : 0, zoom.id = id, zoom.scroll = 0;
    char p[320];
    snprintf(p, sizeof p, "%s/img/%d_%dg.jpg", deckDir, def, zoom.face);
    zoom.big = IMG_LoadTexture(R, p), nOpen++;
}
static void closeZoom(void) {
    if (zoom.big) SDL_DestroyTexture(zoom.big);
    zoom.big = NULL;
    zoom.on = 0;
}
void zoomExt(const Face *f, const char *bigPath, const char *extra) {
    if (zoom.big) SDL_DestroyTexture(zoom.big);
    zext = *f;
    snprintf(zextExtra, sizeof zextExtra, "%s", extra ? extra : "");
    zoom.on = 1, zoom.def = -1, zoom.face = 0, zoom.id = 0, zoom.scroll = 0;
    zoom.big = bigPath && *bigPath ? IMG_LoadTexture(R, bigPath) : NULL;
}
static void zoomInput(void) {
    if (pressed[B_TRI] || pressed[B_O] || pressed[B_X]) {
        closeZoom();
        return;
    }
    if ((pressed[B_LEFT] || pressed[B_RIGHT]) && zoom.def >= 0 && defs[zoom.def].nf > 1) openZoom(zoom.def, (zoom.face + 1) % defs[zoom.def].nf, zoom.id);
    if (held[B_DOWN]) zoom.scroll += 3;
    if (held[B_UP]) zoom.scroll -= 3;
    if (zoom.scroll < 0) zoom.scroll = 0;
}
static void drawZoom(void) {
    fill(0, 0, SW, SH, (SDL_Color){6, 9, 11, 245});
    int nf = zoom.def >= 0 ? defs[zoom.def].nf : 1;
    const Face *f = zoom.def >= 0 ? &defs[zoom.def].f[zoom.face] : &zext;
    SDL_Rect r = {6, 6, 186, 260};
    if (zoom.big)
        SDL_RenderCopy(R, zoom.big, NULL, &r);
    else if (zoom.def >= 0)
        drawCardTex(zoom.def, zoom.face, 0, 6, 6, 186, 260, 0);
    else
        drawPaper(f, 6, 6, 186, 260);
    int x = 202, w = SW - x - 8;
    SDL_Rect clipR = {x, 4, w, SH - 22};
    SDL_RenderSetClipRect(R, &clipR);
    int y = 8 - zoom.scroll;
    y += textWrap(x, y, w, f->n, 14, 1, cAcc) + 2;
    if (*f->c) {
        int n = 0;
        for (const char *p = f->c; *p; p++) n += *p == '{';
        drawCost(x + n * 16, y + 1, f->c, 15);
        y += 19;
    }
    y += textWrap(x, y, w, f->t, 11, 0, cTxt) + 6;
    if (*f->x) y += textWrap(x, y, w, f->x, 11, 0, cTxt) + 6;
    char buf[96];
    if (*f->p || *f->o) {
        snprintf(buf, sizeof buf, "%s/%s", f->p, f->o);
        y += textWrap(x, y, w, buf, 13, 1, cTxt) + 4;
    }
    if (*f->l) {
        snprintf(buf, sizeof buf, "Lealdade: %s", f->l);
        y += textWrap(x, y, w, buf, 11, 1, cTxt) + 4;
    }
    if (zoom.def < 0 && *zextExtra) y += textWrap(x, y, w, zextExtra, 11, 1, cAcc) + 2;
    if (zoom.id > 0 && S.in[zoom.id].used) {
        Inst *i = &S.in[zoom.id];
        if (i->plus) {
            snprintf(buf, sizeof buf, "Marcadores %+d/%+d: %d", i->plus > 0 ? 1 : -1, i->plus > 0 ? 1 : -1, abs(i->plus));
            y += textWrap(x, y, w, buf, 11, 1, cAcc) + 2;
        }
        if (i->cnt) {
            snprintf(buf, sizeof buf, "Marcadores: %d", i->cnt);
            y += textWrap(x, y, w, buf, 11, 1, cAcc) + 2;
        }
        if (i->cmdr && i->tax) {
            snprintf(buf, sizeof buf, "Imposto do comandante: +%d", i->tax * 2);
            y += textWrap(x, y, w, buf, 11, 1, cAcc) + 2;
        }
        if (hostOf(zoom.id)) {
            snprintf(buf, sizeof buf, "Presa em: %s", faceOf(hostOf(zoom.id))->n);
            y += textWrap(x, y, w, buf, 11, 1, cAcc2) + 2;
        }
        for (int k = 1; k < MAXI; k++)
            if (S.in[k].used && hostOf(k) == zoom.id) {
                snprintf(buf, sizeof buf, "Com: %s", faceOf(k)->n);
                y += textWrap(x, y, w, buf, 11, 1, cAcc2) + 2;
            }
    }
    SDL_RenderSetClipRect(R, NULL);
    textC(x + w / 2, SH - 15, nf > 1 ? "← → outra face · ↑ ↓ rola o texto · O fecha" : "↑ ↓ rola o texto · ▲ ou O fecha", 9, 0, cMut);
}

/* ===================== ver zona (cemitério, exílio, grimório, fichas) ===================== */

enum { V_ZONE, V_TOKENS };
static struct {
    int on, mode, zone, limit, shuf, cur, top;
    char title[64];
    short ids[MAXI];
    int n;
} vw;

static void vwFill(void) {
    vw.n = 0;
    if (vw.mode == V_TOKENS) {
        for (int d = 0; d < ndef; d++)
            if (defs[d].token) vw.ids[vw.n++] = d;
    } else {
        int z = vw.zone, n = S.zn[z];
        if (vw.limit > 0 && vw.limit < n) n = vw.limit;
        for (int k = 0; k < n; k++) vw.ids[vw.n++] = (z == Z_GY || z == Z_EX) ? S.z[z][S.zn[z] - 1 - k] : S.z[z][k];
        if (z == Z_LIB && vw.limit == 0) /* busca: em ordem alfabética */
            for (int a = 1; a < vw.n; a++)
                for (int b = a; b > 0 && strcmp(faceOf(vw.ids[b - 1])->n, faceOf(vw.ids[b])->n) > 0; b--) {
                    short t = vw.ids[b];
                    vw.ids[b] = vw.ids[b - 1];
                    vw.ids[b - 1] = t;
                }
    }
    if (vw.cur >= vw.n) vw.cur = vw.n - 1;
    if (vw.cur < 0) vw.cur = 0;
}
static void openViewer(int mode, int zone, int limit, const char *title) {
    memset(&vw, 0, sizeof vw);
    vw.on = 1, vw.mode = mode, vw.zone = zone, vw.limit = limit, vw.shuf = (zone == Z_LIB && limit == 0);
    snprintf(vw.title, sizeof vw.title, "%s", title);
    vwFill();
}
static void closeViewer(void) {
    if (vw.mode == V_ZONE && vw.zone == Z_LIB && vw.limit == 0 && vw.shuf) {
        shuffleLib();
        announce("Grimório embaralhado");
    }
    vw.on = 0;
}

static void cardMenu(int id);
static void makeToken(int def) {
    snap();
    int id = newInst(def, Z_BF, 1);
    if (id) announce("Ficha: %s", defs[def].f[0].n);
}

static void viewerInput(void) {
    const int cols = 8;
    vwFill();
    if (pressed[B_LEFT] && vw.cur > 0) vw.cur--;
    if (pressed[B_RIGHT] && vw.cur < vw.n - 1) vw.cur++;
    if (pressed[B_UP] && vw.cur >= cols) vw.cur -= cols;
    if (pressed[B_DOWN] && vw.cur + cols < vw.n) vw.cur += cols;
    if ((pressed[B_L] || pressed[B_R]) && vw.n) {
        int dir = pressed[B_R] ? 1 : -1;
        if (vw.mode == V_ZONE && vw.zone == Z_LIB && vw.limit == 0) { /* busca em ordem alfabética: pula para a próxima letra */
            const char *n0 = defs[S.in[vw.ids[vw.cur]].def].f[0].n;
            int c = vw.cur;
            if (dir < 0) /* volta ao começo da letra atual; se já está nele, à letra anterior */
                while (c > 0 && defs[S.in[vw.ids[c - 1]].def].f[0].n[0] == n0[0]) c--;
            if (dir < 0 && c == vw.cur && c > 0) {
                c--;
                char l = defs[S.in[vw.ids[c]].def].f[0].n[0];
                while (c > 0 && defs[S.in[vw.ids[c - 1]].def].f[0].n[0] == l) c--;
            }
            if (dir > 0)
                while (c < vw.n - 1 && defs[S.in[vw.ids[c]].def].f[0].n[0] == n0[0]) c++;
            vw.cur = c;
        } else {
            vw.cur += dir * cols * 3;
            if (vw.cur < 0) vw.cur = 0;
            if (vw.cur > vw.n - 1) vw.cur = vw.n - 1;
        }
    }
    if (pressed[B_O]) {
        closeViewer();
        return;
    }
    if (pressed[B_SQ] && vw.mode == V_ZONE && vw.zone == Z_LIB && vw.limit == 0) vw.shuf = !vw.shuf;
    if (!vw.n) return;
    int v = vw.ids[vw.cur];
    if (pressed[B_TRI]) {
        if (vw.mode == V_TOKENS)
            openZoom(v, 0, 0);
        else
            openZoom(S.in[v].def, S.in[v].face, v);
    }
    if (pressed[B_X]) {
        if (vw.mode == V_TOKENS)
            makeToken(v);
        else
            cardMenu(v);
    }
}

static void drawViewer(void) {
    const int cols = 8, cw = 46, chh = 64, gx = 56, gy = 74;
    fill(0, 0, SW, SH, cBg);
    fill(0, 0, SW, 20, cPanel);
    char buf[128];
    snprintf(buf, sizeof buf, "%s (%d)", vw.title, vw.n);
    text(8, 3, buf, 12, 1, cAcc);
    int rows = 3, row = vw.cur / cols;
    if (row < vw.top) vw.top = row;
    if (row >= vw.top + rows) vw.top = row - rows + 1;
    int x0 = (SW - cols * gx) / 2 + (gx - cw) / 2;
    for (int k = vw.top * cols; k < vw.n && k < (vw.top + rows) * cols; k++) {
        int x = x0 + (k % cols) * gx, y = 26 + (k / cols - vw.top) * gy;
        if (vw.mode == V_TOKENS)
            drawCardTex(vw.ids[k], 0, 0, x, y, cw, chh, 0);
        else {
            Inst *i = &S.in[vw.ids[k]];
            drawCardTex(i->def, i->face, 0, x, y, cw, chh, 0);
        }
        if (k == vw.cur) {
            frame(x - 2, y - 2, cw + 4, chh + 4, cAcc);
            frame(x - 3, y - 3, cw + 6, chh + 6, cAcc);
        }
    }
    if (!vw.n) textC(SW / 2, 110, "Vazio.", 13, 0, cMut);
    if (vw.top > 0) textC(SW / 2, 20, "▲", 9, 0, cMut);
    if ((vw.top + rows) * cols < vw.n) textC(SW / 2, 246, "▼", 9, 0, cMut);
    fill(0, SH - 22, SW, 22, cPanel);
    if (vw.n) {
        const char *nm = vw.mode == V_TOKENS ? defs[vw.ids[vw.cur]].f[0].n : faceOf(vw.ids[vw.cur])->n;
        fit(buf, sizeof buf, nm, 11, 1, 200);
        text(8, SH - 18, buf, 11, 1, cTxt);
    }
    const char *help = vw.mode == V_TOKENS ? "X cria · ▲ ver · L R página · O fecha" : "X mover · ▲ ver · L R página · O fecha";
    if (vw.mode == V_ZONE && vw.zone == Z_LIB && vw.limit == 0) help = vw.shuf ? "X mover · ▲ ver · L R letra · □ embaralhar: sim · O fecha" : "X mover · ▲ ver · L R letra · □ embaralhar: não · O fecha";
    int hw = textW(help, 9, 0);
    text(SW - hw - 8, SH - 16, help, 9, 0, cMut);
}

/* ===================== menus das cartas ===================== */

static void aTap(int id) {
    snap();
    S.in[id].tapped = !S.in[id].tapped;
}
static void aFace(int id) {
    snap();
    S.in[id].face = (S.in[id].face + 1) % defs[S.in[id].def].nf;
}
static void aDown(int id) {
    snap();
    S.in[id].down = !S.in[id].down;
}
static void aMoveZ(int id, int z, int top) {
    snap();
    const char *nm = faceOf(id)->n;
    char name[96];
    snprintf(name, sizeof name, "%s", S.in[id].down ? "Carta virada" : nm);
    moveTo(id, z, top);
    announce("%s → %s", name, z == Z_LIB ? (top ? "topo do grimório" : "fundo do grimório") : zoneNames[z]);
    net_log("%s → %s", name, zoneNames[z]);
}
static void aBF(int id) { aMoveZ(id, Z_BF, 0); }
static void aBFdown(int id) {
    aMoveZ(id, Z_BF, 0);
    S.in[id].down = 1;
}
static void aHand(int id) { aMoveZ(id, Z_HAND, 0); }
static void aGY(int id) { aMoveZ(id, Z_GY, 0); }
static void aEX(int id) { aMoveZ(id, Z_EX, 0); }
static void aTop(int id) { aMoveZ(id, Z_LIB, 1); }
static void aBottom(int id) { aMoveZ(id, Z_LIB, 0); }
static void aCmd(int id) { aMoveZ(id, Z_CMD, 0); }
static void aZoom(int id) { openZoom(S.in[id].def, S.in[id].face, id); }
/* equipar / anexar: escolhe a carta e depois a dona no campo */
static int equipSrc;
static void aEquip(int id) {
    equipSrc = id; /* a faixa embaixo do campo explica o que fazer */
}
static void doAttach(int src, int dst) {
    if (dst == src || !S.in[dst].used || S.in[dst].zone != Z_BF) return;
    if (hostOf(dst)) dst = hostOf(dst); /* escolheu uma carta presa: prende na dona dela */
    if (dst == src) return;
    snap();
    if (S.in[src].zone != Z_BF) moveTo(src, Z_BF, 0);
    for (int k = 1; k < MAXI; k++) /* o que estava preso na carta vai junto */
        if (S.in[k].used && S.in[k].att == src) S.in[k].att = dst;
    S.in[src].att = dst;
    char a[96];
    snprintf(a, sizeof a, "%s", faceOf(src)->n);
    announce("%s → presa em %s", a, faceOf(dst)->n);
    net_log("%s presa em %s", a, faceOf(dst)->n);
}
static void aDetach(int id) {
    snap();
    S.in[id].att = 0;
    announce("%s solta", faceOf(id)->n);
}
static void aDetachAll(int id) {
    snap();
    for (int k = 1; k < MAXI; k++)
        if (S.in[k].used && S.in[k].att == id) S.in[k].att = 0;
    announce("Cartas presas soltas");
}
static void aCopy(int id) {
    snap();
    int n = newInst(S.in[id].def, Z_BF, 1);
    if (n) {
        S.in[n].face = S.in[id].face;
        announce("Cópia (ficha) criada");
    }
}
static int lastAdj;
static void adjPlus(int id, int d) {
    if (lastAdj != (int)frameN) snap();
    lastAdj = frameN;
    S.in[id].plus += d;
}
static void adjCnt(int id, int d) {
    if (lastAdj != (int)frameN) snap();
    lastAdj = frameN;
    S.in[id].cnt += d;
    if (S.in[id].cnt < 0) S.in[id].cnt = 0;
}
static void adjTax(int id, int d) {
    snap();
    S.in[id].tax += d;
    if (S.in[id].tax < 0) S.in[id].tax = 0;
}

static void relabelCard(void) {
    Menu *m = &menus[nmenu - 1];
    for (int k = 0; k < m->n; k++) {
        MItem *it = &m->it[k];
        Inst *i = &S.in[it->arg];
        if (it->adj == adjPlus) {
            if (i->plus >= 0)
                snprintf(it->label, sizeof it->label, "Marcadores +1/+1: < %d >", i->plus);
            else
                snprintf(it->label, sizeof it->label, "Marcadores -1/-1: < %d >", -i->plus);
        } else if (it->adj == adjCnt)
            snprintf(it->label, sizeof it->label, "Outros marcadores: < %d >", i->cnt);
        else if (it->adj == adjTax)
            snprintf(it->label, sizeof it->label, "Imposto do comandante: < +%d >", i->tax * 2);
        else if (it->fn == aTap)
            snprintf(it->label, sizeof it->label, "%s", i->tapped ? "Desvirar" : "Virar (usar)");
    }
}

static void cardMenu(int id) {
    if (id <= 0 || !S.in[id].used) return;
    Inst *i = &S.in[id];
    char title[100];
    snprintf(title, sizeof title, "%s", i->down && i->zone == Z_BF ? "Carta virada para baixo" : faceOf(id)->n);
    Menu *m = openMenu(title);
    m->relabel = relabelCard;
    iSel = id;
    switch (i->zone) {
    case Z_HAND:
        mAdd(m, "Jogar no campo", aBF, id);
        mAdd(m, "Jogar prendendo em…", aEquip, id);
        mAdd(m, "Jogar virada para baixo", aBFdown, id);
        mAdd(m, "Descartar (cemitério)", aGY, id);
        mAdd(m, "Exilar", aEX, id);
        mAdd(m, "Topo do grimório", aTop, id);
        mAdd(m, "Fundo do grimório", aBottom, id);
        if (i->cmdr) mAdd(m, "Zona de comando", aCmd, id);
        mAdd(m, "Ver grande", aZoom, id);
        break;
    case Z_BF:
        mAdd(m, "Virar", aTap, id);
        mAdd(m, "Equipar / prender em…", aEquip, id);
        if (hostOf(id)) mAdd(m, "Soltar (desequipar)", aDetach, id);
        for (int k = 1; k < MAXI; k++)
            if (S.in[k].used && hostOf(k) == id) {
                mAdd(m, "Soltar tudo que está preso nela", aDetachAll, id);
                break;
            }
        if (defs[i->def].nf > 1) mAdd(m, "Transformar (outra face)", aFace, id);
        mAdd(m, i->down ? "Virar a face para cima" : "Virar a face para baixo", aDown, id);
        mAdd(m, "", NULL, id)->adj = adjPlus;
        mAdd(m, "", NULL, id)->adj = adjCnt;
        mAdd(m, "Criar uma cópia (ficha)", aCopy, id);
        mAdd(m, "Para a mão", aHand, id);
        mAdd(m, "Para o cemitério", aGY, id);
        mAdd(m, "Exilar", aEX, id);
        mAdd(m, "Topo do grimório", aTop, id);
        mAdd(m, "Fundo do grimório", aBottom, id);
        mAdd(m, "Zona de comando", aCmd, id);
        mAdd(m, "Ver grande", aZoom, id);
        break;
    case Z_CMD:
        mAdd(m, "Jogar no campo", aBF, id);
        if (i->cmdr) mAdd(m, "", NULL, id)->adj = adjTax;
        mAdd(m, "Para a mão", aHand, id);
        mAdd(m, "Ver grande", aZoom, id);
        break;
    default: /* cemitério, exílio, grimório (dentro da janela de ver) */
        mAdd(m, "Para a mão", aHand, id);
        mAdd(m, "Para o campo", aBF, id);
        if (i->zone != Z_GY) mAdd(m, "Para o cemitério", aGY, id);
        if (i->zone != Z_EX) mAdd(m, "Exilar", aEX, id);
        mAdd(m, "Topo do grimório", aTop, id);
        mAdd(m, "Fundo do grimório", aBottom, id);
        if (i->cmdr) mAdd(m, "Zona de comando", aCmd, id);
        mAdd(m, "Ver grande", aZoom, id);
        break;
    }
    for (int k = 0; k < m->n; k++)
        if (m->it[k].adj) m->it[k].keep = 1;
    relabelCard();
}

/* grimório e pilhas */
static void aDraw(int n) { drawCmd(n); }
static void aShuffle(int x) {
    (void)x;
    snap();
    shuffleLib();
    announce("Grimório embaralhado");
}
static void aMill(int x) {
    (void)x;
    if (!S.zn[Z_LIB]) return;
    aMoveZ(S.z[Z_LIB][0], Z_GY, 0);
}
static void aExTop(int x) {
    (void)x;
    if (!S.zn[Z_LIB]) return;
    aMoveZ(S.z[Z_LIB][0], Z_EX, 0);
}
static void aLook(int n) {
    char t[48];
    snprintf(t, sizeof t, n == 1 ? "A carta de cima" : "As %d de cima (a primeira é a do topo)", n);
    openViewer(V_ZONE, Z_LIB, n, t);
}
static void aSearch(int x) {
    (void)x;
    openViewer(V_ZONE, Z_LIB, 0, "Buscar no grimório");
}
static void aView(int z) { openViewer(V_ZONE, z, 0, z == Z_GY ? "Cemitério" : "Exílio"); }
static void aTokens(int x) {
    (void)x;
    openViewer(V_TOKENS, 0, 0, "Criar ficha");
}

/* ações para a pilha inteira (cemitério ou exílio); arg = zona*10 + ação */
static void aPileAll(int arg) {
    int z = arg / 10, a = arg % 10;
    if (!S.zn[z]) return;
    snap();
    short ids[MAXI];
    int n = S.zn[z];
    memcpy(ids, S.z[z], sizeof(short) * n);
    int to = a <= 1 ? Z_LIB : a == 2 ? Z_HAND : (z == Z_GY ? Z_EX : Z_GY);
    for (int k = 0; k < n; k++) moveTo(ids[k], to, 0);
    if (a == 0) shuffleLib();
    static const char *msg[4] = {"embaralhado no grimório", "no fundo do grimório", "voltou para a mão", ""};
    if (a == 3)
        announce(z == Z_GY ? "Cemitério exilado" : "Exílio foi para o cemitério");
    else
        announce("%s %s", z == Z_GY ? "Cemitério" : "Exílio", msg[a]);
    net_log("%s: %d cartas movidas", z == Z_GY ? "Cemitério" : "Exílio", n);
}
static void pileMenu(int z) {
    Menu *m;
    if (z == Z_LIB) {
        m = openMenu("Grimório");
        mAdd(m, "Comprar 1", aDraw, 1);
        mAdd(m, "Olhar a de cima", aLook, 1);
        mAdd(m, "Olhar as 3 de cima", aLook, 3);
        mAdd(m, "Buscar no grimório", aSearch, 0);
        mAdd(m, "Embaralhar", aShuffle, 0);
        mAdd(m, "Moer 1 (topo no cemitério)", aMill, 0);
        mAdd(m, "Exilar a de cima", aExTop, 0);
    } else {
        m = openMenu(z == Z_GY ? "Cemitério" : "Exílio");
        mAdd(m, "Ver as cartas", aView, z);
        if (S.zn[z]) {
            mAdd(m, "Todas: embaralhar no grimório", aPileAll, z * 10 + 0);
            mAdd(m, "Todas: fundo do grimório", aPileAll, z * 10 + 1);
            mAdd(m, "Todas: para a mão", aPileAll, z * 10 + 2);
            mAdd(m, z == Z_GY ? "Todas: exilar o cemitério" : "Todas: para o cemitério", aPileAll, z * 10 + 3);
        }
    }
}

/* menu geral (START) */
static int wantNew, wantDecks;
static void adjLife(int x, int d) {
    (void)x;
    if (lastAdj != (int)frameN) snap();
    lastAdj = frameN;
    S.life += d;
}
static void adjPoison(int x, int d) {
    (void)x;
    snap();
    S.poison += d;
    if (S.poison < 0) S.poison = 0;
}
static void cfgSave(void);
static void adjBar(int x, int d) {
    (void)x, (void)d;
    showBar = !showBar;
    cfgSave();
}
static void aBar(int x) { adjBar(x, 1); }
static void adjInfo(int x, int d) {
    (void)x, (void)d;
    infoPanel = !infoPanel;
    cfgSave();
}
static void aInfo(int x) { adjInfo(x, 1); }
static void adjCpu(int x, int d) {
    (void)x, (void)d;
    cpuMax = !cpuMax;
    cpuSpeed(0);
    cfgSave();
}
static void aCpu(int x) { adjCpu(x, 1); }
static void relabelMain(void) {
    Menu *m = &menus[nmenu - 1];
    for (int k = 0; k < m->n; k++) {
        if (m->it[k].adj == adjCpu) snprintf(m->it[k].label, sizeof m->it[k].label, "Processador: < %s >", cpuMax ? "333 MHz, rápido" : "222 MHz, poupa bateria");
        if (m->it[k].adj == adjInfo) snprintf(m->it[k].label, sizeof m->it[k].label, "Painel da carta à esquerda: < %s >", infoPanel ? "sim" : "não");
        if (m->it[k].adj == adjBar) snprintf(m->it[k].label, sizeof m->it[k].label, "Nome e custo nas cartas: < %s >", showBar ? "sim" : "não");
        if (m->it[k].adj == adjLife) snprintf(m->it[k].label, sizeof m->it[k].label, "Vida: < %d >", S.life);
        if (m->it[k].adj == adjPoison) snprintf(m->it[k].label, sizeof m->it[k].label, "Veneno: < %d >", S.poison);
    }
}
static void aNextStep(int x) {
    (void)x;
    nextStep();
}
static void aNextTurn(int x) {
    (void)x;
    nextTurn();
}
static void aUntap(int x) {
    (void)x;
    snap();
    untapAll();
    announce("Desvirou tudo");
}
static void aUndo(int x) {
    (void)x;
    undoLast();
}
static void aNew(int x) { wantNew = 1 + x; }
static void aDecks(int x) {
    (void)x;
    wantDecks = 1;
}
static void aQuit(int x) {
    (void)x;
    running = 0;
}
static int helpOn, helpScroll;
static void helpOpen(void);
static void aHelp(int x) {
    (void)x;
    helpOpen();
}
static void aNet(int x) {
    (void)x;
    net_menu();
}

/* histórico: o jogador registra o resultado pelo menu START (historico.txt ao lado do EBOOT) */
static void aRecord(int r) {
    static const char *res[3] = {"Venci", "Perdi", "Empate"};
    if (gameSaved) {
        announce("Esta partida já foi registrada");
        return;
    }
    FILE *f = fopen("historico.txt", "a");
    if (!f) {
        announce("Não consegui gravar o histórico");
        return;
    }
    time_t now = time(NULL);
    struct tm *t = localtime(&now);
    int mins = gameStart ? (int)((now - gameStart + 30) / 60) : 0;
    fprintf(f, "%02d/%02d %02d:%02d\t%s\t%s\t%d\t%d\n", t->tm_mday, t->tm_mon + 1, t->tm_hour, t->tm_min, deckName, res[r], S.turn, mins);
    fclose(f);
    gameSaved = 1;
    announce("Partida registrada: %s", res[r]);
}
static void aHistory(int x) {
    (void)x;
    Menu *m = openMenu("Resultado da partida");
    char b[80];
    for (int r = 0; r < 3; r++) {
        snprintf(b, sizeof b, "%s (turno %d)", r == 0 ? "Venci" : r == 1 ? "Perdi" : "Empate", S.turn);
        mAdd(m, b, aRecord, r);
    }
    /* últimas partidas (só para ler) */
    char lines[8][200];
    int n = 0, wins = 0, tot = 0;
    FILE *f = fopen("historico.txt", "r");
    char line[200];
    while (f && fgets(line, sizeof line, f)) {
        line[strcspn(line, "\r\n")] = 0;
        snprintf(lines[n % 8], 200, "%s", line);
        n++;
        char *p = strchr(line, '\t');
        char *q = p ? strchr(p + 1, '\t') : NULL;
        if (q && !strncmp(p + 1, deckName, q - p - 1) && (int)strlen(deckName) == q - p - 1) {
            tot++;
            if (!strncmp(q + 1, "Venci", 5)) wins++;
        }
    }
    if (f) fclose(f);
    if (tot) {
        snprintf(b, sizeof b, "Este deck: %d de %d vitórias", wins, tot);
        mAdd(m, b, NULL, 0);
    }
    for (int k = n - 1; k >= 0 && k >= n - 6; k--) {
        char *c[5] = {lines[k % 8]};
        for (int j = 1; j < 5; j++) {
            c[j] = c[j - 1] ? strchr(c[j - 1], '\t') : NULL;
            if (c[j]) *c[j]++ = 0;
        }
        if (!c[4]) continue;
        snprintf(b, sizeof b, "%s · %s · %s · turno %s · %s min", c[0], c[2], c[1], c[3], c[4]);
        mAdd(m, b, NULL, 0);
    }
}

/* confirmação antes do que perde a partida em andamento */
static void aNo(int x) { (void)x; }
static void askSure(const char *q, const char *yes, void (*fn)(int)) {
    Menu *m = openMenu(q);
    mAdd(m, "Não, voltar", aNo, 0);
    mAdd(m, yes, fn, 0);
}
static void cNew(int x) { (void)x; askSure("Começar outra partida? A atual se perde.", "Sim, nova partida", aNew); }
static void cDecks(int x) { (void)x; askSure("Trocar de deck? A partida atual se perde.", "Sim, trocar de deck", aDecks); }
static void cQuit(int x) { (void)x; askSure("Sair do Mesa Arcana?", "Sim, sair", aQuit); }
static void mainMenu(void) {
    Menu *m = openMenu("Mesa Arcana");
    m->relabel = relabelMain;
    mAdd(m, "Próxima fase (SELECT)", aNextStep, 0);
    mAdd(m, "Próximo turno (desvira tudo, não compra)", aNextTurn, 0);
    mAdd(m, "Comprar 1 (□)", aDraw, 1);
    mAdd(m, "Desvirar tudo", aUntap, 0);
    mAdd(m, "Criar ficha", aTokens, 0);
    mAdd(m, "", NULL, 0)->adj = adjLife;
    mAdd(m, "", NULL, 0)->adj = adjPoison;
    mAdd(m, "Desfazer (L)", aUndo, 0);
    mAdd(m, "Embaralhar o grimório", aShuffle, 0);
    mAdd(m, "Jogar em rede (Wi-Fi entre PSPs)", aNet, 0);
    mAdd(m, "Resultado e histórico", aHistory, 0);
    mAdd(m, "Atalhos e controles (R + SELECT)", aHelp, 0);
    mAdd(m, "", aInfo, 0)->adj = adjInfo;
    mAdd(m, "", aBar, 0)->adj = adjBar;
    mAdd(m, "", aCpu, 0)->adj = adjCpu;
    mAdd(m, "Nova partida (mesmo deck)", cNew, 0);
    mAdd(m, "Trocar de deck", cDecks, 0);
    mAdd(m, "Sair", cQuit, 0);
    for (int k = 0; k < m->n; k++)
        if (m->it[k].adj) m->it[k].keep = 1;
    relabelMain();
}

/* lista de todos os atalhos: START > Atalhos, ou R + SELECT em qualquer tela.
   Uma página por tela; abre na página da tela em que você está. */
typedef struct {
    const char *tab, *title;
    const char *l[16]; /* "teclas|o que faz"; teclas separadas por espaço */
} HelpPage;
static const HelpPage helpPages[] = {
    {"Mesa", "Na mesa", {
        "DPAD|escolhe a carta ou a pilha (fica com borda dourada)",
        "X|joga da mão ou do comando; vira/desvira no campo",
        "O|menu da carta: mover, equipar/prender, marcadores, transformar, cópia",
        "TRI|carta grande, com o texto",
        "SQ|compra 1 carta",
        "SELECT|próxima fase (só anunciada)",
        "START|menu geral (turno, fichas, vida, rede, histórico…)",
        "L|desfaz a última ação",
        "R + ↑ ↓|vida +1 / -1",
        "R + ← →|vida -5 / +5",
        "R + SELECT|esta lista de atalhos",
        "R + SQ|esconde/mostra o painel da carta (à esquerda)",
        "ANALOG|rola o texto do painel da carta",
    }},
    {"Pilhas", "Pilhas (coluna da direita)", {
        "X|grimório: compra 1",
        "O|grimório: comprar, olhar 1 ou 3, buscar, embaralhar, moer, exilar a de cima",
        "X|cemitério ou exílio: ver as cartas",
        "O|cemitério ou exílio: menu da pilha inteira (embaralhar, fundo, mão, exilar)",
        "TRI|comando: carta grande do comandante",
    }},
    {"Listas", "Listas (grimório, cemitério, exílio, fichas)", {
        "DPAD|anda pela grade",
        "X|move a carta (ou cria a ficha)",
        "TRI|carta grande",
        "L R|pulam uma página; na busca do grimório, pulam de letra",
        "SQ|na busca: embaralhar ao fechar, sim ou não",
        "O|fecha",
    }},
    {"Carta", "Carta grande", {
        "↑ ↓|rola o texto",
        "← →|outra face (cartas de duas faces)",
        "TRI ou O|fecha",
    }},
    {"Menus", "Menus", {
        "↑ ↓|escolhe",
        "← →|muda valores (vida, veneno, marcadores, opções)",
        "X|confirma",
        "O|volta",
    }},
    {"Início", "Início da partida", {
        "X|fica com a mão",
        "O|mulligan (o primeiro é grátis)",
        "X|depois do mulligan: marca ou desmarca a carta que vai para o fundo",
        "START|confirma as cartas do fundo",
        "TRI|carta grande",
    }},
    {"Rede", "Rede entre PSPs", {
        "R + L|alterna entre a sua mesa e a dos amigos",
        "X ou TRI|na mesa do amigo: carta grande",
        "START|na lista de decks: menu da rede (sala, entrar, sair)",
        "START|na partida: \"Jogar em rede\" no menu geral",
    }},
    {"Decks", "Lista de decks", {
        "↑ ↓|escolhe o deck",
        "X|joga",
        "START|menu da rede",
        "SELECT|sai do programa",
    }},
};
enum { NHELP = sizeof helpPages / sizeof *helpPages };
static int helpPage;

/* abre na página da tela atual */
static int helpPageFor(void);

/* desenha um botão do PSP como ícone pequeno; devolve a largura */
static int drawKey(int x, int y, const char *k) {
    SDL_Color dark = {22, 26, 30, 255}, rim = {70, 80, 88, 255};
    int cy = y + 7;
    if (!strcmp(k, "X") || !strcmp(k, "O") || !strcmp(k, "TRI") || !strcmp(k, "SQ")) {
        SDL_Color c = !strcmp(k, "X") ? (SDL_Color){126, 170, 232, 255} : !strcmp(k, "O") ? (SDL_Color){232, 120, 110, 255}
                    : !strcmp(k, "TRI") ? (SDL_Color){110, 204, 160, 255} : (SDL_Color){214, 146, 204, 255};
        int cx = x + 7;
        disc(cx, cy, 7, rim);
        disc(cx, cy, 6, dark);
        SDL_SetRenderDrawColor(R, c.r, c.g, c.b, 255);
        if (!strcmp(k, "X")) {
            SDL_RenderDrawLine(R, cx - 3, cy - 3, cx + 3, cy + 3), SDL_RenderDrawLine(R, cx - 3, cy + 3, cx + 3, cy - 3);
            SDL_RenderDrawLine(R, cx - 2, cy - 3, cx + 3, cy + 2), SDL_RenderDrawLine(R, cx - 3, cy + 2, cx + 2, cy - 3);
        } else if (!strcmp(k, "O")) {
            disc(cx, cy, 4, c);
            disc(cx, cy, 2, dark);
        } else if (!strcmp(k, "TRI")) {
            SDL_RenderDrawLine(R, cx, cy - 4, cx - 4, cy + 3), SDL_RenderDrawLine(R, cx, cy - 4, cx + 4, cy + 3);
            SDL_RenderDrawLine(R, cx - 4, cy + 3, cx + 4, cy + 3);
            SDL_RenderDrawLine(R, cx, cy - 3, cx - 3, cy + 2), SDL_RenderDrawLine(R, cx, cy - 3, cx + 3, cy + 2);
        } else {
            frame(cx - 3, cy - 3, 7, 7, c);
            frame(cx - 2, cy - 2, 5, 5, c);
        }
        return 15;
    }
    if (!strcmp(k, "ANALOG")) { /* alavanca analógica */
        disc(x + 7, cy, 7, rim);
        disc(x + 7, cy, 5, (SDL_Color){120, 130, 138, 255});
        disc(x + 7, cy, 3, (SDL_Color){150, 160, 168, 255});
        return 15;
    }
    if (!strcmp(k, "DPAD")) { /* cruz dos direcionais */
        fill(x + 4, y, 6, 14, rim), fill(x, y + 4, 14, 6, rim);
        fill(x + 5, y + 1, 4, 12, dark), fill(x + 1, y + 5, 12, 4, dark);
        fill(x + 6, y + 6, 2, 2, cMut);
        return 15;
    }
    if (!strcmp(k, "+") || !strcmp(k, "ou")) return text(x + 1, y, k, 10, 0, cMut) + 2;
    int sz = strlen(k) > 3 ? 8 : 10, tw = textW(k, sz, 1), w = tw + (strlen(k) > 3 ? 10 : 8);
    if (w < 14) w = 14;
    fill(x, y, w, 14, rim);
    fill(x + 1, y + 1, w - 2, 12, dark);
    text(x + (w - tw) / 2, y + (sz == 8 ? 1 : 0), k, sz, 1, cTxt);
    return w;
}
static void helpOpen(void) {
    helpOn = 1, helpScroll = 0, helpPage = helpPageFor();
}
static void helpInput(void) {
    if (held[B_DOWN] || pressed[B_DOWN]) helpScroll += 4;
    if (held[B_UP] || pressed[B_UP]) helpScroll -= 4;
    if (pressed[B_RIGHT] || pressed[B_R]) helpPage = (helpPage + 1) % NHELP, helpScroll = 0;
    if (pressed[B_LEFT] || pressed[B_L]) helpPage = (helpPage + NHELP - 1) % NHELP, helpScroll = 0;
    if (helpScroll < 0) helpScroll = 0;
    if (pressed[B_O] || pressed[B_X] || pressed[B_START] || pressed[B_TRI] || pressed[B_SELECT]) helpOn = 0;
}
static void drawHelp(void) {
    fill(0, 0, SW, SH, cBg);
    const HelpPage *p = &helpPages[helpPage];
    char t[96];
    /* conteúdo primeiro; as barras de cima e de baixo são desenhadas por cima (sem recorte, que alguns PSPs não respeitam) */
    int y = 30 - helpScroll;
    if (y > 4) text(12, y, p->title, 13, 1, cAcc2);
    y += 22;
    for (int k = 0; k < 16 && p->l[k]; k++) {
        const char *bar = strchr(p->l[k], '|');
        int h = textW(bar + 1, 11, 0) > SW - 140 ? 28 : 15; /* duas linhas quando não cabe */
        if (y + h > 10 && y < SH - 19) {
            char keys[48];
            int n = (int)(bar - p->l[k]) < 47 ? (int)(bar - p->l[k]) : 47;
            memcpy(keys, p->l[k], n), keys[n] = 0;
            if (k % 2) fill(6, y - 2, SW - 12, h + 3, (SDL_Color){255, 255, 255, 8});
            int kx = 14;
            for (char *tok = strtok(keys, " "); tok; tok = strtok(NULL, " ")) kx += drawKey(kx, y, tok) + 3;
            textWrap(128, y, SW - 140, bar + 1, 11, 0, cTxt);
        }
        y += h + 3;
    }
    int maxS = y + helpScroll - (SH - 22);
    if (maxS < 0) maxS = 0;
    if (helpScroll > maxS) helpScroll = maxS;
    /* abas */
    fill(0, 0, SW, 22, cPanel);
    int tx = 6;
    for (int k = 0; k < NHELP; k++) {
        int w = textW(helpPages[k].tab, 10, k == helpPage) + 12;
        if (k == helpPage) {
            fill(tx, 3, w, 17, cPanel2);
            fill(tx, 18, w, 2, cAcc);
        }
        text(tx + 6, 5, helpPages[k].tab, 10, k == helpPage, k == helpPage ? cAcc : cMut);
        tx += w + 2;
    }
    fill(0, SH - 19, SW, 19, cPanel);
    snprintf(t, sizeof t, "%d/%d  ·  ← → ou L R: página  ·  ↑ ↓ rola  ·  O fecha", helpPage + 1, (int)NHELP);
    text(8, SH - 16, t, 9, 0, cMut);
    snprintf(t, sizeof t, "arquivos lidos: %d", nOpen);
    text(SW - 8 - textW(t, 9, 0), SH - 16, t, 9, 0, cMut);
}

/* ===================== tela da mesa ===================== */

/* campo: começa depois do painel da carta (se ligado) e vai até a coluna das pilhas */
#define PILEX 424
#define INFOW 140
static int BFX = 0, BFW = PILEX;
#define BFY 18
#define BFH 180
#define HANDY 200

static void drawCardAt(int id, int x, int y, int w, int h, int zone) {
    Inst *i = &S.in[id];
    int tapped = i->tapped && zone == Z_BF;
    int down = i->down && zone == Z_BF;
    drawCardTex(i->def, i->face, down, x, y, w, h, tapped);
    int fw = tapped ? h : w, fh = tapped ? w : h;
    int by = y + fh - 13;
    char b[24];
    if (i->plus) {
        snprintf(b, sizeof b, "%+d", i->plus);
        badge(x + 1, by, b, (SDL_Color){0, 0, 0, 220}, i->plus > 0 ? cAcc : cBad);
        by -= 13;
    }
    if (i->cnt) {
        snprintf(b, sizeof b, "●%d", i->cnt);
        badge(x + 1, by, b, (SDL_Color){0, 0, 0, 220}, cAcc2);
    }
    if (i->token && zone == Z_BF) badge(x + fw - 12, y + 1, "F", (SDL_Color){0, 0, 0, 200}, cAcc2);
    if (zone == Z_HAND) { /* mão: P/R (ou lealdade) bem visível no canto de baixo */
        const Face *f = faceOf(id);
        if (*f->p || *f->o)
            snprintf(b, sizeof b, "%s/%s", f->p, f->o);
        else if (*f->l)
            snprintf(b, sizeof b, "L%s", f->l);
        else
            b[0] = 0;
        if (*b) {
            int bw = textW(b, 9, 1) + 6; /* canto de baixo à esquerda: a carta seguinte da mão cobre a direita */
            fill(x + 1, y + fh - 13, bw, 12, (SDL_Color){0, 0, 0, 220});
            frame(x + 1, y + fh - 13, bw, 12, cAcc2);
            text(x + 4, y + fh - 14, b, 9, 1, cTxt);
        }
    }
    if (i->cmdr && i->tax && zone == Z_CMD) {
        snprintf(b, sizeof b, "+%d", i->tax * 2);
        badge(x + 1, y + 1, b, (SDL_Color){0, 0, 0, 220}, cAcc);
    }
    if (zone == Z_HAND && S.phase == PH_BOTTOM)
        for (int k = 0; k < S.npick; k++)
            if (S.pick[k] == id) {
                fill(x, y, w, h, (SDL_Color){0, 0, 0, 150});
                textC(x + w / 2, y + h / 2 - 7, "FUNDO", 9, 1, cAcc);
            }
    if (isSel(K_CARD, zone, id)) {
        frame(x - 2, y - 2, fw + 4, fh + 4, cAcc);
        frame(x - 3, y - 3, fw + 6, fh + 6, cAcc);
    }
    addSel(x, y, fw, fh, K_CARD, zone, id);
}

/* com a rede ligada sobra uma faixa no alto do campo para os jogadores */
void bfGeom(int *top, int *rowH, int *cw, int *ch) {
    *top = BFY + (net_on() ? 14 : 0);
    *rowH = (BFY + BFH - *top) / 3;
    *ch = *rowH - 3;
    *cw = *ch * 5 / 7;
}
#define ATT_PEEK 12 /* quanto de cada carta presa aparece ao lado da dona */
static void drawBattlefield(void) {
    fill(BFX, BFY, BFW, BFH, cFelt);
    int top, rowH, cw, ch;
    bfGeom(&top, &rowH, &cw, &ch);
    /* um toque de mesa: linhas douradas finas entre as faixas */
    for (int r = 1; r < 3; r++) fill(BFX + 6, top + r * rowH, BFW - 12, 1, (SDL_Color){232, 185, 74, 60});
    static const char *rn[3] = {"CRIATURAS", "OUTRAS", "TERRENOS"};
    for (int r = 0; r < 3; r++) {
        short ids[MAXI];
        int n = 0, tot = 0;
        short natt[MAXI] = {0};
        for (int k = 0; k < S.zn[Z_BF]; k++) {
            int h = hostOf(S.z[Z_BF][k]);
            if (h) natt[h]++;
        }
        for (int k = 0; k < S.zn[Z_BF]; k++) {
            int id = S.z[Z_BF][k];
            if (!hostOf(id) && rowOf(id) == r) {
                ids[n++] = id;
                tot += (S.in[id].tapped ? ch : cw) + 3 + natt[id] * ATT_PEEK;
            }
        }
        int y = top + r * rowH + 2;
        if (!n) {
            textC(BFX + BFW / 2, y + 22, rn[r], 9, 1, (SDL_Color){255, 255, 255, 50});
            continue;
        }
        float k = 1;
        if (tot > BFW - 8) k = (float)(BFW - 8 - ch) / (tot - (S.in[ids[n - 1]].tapped ? ch : cw) - 3);
        float x = BFX + 4;
        for (int j = 0; j < n; j++) {
            int id = ids[j], t = S.in[id].tapped, hw = t ? ch : cw;
            /* as presas ficam atrás, aparecendo uma tira de cada à direita */
            int on = 0, selA = 0, selAx = 0;
            for (int q = 0; q < S.zn[Z_BF]; q++) {
                int a = S.z[Z_BF][q];
                if (hostOf(a) != id) continue;
                on++;
                int ax = (int)x + hw - cw + on * ATT_PEEK;
                drawCardAt(a, ax, y, cw, ch, Z_BF);
                if (isSel(K_CARD, Z_BF, a)) selA = a, selAx = ax;
            }
            drawCardAt(id, (int)x, t ? y + (ch - cw) : y, cw, ch, Z_BF);
            if (on) { /* elos: tem carta presa */
                int lx = (int)x + 1, ly = (t ? y + (ch - cw) : y) + (t ? cw : ch) / 2 - 6;
                fill(lx, ly, 15, 12, (SDL_Color){0, 0, 0, 210});
                frame(lx + 2, ly + 3, 7, 6, cAcc);
                frame(lx + 6, ly + 3, 7, 6, cAcc);
            }
            if (selA) drawCardAt(selA, selAx, y, cw, ch, Z_BF); /* a escolhida aparece por cima */
            x += (hw + 3 + on * ATT_PEEK) * k;
        }
    }
}

static void drawHand(void) {
    fill(BFX, HANDY - 2, BFW, SH - HANDY + 2, cPanel);
    const int cw = 48, ch = 67;
    int n = S.zn[Z_HAND];
    float step = cw + 4;
    if (n > 1 && step * (n - 1) + cw > BFW - 10) step = (float)(BFW - 10 - cw) / (n - 1);
    int selK = -1;
    for (int k = 0; k < n; k++) {
        int id = S.z[Z_HAND][k];
        if (isSel(K_CARD, Z_HAND, id)) {
            selK = k;
            continue;
        }
        drawCardAt(id, BFX + 5 + (int)(step * k), HANDY + 2, cw, ch, Z_HAND);
    }
    if (selK >= 0) /* a escolhida sobe e fica por cima das vizinhas */
        drawCardAt(S.z[Z_HAND][selK], BFX + 5 + (int)(step * selK), HANDY - 6, cw, ch, Z_HAND);
    if (!n) textC(BFX + BFW / 2, HANDY + 26, "mão vazia", 10, 0, cMut);
}

static void drawPiles(void) {
    int x = PILEX, w = SW - PILEX;
    fill(x, BFY, w, SH - BFY, cPanel);
    fill(x, BFY, 1, SH - BFY, cLine);
    const int cw = 38, ch = 53;
    const char *lab[4] = {"COMANDO", "GRIMÓRIO", "CEMITÉRIO", "EXÍLIO"};
    int zs[4] = {Z_CMD, Z_LIB, Z_GY, Z_EX};
    for (int k = 0; k < 4; k++) {
        int y = BFY + 2 + k * 63, z = zs[k];
        char b[24];
        text(x + 4, y, lab[k], 7, 1, cMut);
        if (z != Z_CMD) {
            snprintf(b, sizeof b, "%d", S.zn[z]);
            text(x + w - 4 - textW(b, 9, 1), y - 1, b, 9, 1, cTxt);
        }
        int cx = x + (w - cw) / 2, cy = y + 9;
        frame(cx, cy, cw, ch, cLine);
        if (z == Z_CMD) {
            for (int j = 0; j < S.zn[Z_CMD] && j < 2; j++) drawCardAt(S.z[Z_CMD][j], cx + j * 4, cy + j * 4, cw, ch, Z_CMD);
            if (!S.zn[Z_CMD]) addSel(cx, cy, cw, ch, K_PILE, Z_CMD, 0);
            continue;
        }
        if (S.zn[z]) {
            if (z == Z_LIB)
                drawBack(cx, cy, cw, ch);
            else {
                Inst *i = &S.in[S.z[z][S.zn[z] - 1]];
                drawCardTex(i->def, i->face, 0, cx, cy, cw, ch, 0);
            }
        }
        if (isSel(K_PILE, z, 0)) {
            frame(cx - 2, cy - 2, cw + 4, ch + 4, cAcc);
            frame(cx - 3, cy - 3, cw + 6, ch + 6, cAcc);
        }
        addSel(cx, cy, cw, ch, K_PILE, z, 0);
    }
}

/* bateria e relógio no canto (PSP de verdade); devolve a largura usada */
static int drawStatus(int xr, int y) {
    /* lido a cada 10 s, não a cada quadro */
    static char b[40];
    static unsigned last;
    static int pct = -1, chg;
    if (!*b || frameN - last >= 600) {
        last = frameN;
        time_t now = time(NULL);
        struct tm *t = localtime(&now);
#ifdef __PSP__
        pct = scePowerGetBatteryLifePercent();
        chg = scePowerIsBatteryCharging();
#endif
        if (pct >= 0)
            snprintf(b, sizeof b, "%s%d%%  %02d:%02d", chg ? "+" : "", pct, t->tm_hour, t->tm_min);
        else
            snprintf(b, sizeof b, "%02d:%02d", t->tm_hour, t->tm_min);
    }
    int w = textW(b, 9, 0);
    text(xr - w, y, b, 9, 0, pct >= 0 && pct <= 15 && !chg ? cBad : cMut);
    return w + 6;
}
static void drawTop(void) {
    fill(0, 0, SW, BFY, cPanel);
    fill(0, BFY - 1, SW, 1, cLine);
    char b[160];
    snprintf(b, sizeof b, "Vida %d", S.life);
    int x = 6 + text(6, 2, b, 12, 1, cAcc2) + 8;
    if (S.poison) {
        snprintf(b, sizeof b, "veneno %d", S.poison);
        x += text(x, 3, b, 10, 1, cBad) + 8;
    }
    snprintf(b, sizeof b, "T%d", S.turn);
    x += text(x, 3, b, 11, 1, cTxt) + 6;
    const char *ph = S.phase == PH_MULL ? "Mão inicial" : S.phase == PH_BOTTOM ? "Fundo do grimório" : stepNames[S.step];
    int pw = textW(ph, 10, 1) + 10;
    fill(x, 2, pw, 13, cAcc);
    text(x + 5, 2, ph, 10, 1, cInk);
    x += pw + 8;
    /* nome da carta escolhida */
    const char *nm = NULL;
    if (selKind == K_CARD && selId > 0 && S.in[selId].used) nm = (S.in[selId].down && S.in[selId].zone == Z_BF) ? "Carta virada" : faceOf(selId)->n;
    if (selKind == K_PILE) nm = selZone == Z_LIB ? "Grimório: X compra · O menu" : selZone == Z_GY ? "Cemitério: X abre" : selZone == Z_EX ? "Exílio: X abre" : "Zona de comando";
    int sw = drawStatus(SW - 4, 4);
    if (nm) {
        /* carta escolhida: nome e, logo depois, o custo */
        const char *cost = (selKind == K_CARD && selId > 0 && S.in[selId].used && !(S.in[selId].down && S.in[selId].zone == Z_BF)) ? faceOf(selId)->c : "";
        int nsym = 0;
        for (const char *p = cost; *p; p++) nsym += *p == '{';
        int cw = nsym ? nsym * 12 + 6 : 0;
        fit(b, sizeof b, nm, 10, 0, SW - x - 6 - sw - cw);
        int tw = text(x, 3, b, 10, 0, cTxt);
        if (nsym) drawCost(x + tw + 6 + nsym * 12, 3, cost, 11);
    }
}

/* painel da carta escolhida (à esquerda): imagem pequena, nome, custo, tipo, P/R e o texto em letra maior */
static void drawInfo(void) {
    int x = 0, y = BFY, w = INFOW, h = SH - BFY;
    fill(x, y, w, h, cPanel);
    fill(x + w - 1, y, 1, h, cLine);
    const Face *f = NULL;
    SDL_Texture *img = NULL;
    int id = 0;
    if (selKind == K_CARD && selId > 0 && S.in[selId].used) {
        id = selId;
        if (S.in[id].down && S.in[id].zone == Z_BF) {
            drawBack(x + 6, y + 6, 54, 75);
            text(x + 66, y + 8, "Virada para baixo", 9, 1, cMut);
            textWrap(x + 6, y + 90, w - 12, "▲ ou O > Ver grande mostra a carta para você.", 9, 0, cMut);
            return;
        }
        f = faceOf(id);
        Def *d = &defs[S.in[id].def];
        img = d->sm[S.in[id].face < d->nf ? S.in[id].face : 0];
    } else if (selKind == K_PILE) {
        static const char *nm[NZ] = {"Grimório", "Mão", "Campo", "Cemitério", "Exílio", "Zona de comando"};
        char b[64];
        text(x + 8, y + 8, nm[selZone], 12, 1, cAcc);
        snprintf(b, sizeof b, "%d cartas", S.zn[selZone]);
        text(x + 8, y + 26, b, 10, 0, cTxt);
        textWrap(x + 8, y + 46, w - 16, selZone == Z_LIB ? "X compra 1\nO comprar, olhar, buscar, embaralhar…" : selZone == Z_CMD ? "X joga o comandante\nO menu" : "X ver as cartas\nO menu da pilha inteira", 9, 0, cMut);
        return;
    }
    if (!f) {
        textWrap(x + 8, y + 10, w - 16, "Escolha uma carta para ver o texto aqui.", 10, 0, cMut);
        return;
    }
    /* topo: imagem e, ao lado, nome, custo e P/R */
    int iw = 54, ih = 75, tx = x + 6 + iw + 6, tw = w - (tx - x) - 5;
    if (img) {
        SDL_Rect r = {x + 6, y + 6, iw, ih};
        SDL_RenderCopy(R, img, NULL, &r);
    } else {
        drawPaper(f, x + 6, y + 6, iw, ih);
        nameBar(f, x + 6, y + 6, iw);
    }
    int ty = y + 5;
    ty += textWrap(tx, ty, tw, f->n, 10, 1, cAcc) + 2;
    if (f->c && *f->c) {
        int n = 0;
        for (const char *p = f->c; *p; p++) n += *p == '{';
        int d = n > 5 ? 9 : 11;
        drawCost(tx + n * (d + 1), ty, f->c, d);
        ty += d + 4;
    }
    char b[40];
    if (*f->p || *f->o) {
        snprintf(b, sizeof b, "%s/%s", f->p, f->o);
        Inst *i = &S.in[id];
        int bw = textW(b, 13, 1) + 10;
        fill(tx, ty, bw, 17, (SDL_Color){0, 0, 0, 200});
        frame(tx, ty, bw, 17, cAcc2);
        text(tx + 5, ty, b, 13, 1, cTxt);
        if (i->plus) {
            snprintf(b, sizeof b, "%+d", i->plus);
            text(tx + bw + 4, ty + 2, b, 10, 1, i->plus > 0 ? cAcc : cBad);
        }
    } else if (*f->l) {
        snprintf(b, sizeof b, "Lealdade %s", f->l);
        text(tx, ty, b, 10, 1, cTxt);
    }
    /* tipo e texto */
    int by = y + 6 + ih + 5;
    by += textWrap(x + 6, by, w - 12, f->t, 9, 1, cAcc2) + 3;
    fill(x + 6, by, w - 12, 1, cLine);
    by += 4;
    int avail = SH - 4 - by;
    if (!f->x || !*f->x || avail < 12) return;
    /* a maior letra em que o texto cabe (11 ou 10); se não cabe, fica em 10 e a alavanca rola */
    static int lastKind, lastId;
    if (lastKind != selKind || lastId != selId) lastKind = selKind, lastId = selId, infoScroll = 0;
    TC *t = textTex(f->x, 11, 0, cTxt, w - 12);
    if (t && t->hh > avail) t = textTex(f->x, 10, 0, cTxt, w - 12);
    if (!t) return;
    int maxS = t->hh - avail;
    if (maxS < 0) maxS = 0;
    if (infoScroll > maxS) infoScroll = maxS;
    if (infoScroll < 0) infoScroll = 0;
    int hh = t->hh - infoScroll < avail ? t->hh - infoScroll : avail;
    SDL_Rect src = {0, infoScroll, t->w, hh}, dst = {x + 6, by, t->w, hh};
    SDL_RenderCopy(R, t->t, &src, &dst);
    if (maxS > 0) { /* barrinha de rolagem */
        int bh = avail * avail / t->hh, byy = by + (avail - bh) * infoScroll / maxS;
        fill(x + w - 5, by, 2, avail, cPanel2);
        fill(x + w - 5, byy, 2, bh, cAcc);
    }
}

static void drawPrompt(void) {
    char b[160];
    if (equipSrc) {
        char n[60];
        fit(n, sizeof n, faceOf(equipSrc)->n, 10, 1, 170);
        snprintf(b, sizeof b, "Prender %s em… · X prende · O cancela", n);
        int w = textW(b, 10, 1) + 16;
        fill(BFX + (BFW - w) / 2, BFY + BFH - 22, w, 20, (SDL_Color){0, 0, 0, 220});
        frame(BFX + (BFW - w) / 2, BFY + BFH - 22, w, 20, cAcc);
        textC(BFX + BFW / 2, BFY + BFH - 19, b, 10, 1, cAcc);
        return;
    }
    if (S.phase == PH_PLAY) return;
    if (S.phase == PH_MULL)
        snprintf(b, sizeof b, S.mulls ? "Mulligan %d · X mantém · O faz outro mulligan" : "Mão inicial · X mantém · O faz mulligan", S.mulls);
    else
        snprintf(b, sizeof b, "Marque %d carta(s) com X para o fundo · START confirma (%d/%d)", bottomCount(), S.npick, bottomCount());
    int w = textW(b, 11, 1) + 20;
    if (w > BFW - 4) w = BFW - 4;
    fill(BFX + (BFW - w) / 2, 80, w, 24, (SDL_Color){0, 0, 0, 210});
    frame(BFX + (BFW - w) / 2, 80, w, 24, cAcc);
    textC(BFX + BFW / 2, 85, b, 11, 1, cAcc);
}

static void gameInput(void) {
    if (held[B_R] && pressed[B_SQ]) { /* R + □: esconde/mostra o painel da carta */
        infoPanel = !infoPanel;
        cfgSave();
        return;
    }
    if (held[B_R] && pressed[B_L]) {
        if (net_on()) net_cycle();
        return;
    }
    if (held[B_R]) { /* vida */
        int d = pressed[B_UP] ? 1 : pressed[B_DOWN] ? -1 : pressed[B_RIGHT] ? 5 : pressed[B_LEFT] ? -5 : 0;
        if (d) {
            snap();
            S.life += d;
            net_log("vida %d", S.life);
        }
        return;
    }
    for (int b = B_UP; b <= B_RIGHT; b++)
        if (pressed[b]) moveSel(b);
    if (pressed[B_START]) {
        if (S.phase == PH_BOTTOM)
            confirmBottom();
        else
            mainMenu();
        return;
    }
    if (pressed[B_L] && !held[B_R]) undoLast();
    if (S.phase == PH_MULL) {
        if (pressed[B_X]) keepHand();
        if (pressed[B_O]) mulligan();
        if (pressed[B_TRI] && selKind == K_CARD) aZoom(selId);
        return;
    }
    if (S.phase == PH_BOTTOM) {
        if (pressed[B_X] && selKind == K_CARD && selZone == Z_HAND) {
            int k;
            for (k = 0; k < S.npick; k++)
                if (S.pick[k] == selId) break;
            if (k < S.npick) {
                memmove(&S.pick[k], &S.pick[k + 1], sizeof(short) * (S.npick - k - 1));
                S.npick--;
            } else if (S.npick < bottomCount())
                S.pick[S.npick++] = selId;
        }
        if (pressed[B_TRI] && selKind == K_CARD) aZoom(selId);
        return;
    }
    if (pressed[B_SELECT]) nextStep();
    if (pressed[B_SQ]) drawCmd(1);
    if (equipSrc) { /* escolhendo onde prender */
        if (!S.in[equipSrc].used || (S.in[equipSrc].zone != Z_BF && S.in[equipSrc].zone != Z_HAND)) equipSrc = 0;
        if (pressed[B_O]) {
            equipSrc = 0;
            announce("Cancelado");
        } else if (pressed[B_X] && selKind == K_CARD && selZone == Z_BF && selId > 0 && selId != equipSrc && hostOf(selId) != equipSrc) {
            int s = equipSrc;
            equipSrc = 0;
            doAttach(s, selId);
        } else if (pressed[B_TRI] && selKind == K_CARD && selId > 0)
            aZoom(selId);
        return;
    }
    if (selKind == K_CARD && selId > 0 && S.in[selId].used) {
        int id = selId;
        if (pressed[B_TRI]) aZoom(id);
        if (pressed[B_O]) cardMenu(id);
        if (pressed[B_X]) {
            if (selZone == Z_HAND || selZone == Z_CMD)
                aBF(id);
            else if (selZone == Z_BF)
                aTap(id);
        }
    } else if (selKind == K_PEER) {
        if (pressed[B_TRI] || pressed[B_X]) net_zoom(selId);
    } else if (selKind == K_PILE) {
        if (pressed[B_X]) {
            if (selZone == Z_LIB)
                drawCmd(1);
            else if (selZone != Z_CMD)
                aView(selZone);
        }
        if (pressed[B_O]) pileMenu(selZone);
    }
}

/* ===================== escolher deck ===================== */

static char deckDirs[40][128], deckTitles[40][96];
static int ndecks, deckCur;
static char netMsg[160];

/* config.txt ao lado do EBOOT: guarda a faixa de nome/custo e o último deck aberto */
static char lastDeck[128];
static void cfgSave(void) {
    FILE *f = fopen("config.txt", "w");
    if (!f) return;
    fprintf(f, "barra\t%d\ndeck\t%s\ncpu\t%d\npainel\t%d\n", showBar, lastDeck, cpuMax, infoPanel);
    fclose(f);
}
static void cfgLoad(void) {
    FILE *f = fopen("config.txt", "r");
    char line[200];
    while (f && fgets(line, sizeof line, f)) {
        line[strcspn(line, "\r\n")] = 0;
        if (!strncmp(line, "barra\t", 6)) showBar = atoi(line + 6) != 0;
        if (!strncmp(line, "cpu\t", 4)) cpuMax = atoi(line + 4) != 0;
        if (!strncmp(line, "painel\t", 7)) infoPanel = atoi(line + 7) != 0;
        if (!strncmp(line, "deck\t", 5)) snprintf(lastDeck, sizeof lastDeck, "%s", line + 5);
    }
    if (f) fclose(f);
}
static void scanDecks(void) {
    ndecks = 0;
    DIR *d = opendir("decks");
    if (!d) return;
    struct dirent *e;
    while ((e = readdir(d)) && ndecks < 40) {
        if (e->d_name[0] == '.') continue;
        char p[300];
        snprintf(p, sizeof p, "decks/%s/deck.txt", e->d_name);
        FILE *f = fopen(p, "r");
        if (!f) continue;
        char line[200];
        snprintf(deckTitles[ndecks], 96, "%s", e->d_name);
        while (fgets(line, sizeof line, f))
            if (!strncmp(line, "NOME\t", 5)) {
                line[strcspn(line, "\r\n")] = 0;
                snprintf(deckTitles[ndecks], 96, "%s", line + 5);
                break;
            }
        fclose(f);
        snprintf(deckDirs[ndecks], 128, "decks/%s", e->d_name);
        ndecks++;
    }
    closedir(d);
    for (int a = 1; a < ndecks; a++) /* ordem alfabética */
        for (int b = a; b > 0 && strcmp(deckTitles[b - 1], deckTitles[b]) > 0; b--) {
            char t[128];
            memcpy(t, deckTitles[b], 96), memcpy(deckTitles[b], deckTitles[b - 1], 96), memcpy(deckTitles[b - 1], t, 96);
            memcpy(t, deckDirs[b], 128), memcpy(deckDirs[b], deckDirs[b - 1], 128), memcpy(deckDirs[b - 1], t, 128);
        }
    for (int k = 0; k < ndecks; k++) /* começa no último deck usado */
        if (*lastDeck && !strcmp(deckDirs[k], lastDeck)) deckCur = k;
}

static void drawLogo(int x, int y, int s) {
    fill(x, y, s, s, cAcc);
    fill(x + 2, y + 2, s - 4, s - 4, cPanel);
    fill(x + s * 3 / 10, y + s * 3 / 10, s * 4 / 10, s * 4 / 10, cAcc2);
}

static int screen; /* 0 decks, 1 mesa */
static int helpPageFor(void) {
    if (screen == 0) return 7;
    if (zoom.on) return 3;
    if (nmenu) return 4;
    if (vw.on) return 2;
    if (S.phase == PH_MULL || S.phase == PH_BOTTOM) return 5;
    if (net_view() > 0) return 6;
    if (selKind == K_PILE) return 1;
    return 0;
}

static void drawDecks(void) {
    fill(0, 0, SW, SH, cBg);
    drawLogo(14, 10, 22);
    text(44, 8, "Mesa Arcana", 20, 1, cAcc);
    text(16, 38, "Escolha o deck", 11, 0, cMut);
    drawStatus(SW - 8, 8);
    if (!ndecks) {
        textWrap(16, 64, SW - 32,
                 "Nenhum deck encontrado.\n\nNo programa do PC, escolha o deck e clique em \"PSP\". Ele grava a pasta PSP ao lado do programa: copie essa pasta para a raiz do Memory Stick (junte com a que já existe).",
                 12, 0, cTxt);
    }
    int vis = 9, first = deckCur - vis + 1 > 0 ? deckCur - vis + 1 : 0;
    for (int k = first; k < ndecks && k < first + vis; k++) {
        int y = 58 + (k - first) * 21;
        if (k == deckCur) {
            fill(12, y - 2, SW - 24, 20, cPanel2);
            fill(12, y - 2, 3, 20, cAcc);
        }
        char b[120];
        fit(b, sizeof b, deckTitles[k], 13, k == deckCur, SW - 50);
        text(22, y, b, 13, k == deckCur, k == deckCur ? cTxt : cMut);
    }
    if (*netMsg) text(16, SH - 34, netMsg, 10, 0, cAcc2);
    textC(SW / 2, SH - 16, ndecks ? "X joga · START rede · R+SELECT atalhos · SELECT sai" : "SELECT sai", 9, 0, cMut);
}

static int decksInput(void) {
    if (pressed[B_UP] && deckCur > 0) deckCur--;
    if (pressed[B_DOWN] && deckCur < ndecks - 1) deckCur++;
    if (pressed[B_SELECT]) running = 0;
    if (pressed[B_START]) net_menu();
    if (pressed[B_X] && ndecks) {
        if (loadDeck(deckDirs[deckCur])) {
            snprintf(lastDeck, sizeof lastDeck, "%s", deckDirs[deckCur]);
            cfgSave();
            newGame();
            selKind = 0, selX = 200, selY = 230;
            return 1;
        }
        announce("Não consegui abrir o deck");
    }
    return 0;
}

/* ===================== principal ===================== */

static void shot(const char *name) {
#ifndef __PSP__
    int w, h;
    SDL_GetRendererOutputSize(R, &w, &h);
    SDL_Surface *s = SDL_CreateRGBSurfaceWithFormat(0, w, h, 32, SDL_PIXELFORMAT_ARGB8888);
    SDL_RenderReadPixels(R, NULL, SDL_PIXELFORMAT_ARGB8888, s->pixels, s->pitch);
    char p[128];
    snprintf(p, sizeof p, "shots/%s.bmp", name);
    SDL_SaveBMP(s, p);
    SDL_FreeSurface(s);
    printf("tela %s\n", name);
#else
    (void)name;
#endif
}

int main(int argc, char *argv[]) {
    (void)argc, (void)argv;
    srand((unsigned)time(NULL));
#ifndef __PSP__
    if (getenv("MESA_SEED")) srand((unsigned)atoi(getenv("MESA_SEED"))); /* testes repetíveis */
#endif
    SDL_Init(SDL_INIT_VIDEO | SDL_INIT_JOYSTICK);
    int scale = 1;
#ifndef __PSP__
    if (getenv("MESA_ESCALA")) scale = atoi(getenv("MESA_ESCALA"));
    if (scale < 1) scale = 1;
#endif
    win = SDL_CreateWindow("Mesa Arcana", SDL_WINDOWPOS_CENTERED, SDL_WINDOWPOS_CENTERED, SW * scale, SH * scale, 0);
    R = SDL_CreateRenderer(win, -1, SDL_RENDERER_ACCELERATED | SDL_RENDERER_PRESENTVSYNC);
    SDL_RenderSetLogicalSize(R, SW, SH);
    SDL_SetRenderDrawBlendMode(R, SDL_BLENDMODE_BLEND);
    SDL_JoystickOpen(0);
    TTF_Init();
    IMG_Init(IMG_INIT_JPG | IMG_INIT_PNG);
    net_init();
    FILE *tf = fopen("teste.txt", "r");
    if (tf) {
        fseek(tf, 0, SEEK_END);
        long n = ftell(tf);
        fseek(tf, 0, SEEK_SET);
        script = calloc(1, n + 1);
        if (fread(script, 1, n, tf) != (size_t)n) script[0] = 0;
        fclose(tf);
        scriptPos = script;
    }
    if (!font(12, 0)) {
        printf("faltam as fontes ui.ttf e uib.ttf\n");
    }
    cpuSpeed(1);
    cfgLoad();
    scanDecks();
    cpuSpeed(0);
    while (running) {
        frameN++;
        stickScrolls = screen == 1 && infoPanel && !helpOn && !nmenu && !vw.on && !zoom.on && !net_log_on();
        readInput();
        if (stickScrolls && (stickY > 6000 || stickY < -6000)) infoScroll += stickY / 5000; /* quanto mais empurra, mais rápido */
        net_tick();
        if (wantDecks) {
            wantDecks = 0;
            closeMenus();
            screen = 0;
            scanDecks();
        }
        if (wantNew) {
            wantNew = 0;
            closeMenus();
            newGame();
        }
        /* entrada: o que estiver por cima recebe os botões */
        int anyP = 0;
        for (int b = 0; b < NB; b++) anyP |= pressed[b];
        if (!helpOn && held[B_R] && pressed[B_SELECT]) { /* R + SELECT: atalhos, em qualquer tela */
            helpOpen();
            memset(pressed, 0, sizeof pressed);
        }
        if (helpOn) {
            helpInput();
        } else if (net_log_on())
            net_log_input(anyP);
        else if (zoom.on)
            zoomInput();
        else if (nmenu)
            menuInput();
        else if (vw.on)
            viewerInput();
        else if (screen == 0) {
            if (decksInput()) screen = 1;
        } else
            gameInput();

        /* tela parada: não redesenha (poupa bateria). Volta a desenhar com qualquer botão,
           aviso na tela, pacote da rede, e uma vez por meio segundo (relógio). */
        int anyH = 0;
        for (int b = 0; b < NB; b++) anyH |= held[b];
        static int calm;
        calm = (anyP || anyH || toastT > 0 || net_dirty || scriptPos || scriptWait || (stickScrolls && (stickY > 6000 || stickY < -6000))) ? 0 : calm + 1;
        net_dirty = 0;
        if (calm > 3 && frameN % 30 && !*shotName) {
            SDL_Delay(16);
            continue;
        }
        SDL_SetRenderDrawColor(R, cBg.r, cBg.g, cBg.b, 255);
        SDL_RenderClear(R);
        if (helpOn) /* a lista de atalhos cobre tudo: não desenha a mesa embaixo */
            drawHelp();
        else if (screen == 0)
            drawDecks();
        else {
            nitems = 0;
            BFX = infoPanel ? INFOW : 0, BFW = PILEX - BFX;
            if (infoPanel) drawInfo();
            if (net_view() > 0)
                net_draw_table(BFX, BFY, BFW, BFH);
            else
                drawBattlefield();
            drawHand();
            drawPiles();
            drawTop();
            if (net_on()) net_draw_peers(BFX, BFY, BFW, 14);
            drawPrompt();
            resolveSel();
        }
        if (vw.on && !helpOn) drawViewer();
        if (toastT > 0 && !helpOn) { /* aviso: fica embaixo dos menus e da carta grande */
            toastT--;
            int w = textW(toast, 13, 1) + 24;
            int x = BFX + (BFW - w) / 2, y = 120;
            if (x < 4) x = 4;
            fill(x, y, w, 26, (SDL_Color){0, 0, 0, 200});
            frame(x, y, w, 26, cAcc);
            textC(x + w / 2, y + 5, toast, 13, 1, cAcc);
        }
        if (nmenu && !helpOn) drawMenu();
        if (zoom.on && !helpOn) drawZoom();
        if (net_log_on()) net_draw_log();
        if (*shotName) {
            shot(shotName);
            *shotName = 0;
        }
        SDL_RenderPresent(R);
#ifndef __PSP__
        if (scriptPos || scriptWait) SDL_Delay(1);
#endif
    }
    net_shutdown();
    freeDeck();
    TTF_Quit();
    SDL_Quit();
    return 0;
}
