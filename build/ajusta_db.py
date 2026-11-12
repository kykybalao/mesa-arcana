# Pós-processa o banco: terrenos básicos usam a arte do Bob Ross (Secret Lair "Happy Little Gathering")
# e cada tipo de ficha fica com uma única impressão (uma imagem por ficha).
import gzip, json, sys
BOB = {'Plains':'e20726fa-d99b-4e3d-8054-b7d313ceb02f','Island':'1d433685-601d-4da2-9e4c-796646003ffe','Swamp':'de8e7cd9-354d-4b2e-9823-79d6e8eed7e4',
       'Mountain':'03683fbb-9843-4c14-bb95-387150e97c90','Forest':'c0e38987-e257-4da0-b270-bc48404947cd'}
p = sys.argv[1]
db = json.load(gzip.open(p, 'rt', encoding='utf-8'))
cs = db['cartas']
for c in cs:
    if not c.get('tk') and c['n'] in BOB:
        c['i'] = BOB[c['n']]; c.pop('u', None); c.pop('ni', None)
def key(t):
    f = t['f'][0] if t.get('f') else t
    return '|'.join([t['n'], f.get('t',''), f.get('p',''), f.get('o',''), f.get('x','')])
canon, remap = {}, {}
for t in sorted((c for c in cs if c.get('tk')), key=lambda t: (t.get('ni', 0), t['i'])):
    remap[t['i']] = canon.setdefault(key(t), t['i'])
for c in cs:
    if 'k' in c:
        c['k'] = list(dict.fromkeys(remap[i] for i in c['k'] if i in remap))
keep = set(canon.values())
db['cartas'] = [c for c in cs if not c.get('tk') or c['i'] in keep]
with gzip.open(p, 'wt', encoding='utf-8', compresslevel=9) as f:
    json.dump(db, f, ensure_ascii=False, separators=(',', ':'))
print('fichas:', len(remap), '->', len(keep))
