# Memória de tradução: aprende com os pares oficiais (texto Oracle em inglês x texto impresso em português)
# e reaproveita as frases oficiais nas cartas que nunca saíram em português.
import gzip,json,re,collections,sys
pt=json.load(gzip.open('build/pt.json.gz'))
db=json.load(gzip.open('dados/cartas.json.gz'))
cards=[c for c in db['cartas']]
PAR=re.compile(r'\s*\([^()]*\)')
def clean(s): return PAR.sub('',s).replace('—','—').strip()
NUM=re.compile(r'(?<![\w{+\-/])(\d+)(?![\w}/])')
def absn(s):
    nums=[]
    def f(m):
        v=m.group(1)
        if int(v)<2: return v
        nums.append(v); return '#'
    return NUM.sub(f,s),nums
def fill(t,nums):
    it=iter(nums); return re.sub('#',lambda m:next(it,'#'),t)
def names(n):
    out=[n]
    if ',' in n: out.append(n.split(',')[0])
    return out
SELF=re.compile(r"\b[Tt]his (creature|land|artifact|enchantment|Aura|Equipment|Vehicle|permanent|Saga|Class|Case|Room|Spacecraft|Planet|Siege|battle|planeswalker|card)\b(?! card)")
def tmpl_en(line,name):
    for n in names(name): line=line.replace(n,'~')
    line=SELF.sub('~',line)
    line=line.replace('enters the battlefield','enters').replace("enter the battlefield","enter")
    return absn(line)
TRIG=re.compile(r'^(When|Whenever|At the beginning|Quando|Toda vez|No início|Sempre que)\b')
def lc(x): return x[:1].lower()+x[1:]
def segs(t):
    """quebra uma frase em pedaços reaproveitáveis: custo: efeito  ou  gatilho, efeito"""
    m=re.match(r'^([^.:"“]{1,120}): (.+)$',t)
    if m and not TRIG.match(t): return ['C:'+m.group(1),'E:'+lc(m.group(2))]
    if TRIG.match(t) and ', ' in t:
        a,b=t.split(', ',1); return ['T:'+a,'E:'+lc(b)]
    return None
def sents(l):
    return [x for x in re.split(r'(?<=[.!])\s+(?=[A-ZÀ-Ú~{•"“])',l) if x]
TM=collections.defaultdict(collections.Counter)
TY=collections.defaultdict(collections.Counter); SUB=collections.defaultdict(collections.Counter)
def learn_type(e,p):
    if not e or not p: return
    el,_,er=e.partition(' — '); pl,_,pr=p.partition(' — ')
    TY[el][pl]+=1
    a,b=er.split(),pr.split()
    if a and len(a)==len(b):
        for x,y in zip(a,b): SUB[x][y]+=1
def learn(en_text,pt_text,en_name,pt_name):
    en=[l for l in (clean(x) for x in en_text.split('\n')) if l]
    pp=[l for l in (clean(x) for x in pt_text.split('\n')) if l]
    if len(en)!=len(pp): return
    for a,b in zip(en,pp):
        ta,na=tmpl_en(a,en_name)
        for n in names(pt_name)+names(en_name): b=b.replace(n,'~')
        tb,nb=absn(b)
        if na!=nb or ta.count('~')!=tb.count('~'): continue
        TM[ta][tb]+=1
        sa,sb=sents(ta),sents(tb)
        if len(sa)==len(sb):
            for x,y in zip(sa,sb):
                if x.count('#')!=y.count('#') or x.count('~')!=y.count('~'): continue
                if len(sa)>1: TM[x][y]+=1
                TM['E:'+lc(x)]['E:'+lc(y)]+=1
                ga,gb=segs(x),segs(y)
                if ga and gb and ga[0][0]==gb[0][0]:
                    for u,v in zip(ga,gb):
                        if u.count('#')==v.count('#') and u.count('~')==v.count('~'): TM[u][v]+=1
                    if ga[0][0]=='C':
                        ca,cb=ga[0][2:].split(', '),gb[0][2:].split(', ')
                        if len(ca)==len(cb):
                            for u,v in zip(ca,cb):
                                if u.count('#')==v.count('#') and u.count('~')==v.count('~') and (u.startswith('{'))==(v.startswith('{')): TM['K:'+u]['K:'+v]+=1
