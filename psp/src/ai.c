/* Partida contra a IA (incluído em main.c, usa as funções de lá).
   A IA não lê o efeito das cartas: joga um terreno por turno, conjura pela curva de mana,
   ataca quando é seguro e bloqueia para não morrer. Entende as palavras-chave de combate e
   alguns feitiços simples (destruir/exilar criatura, destruir todas, comprar cartas).
   Na tela ela aparece como um jogador da rede (R + L mostra a mesa dela). */

typedef struct {
    char *n, *c, *t, *x, *p, *o;
} ADef;
static ADef adef[MAXDEF];
static int nadef;

#define AMAX 140
typedef struct {
    short d;
    char tap, sick, cmdr;
} ACard;
static struct {
    int on, over, life, turn, tax, pending;
    char dir[256], name[96];
    ACard c[AMAX];
    int nc;
    short z[NZ][AMAX];
    short zn[NZ];
} A;

/* procura sem diferenciar maiúsculas (só ASCII; acentos têm que bater) */
static int has(const char *s, const char *k) {
    if (!s || !k) return 0;
    size_t n = strlen(k);
    for (; *s; s++) {
        size_t i = 0;
        while (i < n && s[i] && tolower((unsigned char)s[i]) == tolower((unsigned char)k[i])) i++;
        if (i == n) return 1;
    }
    return 0;
}
static int has2(const char *s, const char *en, const char *pt) { return has(s, en) || has(s, pt); }

static int manaVal(const char *c) {
    int n = 0;
    for (const char *p = c; p && *p; p++) {
        if (*p != '{') continue;
        const char *e = strchr(p, '}');
        if (!e) break;
        if (p[1] >= '0' && p[1] <= '9')
            n += atoi(p + 1);
        else if (p[1] != 'X' && p[1] != 'Y' && p[1] != 'Z')
            n++;
        p = e;
    }
    return n;
}

typedef struct {
    char name[48];
    int pow, tou, fly, reach, trample, death, life, vig, defen;
    int mine; /* id da minha carta; -1 = carta da IA */
    int ai;   /* índice da carta da IA */
} Fighter;

static void kwFill(Fighter *f, const char *x) {
    f->fly = has2(x, "Flying", "Voar");
    f->reach = has2(x, "Reach", "Alcance");
    f->trample = has2(x, "Trample", "Atropelar");
    f->death = has2(x, "Deathtouch", "Toque mortífero");
    f->life = has2(x, "Lifelink", "Vínculo com a vida");
    f->vig = has2(x, "Vigilance", "Vigilância");
    f->defen = has2(x, "Defender", "Defensor");
}
static ADef *aD(int k) { return &adef[A.c[k].d]; }
static int aIs(int k, const char *en, const char *pt) { return has2(aD(k)->t, en, pt); }
static int aCreature(int k) { return aIs(k, "Creature", "Criatura"); }
static int aLand(int k) { return aIs(k, "Land", "Terreno"); }
static int aSpell(int k) { return aIs(k, "Instant", "Instantânea") || aIs(k, "Sorcery", "Feitiço"); }
static Fighter fAI(int k) {
    Fighter f = {0};
    snprintf(f.name, sizeof f.name, "%s", aD(k)->n);
    f.pow = atoi(aD(k)->p), f.tou = atoi(aD(k)->o);
    kwFill(&f, aD(k)->x);
    f.mine = -1, f.ai = k;
    return f;
}
static int myCreature(int id) { return !S.in[id].down && has2(defs[S.in[id].def].f[0].t, "Creature", "Criatura"); }
static Fighter fMine(int id) {
    Inst *i = &S.in[id];
    Face *fc = &defs[i->def].f[i->face < defs[i->def].nf ? i->face : 0];
    Fighter f = {0};
    snprintf(f.name, sizeof f.name, "%s", fc->n);
    f.pow = atoi(fc->p ? fc->p : "0") + i->plus, f.tou = atoi(fc->o ? fc->o : "0") + i->plus;
    kwFill(&f, fc->x);
    f.mine = id, f.ai = -1;
    return f;
}
static int canBlk(Fighter *a, Fighter *b) { return !a->fly || b->fly || b->reach; }
static int killsF(Fighter *s, Fighter *d) { return s->pow > 0 && (s->death || s->pow >= d->tou); }

