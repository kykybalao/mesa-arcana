# Monta o banco de texto das cartas pela API do Scryfall (paginado, com pausa entre chamadas).
import json, time, urllib.request, urllib.parse, sys, gzip
H={'User-Agent':'MesaArcana/1.0','Accept':'application/json'}
def get(url):
    for a in range(5):
        try:
            with urllib.request.urlopen(urllib.request.Request(url,headers=H),timeout=60) as r: return json.load(r)
        except Exception as e:
            print('retry',e,file=sys.stderr); time.sleep(2+2*a)
    raise SystemExit('falhou '+url)
def face(f):
    d={'n':f.get('name',''),'t':f.get('type_line',''),'c':f.get('mana_cost',''),'x':f.get('oracle_text','')}
    for k,s in (('power','p'),('toughness','o'),('loyalty','l')):
        if f.get(k) is not None: d[s]=f[k]
    return {k:v for k,v in d.items() if v!='' or k=='n'}
def norm(c):
    fs=c.get('card_faces') or []
    if len(fs)>1 and fs[0].get('image_uris'):
        d={'i':c['id'],'n':c['name'],'t':c.get('type_line',''),'f':[face(f) for f in fs]}
        img=fs[0]['image_uris']['normal']
    else:
        d=face(c); d['i']=c['id']
        if fs:
            d['c']=c.get('mana_cost') or ' // '.join(f.get('mana_cost','') for f in fs if f.get('mana_cost'))
            d['x']=c.get('oracle_text') or '\n—\n'.join(f.get('name','')+(' '+f['mana_cost'] if f.get('mana_cost') else '')+'\n'+f.get('oracle_text','') for f in fs)
            for k,s in (('power','p'),('toughness','o'),('loyalty','l')):
                if s not in d and fs[0].get(k) is not None: d[s]=fs[0][k]
        img=(c.get('image_uris') or {}).get('normal')
    if not img: d['ni']=1
    elif '/%s/%s/%s.jpg'%(c['id'][0],c['id'][1],c['id']) not in img: d['u']=img
    k=[p['id'] for p in c.get('all_parts',[]) if p.get('component')=='token' and p['id']!=c['id']]
    if k: d['k']=k
    return d
def run(q,extra,out):
    url='https://api.scryfall.com/cards/search?'+urllib.parse.urlencode({'q':q,**extra})
    n=0
    while url:
        j=get(url)
        for c in j['data']: out.append(norm(c))
        n+=1; print(q,'pagina',n,len(out),'/',j.get('total_cards'),flush=True)
        url=j.get('next_page') if j.get('has_more') else None
        time.sleep(0.2)
cards=[];toks=[]
run('game:paper',{'unique':'cards','order':'name'},cards)
run('(t:token or t:emblem) game:paper',{'unique':'prints','include_extras':'true','order':'name'},toks)
need={i for c in cards for i in c.get('k',[])}
toks=[t for t in toks if t['i'] in need]
have={t['i'] for t in toks}
for c in cards:
    if 'k' in c:
        c['k']=[i for i in c['k'] if i in have]
        if not c['k']: del c['k']
for t in toks: t['tk']=1
with gzip.open(sys.argv[1],'wt',encoding='utf-8',compresslevel=9) as f:
    json.dump({'data':time.strftime('%Y-%m-%d'),'cartas':cards+toks},f,ensure_ascii=False,separators=(',',':'))
print('FIM',len(cards),len(toks))
