# Baixa os combos do Commander Spellbook (API paginada) e as listas de cartas que pesam na bracket
# (Game Changers, turnos extras, destruição em massa de terrenos, tutores) do Scryfall.
import json, time, urllib.request, urllib.parse, sys, gzip
H={'User-Agent':'MesaArcana/1.0','Accept':'application/json'}
def get(url):
    for a in range(6):
        try:
            with urllib.request.urlopen(urllib.request.Request(url,headers=H),timeout=90) as r: return json.load(r)
        except Exception as e:
            print('retry',e,file=sys.stderr,flush=True); time.sleep(3+3*a)
    raise SystemExit('falhou '+url)
def cut(s,n=160):
    s=' '.join((s or '').split()); return s if len(s)<=n else s[:n-1]+'…'
out={}
def run(q,order,maxn):
    off=0
    while off<maxn:
        j=get('https://backend.commanderspellbook.com/variants/?'+urllib.parse.urlencode({'limit':100,'offset':off,'q':q,'ordering':order}))
        rs=j['results']
        for v in rs:
            if v.get('status') not in ('OK',None) or v.get('spoiler'): continue
            d={'c':[u['card']['name']+('*' if u.get('mustBeCommander') else '') for u in v['uses']],
               'p':[p['feature']['name'] for p in v['produces']][:6],'b':v.get('bracketTag',''),'i':v.get('identity',''),'n':v.get('popularity') or 0}
            t=[(str(r['quantity'])+'x ' if r['quantity']>1 else '')+r['template']['name'] for r in v['requires']]
            if t: d['t']=t
            if v.get('notablePrerequisites'): d['r']=cut(v['notablePrerequisites'])
            if v.get('manaNeeded'): d['m']=cut(v['manaNeeded'],40)
            if v.get('description'): d['d']=v['description'].strip()[:900]
            if v.get('easyPrerequisites'): d['e']=cut(v['easyPrerequisites'],220)
            out[v['id']]=d
        off+=100
        if off%2000==0 or not j.get('next'): print(q,off,len(out),flush=True)
        if not j.get('next') or not rs: break
        time.sleep(0.25)
run('legal:commander cards=2','-popularity',10**9)
run('legal:commander cards=3','-popularity',10**9)
run('legal:commander cards>3','-popularity',10000)
flags={}
for key,q in (('gc','is:gamechanger'),('et','otag:extra-turn'),('mld','otag:mass-land-denial'),('tutor','otag:tutor')):
    names=[]; url='https://api.scryfall.com/cards/search?'+urllib.parse.urlencode({'q':q+' game:paper','unique':'cards'})
    while url:
        j=get(url); names+=[c['name'] for c in j['data']]; url=j.get('next_page') if j.get('has_more') else None; time.sleep(0.2)
    flags[key]=names; print(key,len(names),flush=True)
with gzip.open(sys.argv[1],'wt',encoding='utf-8',compresslevel=9) as f:
    json.dump({'data':time.strftime('%Y-%m-%d'),'combos':[dict(v,id=k) for k,v in out.items()],'listas':flags},f,ensure_ascii=False,separators=(',',':'))
print('FIM',len(out))