/* zonas da IA (mesmos números das minhas) */
static void azrem(int k) {
    for (int z = 0; z < NZ; z++)
        for (int j = 0; j < A.zn[z]; j++)
            if (A.z[z][j] == k) {
                memmove(&A.z[z][j], &A.z[z][j + 1], (A.zn[z] - j - 1) * sizeof(short));
                A.zn[z]--;
                return;
            }
}
static void azadd(int z, int k) {
    if (A.zn[z] < AMAX) A.z[z][A.zn[z]++] = k;
}
static void amove(int k, int z) {
    azrem(k);
    azadd(z, k);
}
static int adraw(int n) {
    int k = 0;
    for (; k < n && A.zn[Z_LIB]; k++) amove(A.z[Z_LIB][0], Z_HAND);
    return k;
}

static void aiSay(const char *fmt, ...) {
    char t[110];
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(t, sizeof t, fmt, ap);
    va_end(ap);
    net_ai_say(t);
}

/* o que a mesa mostra da IA */
static void aiPublish(void) {
    int n = 0;
    for (int r = 0; r < 3; r++)
        for (int j = 0; j < A.zn[Z_BF]; j++) {
            int k = A.z[Z_BF][j];
            int row = aCreature(k) ? 0 : aLand(k) ? 2 : 1;
            if (row != r) continue;
            char pt[12] = "";
            if (aCreature(k)) snprintf(pt, sizeof pt, "%s/%s", aD(k)->p, aD(k)->o);
            net_ai_card(n++, aD(k)->n, pt, A.c[k].tap, row);
        }
    net_ai_stats(A.life, A.turn, A.zn[Z_HAND], A.zn[Z_LIB], A.zn[Z_GY], A.zn[Z_EX], n);
}

static void aiFree(void) {
    for (int i = 0; i < nadef; i++) free(adef[i].n), free(adef[i].c), free(adef[i].t), free(adef[i].x), free(adef[i].p), free(adef[i].o);
    memset(adef, 0, sizeof adef);
    nadef = 0;
}

static int aiLoad(const char *dir) {
    char path[320], line[8192];
    snprintf(path, sizeof path, "%s/deck.txt", dir);
    FILE *f = fopen(path, "r");
    if (!f) return 0;
    aiFree();
    memset(&A, 0, sizeof A);
    snprintf(A.dir, sizeof A.dir, "%s", dir);
    int qty[MAXDEF] = {0}, cm[MAXDEF] = {0}, cur = -1, fi = 0;
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
            snprintf(A.name, sizeof A.name, "%s", fld[1]);
        else if (!strcmp(fld[0], "CARTA") && nf >= 6 && nadef < MAXDEF) {
            cur = atoi(fld[4]) ? -1 : nadef++; /* fichas ficam de fora */
            if (cur >= 0) qty[cur] = atoi(fld[2]), cm[cur] = atoi(fld[3]);
            fi = 0;
        } else if (!strcmp(fld[0], "FACE") && cur >= 0 && fi++ == 0) {
            const char *v[7];
            for (int k = 0; k < 7; k++) v[k] = k + 1 < nf ? fld[k + 1] : "";
            ADef *d = &adef[cur];
            d->n = unesc(v[0]), d->c = unesc(v[1]), d->t = unesc(v[2]), d->x = unesc(v[3]), d->p = unesc(v[4]), d->o = unesc(v[5]);
        }
    }
    fclose(f);
    for (int d = 0; d < nadef; d++) {
        if (!adef[d].n) continue;
        for (int q = 0; q < qty[d] && A.nc < AMAX; q++) {
            int k = A.nc++;
            A.c[k].d = d;
            if (cm[d] && q == 0) {
                A.c[k].cmdr = 1;
                azadd(Z_CMD, k);
            } else
                azadd(Z_LIB, k);
        }
    }
    return A.nc > 0;
}

