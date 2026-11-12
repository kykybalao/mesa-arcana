# Junta: texto oficial em português (Scryfall) + frases oficiais reaproveitadas (tm.py) + traduções manuais (build/manual_pt.json)
import gzip,json,sys,os,re
sys.argv=[sys.argv[0]]
import tm
pt,cards=tm.pt,tm.cards
manual=json.load(open('build/manual_pt.json',encoding='utf-8')) if os.path.exists('build/manual_pt.json') else {}
rank={n:i for i,n in enumerate(json.load(open('build/rank.json')))}
out={};st={'oficial':0,'auto':0,'manual':0,'falta':0};todo=[]
def face_pt(f,p):
    """f: face em inglês; p: face oficial (pode faltar). devolve (dict, completo?)"""
    d={};full=True;src=0
    if p and p.get('n') and p['n']!=f['n']: d['n']=p['n']
    t=(p or {}).get('t') or tm.tr_type(f.get('t',''))
    if t: d['t']=t
    if f.get('x'):
        if p and p.get('x'): d['x']=p['x']
        elif not tm.clean(f['x']): d['x']=f['x'].replace('Add ','Adicione ').replace(' or ',' ou ').replace(', ou ',' ou ')
        else:
            r,_=tm.tr_text(f['x'],f['n'])
            if r: d['x']=r;src=1
            elif r=='': d['x']=f['x'].replace('Add ','Adicione ').replace(' or ',' ou ').replace(', ou ',' ou ')
            else: full=False
    return d,full,src
for c in cards:
    if c.get('tk'): continue
    n=c['n'];p=pt.get(n) or {}
    if n in manual:
        d=dict(manual[n]);d['o']=2
        if p.get('n') and 'n' not in d and not c.get('f'): d['n']=p['n']
        if 't' not in d:
            t=p.get('t') or tm.tr_type(c.get('t',''))
            if t: d['t']=t
        out[n]=d;st['manual']+=1;continue
    if c.get('f'):
        pf=p.get('f') or []
        rs=[face_pt(f,pf[k] if k<len(pf) and len(pf)==len(c['f']) else None) for k,f in enumerate(c['f'])]
        full=all(r[1] for r in rs);src=max(r[2] for r in rs)
        d={'f':[r[0] for r in rs]}
    else:
        if c.get('x') and not p.get('x') and p.get('f') and all(f.get('x') for f in p['f']):
            # split/aventura: texto oficial por face, no mesmo formato do banco
            p=dict(p);p['x']='\n—\n'.join((f.get('n','')+'\n'+f['x']) for f in p['f'])
        d,full,src=face_pt(c,p)
    if not full:
        st['falta']+=1;todo.append((rank.get(n,99999),n))
        if not d or d=={'f':[{}]*len(c.get('f',[]))}: continue
        d['o']=3 # só nome/tipo; texto continua em inglês
    elif src: d['o']=1;st['auto']+=1
    else: st['oficial']+=1
    out[n]=d
ntk=0
for c in cards:
    if not c.get('tk'): continue
    d={}
    t=c.get('t','')
    if t.startswith('Token '):
        r=tm.tr_type(t[6:])
        if r: d['t']='Ficha de '+r
    elif t=='Emblem' or t.startswith('Emblem'): d['t']=t.replace('Emblem','Emblema')
    ws=[tm.SUBb.get(w) for w in c['n'].split()]
    if ws and None not in ws and ' — ' in t and c['n'] in t: d['n']=' '.join(ws)
    if c.get('x'):
        r,_=tm.tr_text(c['x'],c['n'])
        if r:
            d['x']=r.replace(c['n'],d['n']) if d.get('n') else r
            d['o']=1
        elif d: d['o']=3
    if d: out['#'+c['i']]=d;ntk+=1
print('fichas',ntk)
todo.sort()
json.dump([n for r,n in todo],open('build/todo_pt.json','w'),ensure_ascii=False)
with gzip.open('dados/pt.json.gz','wt',encoding='utf-8',compresslevel=9) as f: json.dump(out,f,ensure_ascii=False,separators=(',',':'))
print(st,'| faltam no top 1000/3000/6000/12250:',[sum(1 for r,n in todo if r<k) for k in (1000,3000,6000,12250)],os.path.getsize('dados/pt.json.gz'))
