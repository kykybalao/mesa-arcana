# Baixa o nome, tipo e texto impressos em português (impressão mais nova de cada carta) pela API do Scryfall.
import json, time, urllib.request, urllib.parse, sys, gzip
H={'User-Agent':'MesaArcana/1.0','Accept':'application/json'}
def get(url):
    for a in range(6):
        try:
            with urllib.request.urlopen(urllib.request.Request(url,headers=H),timeout=60) as r: return json.load(r)
        except Exception as e:
            print('retry',e,file=sys.stderr,flush=True); time.sleep(2+3*a)
    raise SystemExit('falhou '+url)
def face(f):
    d={'n':f.get('printed_name',''),'t':f.get('printed_type_line',''),'x':f.get('printed_text','')}
    return {k:v for k,v in d.items() if v}
out={}
url='https://api.scryfall.com/cards/search?'+urllib.parse.urlencode({'q':'lang:pt game:paper prefer:newest','unique':'cards','order':'name'})
n=0
while url:
    j=get(url)
    for c in j['data']:
        fs=c.get('card_faces') or []
        d=face(c)
        if fs: d['f']=[face(f) for f in fs]
        d['s']=c.get('released_at','')[:4]
        if d.get('n') or d.get('x') or fs: out[c['name']]=d
    n+=1
    if n%10==0 or not j.get('has_more'): print('pagina',n,len(out),'/',j.get('total_cards'),flush=True)
    url=j.get('next_page') if j.get('has_more') else None
    time.sleep(0.15)
with gzip.open(sys.argv[1],'wt',encoding='utf-8',compresslevel=9) as f:
    json.dump(out,f,ensure_ascii=False,separators=(',',':'))
print('FIM',len(out))