static void aiShuffle(void) {
    for (int i = A.zn[Z_LIB] - 1; i > 0; i--) {
        int j = rand() % (i + 1);
        short t = A.z[Z_LIB][i];
        A.z[Z_LIB][i] = A.z[Z_LIB][j], A.z[Z_LIB][j] = t;
    }
}

static void aiStart(const char *dir) {
    if (!aiLoad(dir)) {
        announce("Não consegui abrir o deck da IA");
        return;
    }
    A.on = 1;
    A.life = A.zn[Z_CMD] ? 40 : 20;
    aiShuffle();
    adraw(7);
    net_ai_begin("IA");
    aiPublish();
    aiSay("vai jogar com %s", A.name);
}
static void aiRestart(void) {
    if (!A.on) return;
    char d[256];
    snprintf(d, sizeof d, "%s", A.dir);
    aiStart(d);
}
static void aiStop(void) {
    if (!A.on) return;
    A.on = 0;
    net_ai_end();
}

/* ---------- turno da IA ---------- */

static int aiManaSrc(int *out) {
    int n = 0;
    for (int j = 0; j < A.zn[Z_BF]; j++) {
        int k = A.z[Z_BF][j];
        if (A.c[k].tap) continue;
        if (aLand(k) || (has2(aD(k)->x, "{T}: Add", "{T}: Adicione") && !(aCreature(k) && A.c[k].sick))) out[n++] = k;
    }
    return n;
}
static int myBest(void) {
    int best = -1, bv = -1;
    for (int j = 0; j < S.zn[Z_BF]; j++) {
        int id = S.z[Z_BF][j];
        if (!myCreature(id)) continue;
        Fighter f = fMine(id);
        if (f.pow + f.tou > bv) bv = f.pow + f.tou, best = id;
    }
    return best;
}
static int aiWorth(int k) {
    if (!aSpell(k)) return 1;
    const char *x = aD(k)->x;
    if (has2(x, "Destroy all creatures", "Destrua todas as criaturas")) {
        int mine = 0, theirs = 0;
        for (int j = 0; j < S.zn[Z_BF]; j++)
            if (myCreature(S.z[Z_BF][j])) mine += 2;
        for (int j = 0; j < A.zn[Z_BF]; j++)
            if (aCreature(A.z[Z_BF][j])) theirs += 2;
        return mine >= theirs + 3;
    }
    if (has2(x, "target creature", "criatura alvo")) return myBest() >= 0;
    return 1;
}
static void aiSpellFx(int k) {
    const char *x = aD(k)->x, *n = aD(k)->n;
    int b = myBest();
    if (has2(x, "Destroy all creatures", "Destrua todas as criaturas")) {
        snap();
        for (int j = S.zn[Z_BF] - 1; j >= 0; j--)
            if (myCreature(S.z[Z_BF][j])) moveTo(S.z[Z_BF][j], Z_GY, 1);
        for (int j = A.zn[Z_BF] - 1; j >= 0; j--)
            if (aCreature(A.z[Z_BF][j])) amove(A.z[Z_BF][j], A.c[A.z[Z_BF][j]].cmdr ? Z_CMD : Z_GY);
        aiSay("conjurou %s: destruiu todas as criaturas", n);
        announce("IA: %s destruiu todas as criaturas", n);
    } else if (b >= 0 && has2(x, "Exile target creature", "Exile a criatura alvo")) {
        snap();
        aiSay("conjurou %s e exilou %s", n, fMine(b).name);
        announce("IA exilou %s", fMine(b).name);
        moveTo(b, Z_EX, 1);
    } else if (b >= 0 && has2(x, "Destroy target creature", "Destrua a criatura alvo")) {
        snap();
        aiSay("conjurou %s e destruiu %s", n, fMine(b).name);
        announce("IA destruiu %s", fMine(b).name);
        moveTo(b, Z_GY, 1);
    } else if (has2(x, "Draw three cards", "Compre três cards"))
        adraw(3), aiSay("conjurou %s e comprou 3", n);
    else if (has2(x, "Draw two cards", "Compre dois cards"))
        adraw(2), aiSay("conjurou %s e comprou 2", n);
    else if (has2(x, "Draw a card", "Compre um card"))
        adraw(1), aiSay("conjurou %s e comprou 1", n);
    else
        aiSay("conjurou %s (efeito não simulado)", n);
}
static void aiCast(void) {
    for (int guard = 0; guard < 30; guard++) {
        int src[AMAX], mana = aiManaSrc(src);
        if (A.zn[Z_CMD]) {
            int k = A.z[Z_CMD][0], cost = manaVal(aD(k)->c) + 2 * A.tax;
            if (cost <= mana) {
                for (int j = 0; j < cost; j++) A.c[src[j]].tap = 1;
                amove(k, Z_BF);
                A.c[k].sick = 1;
                A.tax++;
                aiSay("conjurou o comandante %s", aD(k)->n);
                continue;
            }
        }
        int pick = -1, pv = -1;
        for (int j = 0; j < A.zn[Z_HAND]; j++) {
            int k = A.z[Z_HAND][j], v = manaVal(aD(k)->c);
            if (!aLand(k) && v <= mana && v > pv && aiWorth(k)) pick = k, pv = v;
        }
        if (pick < 0) return;
        for (int j = 0; j < pv; j++) A.c[src[j]].tap = 1;
        if (aSpell(pick)) {
            amove(pick, Z_GY);
            aiSpellFx(pick);
        } else {
            amove(pick, Z_BF);
            A.c[pick].sick = 1;
            aiSay("conjurou %s", aD(pick)->n);
        }
    }
}