for c in cards:
    if c.get('tk'): continue
    p=pt.get(c['n'])
    if not p or p.get('s','0')<'2012': 
        if p and not c.get('f'): learn_type(c.get('t'),p.get('t'))
        continue
    if c.get('f') and p.get('f') and len(c['f'])==len(p['f']):
        for a,b in zip(c['f'],p['f']):
            learn_type(a.get('t'),b.get('t'))
            if a.get('x') and b.get('x'): learn(a['x'],b['x'],a['n'],b.get('n',a['n']))
    elif not c.get('f') and p.get('x'):
        learn_type(c.get('t'),p.get('t'))
        if c.get('x'): learn(c['x'],p['x'],c['n'],p.get('n',c['n']))
best={k:v.most_common(1)[0][0] for k,v in TM.items()}
TYb={k:v.most_common(1)[0][0] for k,v in TY.items()}; SUBb={k:v.most_common(1)[0][0] for k,v in SUB.items()}
print('modelos',len(best),'tipos',len(TYb),'subtipos',len(SUBb),file=sys.stderr)
def tr_type(t):
    if ' // ' in t: 
        r=[tr_type(x) for x in t.split(' // ')]; return None if None in r else ' // '.join(r)
    l,_,r=t.partition(' — ')
    if l not in TYb: return None
    out=TYb[l]
    if r:
        ws=[SUBb.get(w) for w in r.split()]
        if None in ws: return None
        out+=' — '+' '.join(ws)
    return out
def tr_piece(t,nums_unused=None):
    return best.get(t)
def tr_line(line,name):
    t,nums=tmpl_en(line,name)
    r=best.get(t)
    def sent(x):
        if x in best: return best[x]
        g=segs(x)
        if g:
            if g[0][0]=='C':
                a=best.get(g[0])
                if a is None:
                    ks=[k if re.fullmatch(r'(\{[^}]+\})+',k) else best.get('K:'+k) for k in g[0][2:].split(', ')]
                    if None not in ks: a='C:'+', '.join(k[2:] if k.startswith('K:') else k for k in ks)
                b=best.get(g[1])
                if a and b: return a[2:]+': '+b[2:3].upper()+b[3:]
            else:
                a,b=best.get(g[0]),best.get(g[1])
                if a and b: return a[2:]+', '+b[2:]
        e=best.get('E:'+lc(x))
        if e: return e[2:3].upper()+e[3:]
        return None
    if r is None:
        parts=sents(t)
        rs=[sent(x) for x in parts]
        if None not in rs: r=' '.join(rs)
    if r is None and re.fullmatch(r"[A-Za-z' ]+(, [A-Za-z' ]+)+",t):
        ks=[x.strip() for x in t.split(',')]
        rs=[best.get(k) or best.get(k[:1].upper()+k[1:]) for k in ks]
        if None not in rs: r=', '.join([rs[0]]+[x[:1].lower()+x[1:] for x in rs[1:]])
    if r is None: return None
    return fill(r,nums).replace('~',name.split(',')[0] if t.count('~') and name.split(',')[0] in line and name not in line else name)
def tr_text(x,name):
    out=[]
    for l in x.split('\n'):
        l2=clean(l)
        if not l2:
            if l.strip()=='—': out.append('—')
            continue
        r=tr_line(l2,name)
        if r is None: return None,l2
        out.append(r)
    return '\n'.join(out),None
if __name__=='__main__':
    miss=[c for c in cards if not c.get('tk') and not (pt.get(c['n']) and (pt[c['n']].get('x') or pt[c['n']].get('f') and any(f.get('x') for f in pt[c['n']]['f'])) )]
    ok=0;fail=collections.Counter();tyok=0
    for c in miss:
        faces=c.get('f') or [c]
        good=True
        for f in faces:
            if f.get('x'):
                r,bad=tr_text(f['x'],f['n'])
                if r is None: good=False; fail[tmpl_en(bad,f['n'])[0]]+=1
        ok+=good
        tyok+= tr_type(c.get('t',''))is not None
    print('sem oficial',len(miss),'traduzidas inteiras',ok,'tipos ok',tyok)
    print('modelos que faltam',len(fail))
    for t,n in fail.most_common(40): print(n,t)
