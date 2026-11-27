/* Mesa Arcana para PSP: definições comuns */
#ifndef BASE_H
#define BASE_H

#include <SDL2/SDL.h>
#include <SDL2/SDL_image.h>
#include <SDL2/SDL_ttf.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define SW 480
#define SH 272

#define MAXDEF 260
#define MAXI 400
#define UNDO 24

enum { Z_LIB, Z_HAND, Z_BF, Z_GY, Z_EX, Z_CMD, NZ };
enum { PH_MULL, PH_BOTTOM, PH_PLAY };
enum { B_UP, B_DOWN, B_LEFT, B_RIGHT, B_X, B_O, B_TRI, B_SQ, B_L, B_R, B_START, B_SELECT, NB };

typedef struct {
    char *n, *c, *t, *x, *p, *o, *l;
} Face;

typedef struct {
    int qty, cmdr, token, nf;
    Face f[2];
    SDL_Texture *sm[2];
} Def;

typedef struct {
    short def, face, plus, cnt, tax;
    short att; /* presa (equipada/encantando) na carta att do campo; 0 = solta */
    char zone, used, tapped, down, token, cmdr;
} Inst;

typedef struct {
    Inst in[MAXI];
    short z[NZ][MAXI];
    short zn[NZ];
    int life, poison, turn, step, phase, mulls;
    short pick[16];
    int npick;
} State;

extern Def defs[MAXDEF];
extern int ndef;
extern State S;
extern char deckDir[256], deckName[96];

/* cores */
extern const SDL_Color cBg, cPanel, cPanel2, cLine, cTxt, cMut, cAcc, cAcc2, cBad, cFelt, cInk, cPaper;

/* desenho (main.c) */
extern SDL_Renderer *R;
void fill(int x, int y, int w, int h, SDL_Color c);
void frame(int x, int y, int w, int h, SDL_Color c);
int text(int x, int y, const char *s, int size, int bold, SDL_Color c);
int textC(int cx, int y, const char *s, int size, int bold, SDL_Color c);
int textWrap(int x, int y, int w, const char *s, int size, int bold, SDL_Color c);
int textW(const char *s, int size, int bold);
void fit(char *out, int cap, const char *s, int size, int bold, int maxw);
void announce(const char *fmt, ...);

extern const char *stepNames[6];
enum { K_CARD = 1, K_PILE, K_PEER };
extern int selKind, selZone, selId;
void addSel(int x, int y, int w, int h, int kind, int zone, int id);
int isSel(int kind, int zone, int id);
void drawBack(int x, int y, int w, int h);
void drawPaper(const Face *f, int x, int y, int w, int h);
extern int showBar;
void nameBar(const Face *f, int x, int y, int w);
int drawCost(int xr, int y, const char *cost, int d);
void badge(int x, int y, const char *s, SDL_Color bg, SDL_Color fg);
void bfGeom(int *top, int *rowH, int *cw, int *ch);
void zoomExt(const Face *f, const char *bigPath, const char *extra);
extern unsigned frameN;
extern int nOpen;

/* rede (net.c) */
void net_init(void);
void net_ai_begin(const char *name);
void net_ai_end(void);
void net_ai_say(const char *s);
void net_ai_card(int k, const char *name, const char *pt, int tapped, int row);
void net_ai_stats(int life, int turn, int hand, int lib, int gy, int ex, int ncard);
void net_menu(void);
void net_tick(void);
extern int net_dirty;
int net_on(void);
int net_npeers(void);
void net_draw_peers(int x, int y, int w, int h);
int net_view(void);   /* jogador sendo visto (0 = minha mesa) */
void net_cycle(void); /* troca a mesa que aparece */
void net_draw_table(int x, int y, int w, int h);
void net_log(const char *fmt, ...);
void net_shutdown(void);
void net_zoom(int card);
int net_log_on(void);
void net_log_input(int any);
void net_draw_log(void);

#endif