static Fighter att[24];
static int natt, blk[24]; /* blk: id da minha criatura bloqueando (−1 = ninguém) */

static void aiCheck(void) {
    if (A.life <= 0 && !A.over) {
        A.over = 1;
        aiSay("perdeu a partida");
        announce("Você venceu a IA!");
    }
    if (S.life <= 0 && !A.over) {
        A.over = 1;
        aiSay("venceu a partida");
        announce("A IA venceu");
    }
}
static void aiDies(int k) { amove(k, A.c[k].cmdr ? Z_CMD : Z_GY); }

static void beginTurn(void);
static void aiEndTurn(void) {
    while (A.zn[Z_HAND] > 7) { /* descarta a mais cara */
        int w = 0;
        for (int j = 1; j < A.zn[Z_HAND]; j++)
            if (manaVal(aD(A.z[Z_HAND][j])->c) > manaVal(aD(A.z[Z_HAND][w])->c)) w = j;
        int k = A.z[Z_HAND][w];
        amove(k, Z_GY);
        aiSay("descartou %s", aD(k)->n);
    }
    aiPublish();
    aiCheck();
    beginTurn();
}

static void resolveAIAttack(void) {
    A.pending = 0;
    snap();
    int dmg = 0, gain = 0;
    for (int a = 0; a < natt; a++) {
        Fighter *f = &att[a];
        if (!f->vig) A.c[f->ai].tap = 1;
        if (f->life) gain += f->pow;
        if (blk[a] < 0) {
            dmg += f->pow;
            continue;
        }
        Fighter b = fMine(blk[a]);
        if (f->trample && !b.death && f->pow > b.tou) dmg += f->pow - b.tou;
        int dieB = killsF(f, &b), dieA = killsF(&b, f);
        if (dieB) moveTo(blk[a], Z_GY, 1), aiSay("%s morreu bloqueando", b.name);
        if (dieA) aiDies(f->ai), aiSay("%s (IA) morreu", f->name);
    }
    S.life -= dmg;
    A.life += gain;
    announce("Combate: %d de dano em você", dmg);
    aiSay("você recebeu %d de dano", dmg);
    aiEndTurn();
}

static void relabelBlock(void) {
    Menu *m = &menus[nmenu - 1];
    for (int a = 0; a < natt; a++) {
        char b[80];
        if (blk[a] < 0)
            snprintf(b, sizeof b, "%s %d/%d%s · < sem bloqueio >", att[a].name, att[a].pow, att[a].tou, att[a].fly ? " voa" : "");
        else {
            Fighter f = fMine(blk[a]);
            snprintf(b, sizeof b, "%s %d/%d · < %s %d/%d >", att[a].name, att[a].pow, att[a].tou, f.name, f.pow, f.tou);
        }
        snprintf(m->it[a].label, sizeof m->it[a].label, "%s", b);
    }
}
static void adjBlock(int a, int d) {
    int opts[64], n = 0;
    opts[n++] = -1;
    for (int j = 0; j < S.zn[Z_BF] && n < 64; j++) {
        int id = S.z[Z_BF][j];
        if (!myCreature(id) || S.in[id].tapped) continue;
        int used = 0;
        for (int o = 0; o < natt; o++)
            if (o != a && blk[o] == id) used = 1;
        Fighter b = fMine(id);
        if (!used && canBlk(&att[a], &b)) opts[n++] = id;
    }
    int cur = 0;
    for (int j = 0; j < n; j++)
        if (opts[j] == blk[a]) cur = j;
    blk[a] = opts[(cur + d + n) % n];
}
static void aConfirmBlocks(int x) {
    (void)x;
    resolveAIAttack();
}

static void aiAttack(void) {
    natt = 0;
    int total = 0, myBlockers = 0;
    for (int j = 0; j < S.zn[Z_BF]; j++)
        if (myCreature(S.z[Z_BF][j]) && !S.in[S.z[Z_BF][j]].tapped) myBlockers++;
    for (int j = 0; j < A.zn[Z_BF] && natt < 24; j++) {
        int k = A.z[Z_BF][j];
        if (!aCreature(k) || A.c[k].tap || (A.c[k].sick && !has2(aD(k)->x, "Haste", "Ímpeto"))) continue;
        Fighter f = fAI(k);
        if (f.pow <= 0 || f.defen) continue;
        int safe = 1;
        for (int i = 0; i < S.zn[Z_BF]; i++) {
            int id = S.z[Z_BF][i];
            if (!myCreature(id) || S.in[id].tapped) continue;
            Fighter b = fMine(id);
            if (canBlk(&f, &b) && killsF(&b, &f) && !killsF(&f, &b)) safe = 0;
        }
        total += f.pow;
        if (safe || !myBlockers) att[natt++] = f;
    }
    (void)total;
    if (!natt) {
        aiEndTurn();
        return;
    }
    int sum = 0;
    for (int a = 0; a < natt; a++) blk[a] = -1, sum += att[a].pow;
    aiSay("ataca com %d criatura(s), %d de dano", natt, sum);
    A.pending = 1;
    char t[80];
    snprintf(t, sizeof t, "A IA ataca: %d de dano. Escolha os bloqueios", sum);
    Menu *m = openMenu(t);
    for (int a = 0; a < natt; a++) {
        MItem *it = mAdd(m, "", NULL, a);
        it->adj = adjBlock;
        it->keep = 1;
    }
    mAdd(m, "Confirmar bloqueios", aConfirmBlocks, 0);
    m->relabel = relabelBlock;
    relabelBlock();
}

/* chamado a cada quadro: se a janela de bloqueio foi fechada com O, conta como "sem bloqueios" */
static void aiTick(void) {
    if (A.on && A.pending && !nmenu) resolveAIAttack();
}

static void aiTurn(void) {
    if (A.over) {
        beginTurn();
        return;
    }
    A.turn++;
    for (int j = 0; j < A.zn[Z_BF]; j++) A.c[A.z[Z_BF][j]].tap = 0, A.c[A.z[Z_BF][j]].sick = 0;
    if (!adraw(1)) {
        aiSay("não tinha carta para comprar");
        A.life = 0;
        aiCheck();
        beginTurn();
        return;
    }
    aiSay("turno %d", A.turn);
    for (int j = 0; j < A.zn[Z_HAND]; j++) {
        int k = A.z[Z_HAND][j];
        if (aLand(k)) {
            amove(k, Z_BF);
            A.c[k].tap = has2(aD(k)->x, "enters tapped", "entra virad") || has(aD(k)->x, "battlefield tapped") || has(aD(k)->x, "campo de batalha virad");
            aiSay("jogou %s", aD(k)->n);
            break;
        }
    }
    aiCast();
    aiPublish();
    aiAttack();
}

/* ---------- eu ataco a IA ---------- */
static int pickAtt[MAXI];
static void relabelAtt(void) {
    Menu *m = &menus[nmenu - 1];
    for (int k = 0; k < m->n - 1; k++) {
        int id = m->it[k].arg;
        Fighter f = fMine(id);
        snprintf(m->it[k].label, sizeof m->it[k].label, "%s %s %d/%d", pickAtt[id] ? "[X]" : "[  ]", f.name, f.pow, f.tou);
    }
}
static void aTogAtt(int id) { pickAtt[id] = !pickAtt[id]; }
static void aDoAttack(int x) {
    (void)x;
    Fighter at[24];
    int n = 0, total = 0;
    snap();
    for (int j = 0; j < S.zn[Z_BF] && n < 24; j++) {
        int id = S.z[Z_BF][j];
        if (!pickAtt[id]) continue;
        at[n] = fMine(id);
        total += at[n].pow;
        if (!at[n].vig) S.in[id].tapped = 1;
        n++;
    }
    if (!n) return;
    char used[AMAX] = {0};
    int dmg = 0, gain = 0;
    for (int a = 0; a < n; a++) { /* a IA bloqueia: troca boa, ou sacrifício se o ataque mataria */
        int best = -1;
        for (int j = 0; j < A.zn[Z_BF]; j++) {
            int k = A.z[Z_BF][j];
            if (!aCreature(k) || A.c[k].tap || used[k]) continue;
            Fighter b = fAI(k);
            if (canBlk(&at[a], &b) && killsF(&b, &at[a]) && !killsF(&at[a], &b)) {
                best = k;
                break;
            }
        }
        if (best < 0 && total >= A.life)
            for (int j = 0; j < A.zn[Z_BF]; j++) {
                int k = A.z[Z_BF][j];
                if (!aCreature(k) || A.c[k].tap || used[k]) continue;
                Fighter b = fAI(k);
                if (canBlk(&at[a], &b) && (best < 0 || b.pow + b.tou < fAI(best).pow + fAI(best).tou)) best = k;
            }
        if (at[a].life) gain += at[a].pow;
        if (best < 0) {
            dmg += at[a].pow;
            continue;
        }
        used[best] = 1;
        total -= at[a].pow;
        Fighter b = fAI(best);
        aiSay("%s bloqueou %s", b.name, at[a].name);
        if (at[a].trample && !b.death && at[a].pow > b.tou) dmg += at[a].pow - b.tou;
        int dieB = killsF(&at[a], &b), dieA = killsF(&b, &at[a]);
        if (dieB) aiDies(best);
        if (dieA) moveTo(at[a].mine, Z_GY, 1);
    }
    A.life -= dmg;
    S.life += gain;
    announce("A IA recebeu %d de dano", dmg);
    aiSay("recebeu %d de dano", dmg);
    aiPublish();
    aiCheck();
}
static void aAttackMenu(int x) {
    (void)x;
    if (!A.on || A.over) return;
    Menu *m = openMenu("Atacar a IA: X marca quem ataca");
    memset(pickAtt, 0, sizeof pickAtt);
    for (int j = 0; j < S.zn[Z_BF]; j++) {
        int id = S.z[Z_BF][j];
        if (myCreature(id) && !S.in[id].tapped) mAdd(m, "", aTogAtt, id)->keep = 1;
    }
    if (!m->n) {
        nmenu--;
        announce("Nenhuma criatura desvirada para atacar");
        return;
    }
    mAdd(m, "Atacar", aDoAttack, 0);
    m->relabel = relabelAtt;
    relabelAtt();
}
