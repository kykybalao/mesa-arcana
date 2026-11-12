# Traduz para português os resultados e os passos dos combos do Commander Spellbook.
# Os passos seguem um vocabulário bem repetitivo; um combo só troca de idioma quando TODOS os passos foram entendidos.
import gzip, json, re, sys, collections

N = r'(?:«\d+»)'
M = r'(?:(?:\{[^}]+\})+)'
NUMW = {'a': 'um', 'an': 'um', 'one': 'um', 'two': 'dois', 'three': 'três', 'four': 'quatro', 'five': 'cinco', 'six': 'seis', 'seven': 'sete', 'ten': 'dez'}
ORD = {'first': 'primeira', 'second': 'segunda', 'third': 'terceira', 'last': 'última', 'fourth': 'quarta'}
ORDM = {'first': 'primeiro', 'second': 'segundo', 'third': 'terceiro', 'last': 'último', 'fourth': 'quarto'}
KW = {'persist': 'persistir', 'undying': 'imortal', 'modular': 'modular', 'equip': 'equipar', 'fabricate': 'fabricar', 'earthbend': 'dobrar terra',
      'soulbond': 'elo de almas', 'ninjutsu': 'ninjutsu', 'evoke': 'evocar', 'cascade': 'cascata', 'landfall': 'aterragem', 'enters': 'entrada', 'dies': 'morte',
      'ETB': 'entrada', 'death': 'morte', 'crew': 'tripular', 'unearth': 'desenterrar', 'encore': 'bis', 'dredge': 'dragar', 'storm': 'rajada', 'magecraft': 'artimagia'}
TOK = {'Treasure': 'Tesouro', 'Clue': 'Pista', 'Food': 'Comida', 'Blood': 'Sangue', 'Servo': 'Servo', 'Zombie': 'Zumbi', 'Saproling': 'Saprófita', 'Squirrel': 'Esquilo',
       'Soldier': 'Soldado', 'Spirit': 'Espírito', 'Insect': 'Inseto', 'Ally': 'Aliado', 'Beast': 'Besta', 'Merfolk': 'Tritão', 'Eldrazi Spawn': 'Cria Eldrazi',
       'Goblin': 'Goblin', 'Elf': 'Elfo', 'Warrior': 'Guerreiro', 'Human': 'Humano', 'Thopter': 'Tóptero', 'Cat': 'Felino', 'Faerie': 'Fada', 'Rat': 'Rato',
       'Vampire': 'Vampiro', 'Elemental': 'Elemental', 'Bird': 'Ave', 'Wolf': 'Lobo', 'Knight': 'Cavaleiro', 'Angel': 'Anjo', 'Dragon': 'Dragão', 'Phyrexian Mite': 'Ácaro Phyrexiano',
       'Eldrazi Scion': 'Herdeiro Eldrazi', 'Plant': 'Planta', 'Snake': 'Serpente', 'Wizard': 'Mago', 'Citizen': 'Cidadão', 'Construct': 'Constructo', 'Myr': 'Myr', 'Powerstone': 'Pedra de Poder',
       'Gold': 'Ouro', 'Map': 'Mapa', 'Army': 'Exército', 'Forest': 'Floresta', 'Rabbit': 'Coelho', 'Fish': 'Peixe', 'Lander': 'Lander', 'Shard': 'Fragmento', 'Germ': 'Germe', 'Golem': 'Golem'}
TOKRE = '(' + '|'.join(sorted(map(re.escape, TOK), key=len, reverse=True)) + ')'


def num(w):
    return NUMW.get(w, w)


OBJ = [
    (rf'({N})', r'\1'), (r'it', 'ele'), (r'itself', 'a si mesmo'), (r'them', 'eles'), (rf'({N}) and ({N})', r'\1 e \2'), (rf'it and ({N})', r'ele e \1'),
    (rf'({N}), ({N}),? and ({N})', r'\1, \2 e \3'),
    (r'another creature', 'outra criatura'), (r'another creature you control', 'outra criatura que você controla'),
    (r'another nonland creature( you control)?', 'outra criatura que não seja terreno'), (r'any other creature', 'qualquer outra criatura'),
    (r'a creature( you control)?', 'uma criatura'), (r'the creature', 'a criatura'), (r'that creature', 'aquela criatura'),
    (r'the creature token', 'a ficha de criatura'), (r'a creature token', 'uma ficha de criatura'), (r'the token', 'a ficha'), (r'a token', 'uma ficha'),
    (r'the land', 'o terreno'), (r'a land', 'um terreno'), (r'the fetched land', 'o terreno buscado'), (r'the earthbent land', 'o terreno com terra dobrada'),
    (r'the persist creature|the creature with persist', 'a criatura com persistir'), (r'the undying creature|the creature with undying', 'a criatura com imortal'),
    (rf'the ({N}) tokens?', r'a ficha de \1'), (rf'the token ({N})', r'a ficha de \1'), (rf'the copy of ({N})|the ({N}) copy', r'a cópia de \1\2'),
    (rf'(?:the|a) {TOKRE}(?: token)?', lambda m: 'o ' + TOK[m.group(1)]), (rf'(?:the|both|two) {TOKRE}s(?: tokens)?', lambda m: 'os ' + TOK[m.group(1)] + 's'),
    (r'all creatures( you control)?', 'todas as criaturas que você controla'), (r'all lands you control', 'todos os terrenos que você controla'),
    (r'up to (\w+) lands', lambda m: 'até ' + num(m.group(1)) + ' terrenos'), (r'(five|seven|three|four|two) lands', lambda m: num(m.group(1)) + ' terrenos'),
    (r'a card', 'um card'), (r'an artifact', 'um artefato'), (r'another artifact', 'outro artefato'), (r'a permanent', 'uma permanente'), (r'another permanent', 'outra permanente'),
    (r'any target', 'qualquer alvo'), (r'each opponent', 'cada oponente'), (r'an opponent|target opponent', 'um oponente'),
    (r'a mana-producing permanent', 'uma permanente que gera mana'), (r'the mana-producing permanent', 'a permanente que gera mana'),
    (r'any applicable target', 'qualquer alvo válido'), (r'each creature you control', 'cada criatura que você controla'), (r'a creature you control', 'uma criatura que você controla'),
    (r'another target creature', 'outra criatura alvo'), (r'another nontoken creature you control', 'outra criatura que não seja ficha que você controla'),
    (r'the animated Forest', 'a Floresta animada'), (r'the targeted creature', 'a criatura alvo'),
]
OBJ = [(re.compile(a), b) for a, b in OBJ]


def obj(s):
    for rx, rep in OBJ:
        m = rx.fullmatch(s)
        if m:
            return rep(m) if callable(rep) else m.expand(rep)
    return None


def zone(z):
    return {'your graveyard': 'seu cemitério', 'the battlefield': 'campo de batalha', 'exile': 'exílio', 'your hand': 'sua mão', 'the top of your library': 'topo de seu grimório',
            'your library': 'seu grimório', 'the command zone': 'zona de comando'}.get(z)


def de(z):  # "de" + zona
    return {'seu cemitério': 'de seu cemitério', 'campo de batalha': 'do campo de batalha', 'exílio': 'do exílio', 'sua mão': 'de sua mão', 'topo de seu grimório': 'do topo de seu grimório',
            'seu grimório': 'de seu grimório', 'zona de comando': 'da zona de comando'}[z]


def para(z):
    return {'seu cemitério': 'para seu cemitério', 'campo de batalha': 'para o campo de batalha', 'exílio': 'para o exílio', 'sua mão': 'para sua mão', 'topo de seu grimório': 'para o topo de seu grimório',
            'seu grimório': 'para seu grimório', 'zona de comando': 'para a zona de comando'}[z]


def counters(q, kind):
    q = num(q)
    if q == 'um':
        return f'um marcador {kind}'
    return f'{q} marcadores {kind}'


CT = r'([+\-−]\d+/[+\-−]\d+|charge|loyalty|stun|oil|time)'


def ctname(k):
    return {'charge': 'de carga', 'loyalty': 'de lealdade', 'stun': 'de atordoamento', 'oil': 'de óleo', 'time': 'de tempo'}.get(k, k)


def token_desc(m):
    """a 1/1 Zombie creature token with flying"""
    return None


# gerúndios (custos e efeitos)
def ger(s):
    s = s.strip()
    if s.startswith('and '):
        r = ger(s[4:])
        return r and 'e ' + r
    if s.startswith('then '):
        r = ger(s[5:])
        return r and 'depois ' + r
    for rx, fn in GER:
        m = rx.fullmatch(s)
        if m:
            r = fn(m)
            if r:
                return r
    if ' and ' in s:
        a, b = s.split(' and ', 1)
        ra, rb = ger(a), ger(b)
        if ra and rb:
            return ra + ' e ' + rb
    return None


def o2(f):
    def g(m):
        x = obj(m.group(1))
        if not x:
            return None
        r = f(x, m)
        return r.replace('tendo como alvo a si mesmo', 'tendo a si mesmo como alvo')
    return g


def fichas(q, what):
    q = num(q)
    if q in ('um', 'uma'):
        return 'uma ficha ' + what
    return {'dois': 'duas'}.get(q, q) + ' fichas ' + what


def causing(m):
    r = imp(m.group(1))
    return r and 'fazendo você ' + r


def imp(s):
    """infinitivo depois de "causing you to" """
    T = [
        (r'gain (\d+) life', lambda m: f'ganhar {m.group(1)} de vida'), (r'gain life', lambda m: 'ganhar pontos de vida'), (r'lose (\d+) life', lambda m: f'perder {m.group(1)} de vida'),
        (r'draw a card', lambda m: 'comprar um card'), (r'draw (\w+) cards', lambda m: f'comprar {num(m.group(1))} cards'),
        (r'take an extra turn after this one', lambda m: 'jogar um turno extra depois deste'), (r'take two extra turns after this one', lambda m: 'jogar dois turnos extras depois deste'),
        (r'scry (\d+)', lambda m: f'usar vidência {m.group(1)}'), (r'surveil (\d+)', lambda m: f'vigiar {m.group(1)}'), (r'win the game', lambda m: 'vencer o jogo'),
        (r'loot', lambda m: 'comprar um card e descartar um card'), (r'sacrifice (.+)', lambda m: (lambda x: x and 'sacrificar ' + x)(obj(m.group(1)))),
        (r'discard a card', lambda m: 'descartar um card'), (r'mill (\w+) cards?', lambda m: f'triturar {num(m.group(1))} cards'),
        (r'untap all creatures you control and to get an additional combat phase after this one', lambda m: 'desvirar todas as criaturas que você controla e ganhar uma fase de combate adicional depois desta'),
        (rf'get ({M})', lambda m: 'ganhar ' + m.group(1)), (r'exile it', lambda m: 'exilá-lo'),
        (r'gain (\d+) life and (target|each) opponent to lose (\d+) life', lambda m: f'ganhar {m.group(1)} de vida e {"cada" if m.group(2)=="each" else "o"} oponente{"" if m.group(2)=="each" else " alvo"} perder {m.group(3)} de vida'),
    ]
    for a, f in T:
        m = re.fullmatch(a, s)
        if m:
            return f(m)
    return None


GER = [
    (rf'adding ({M})', lambda m: 'adicionando ' + m.group(1)), (rf'adding at least ({M})', lambda m: 'adicionando pelo menos ' + m.group(1)),
    (rf'adding an amount of ({M})', lambda m: 'adicionando uma quantidade de ' + m.group(1)), (rf'adding (\w+) ({M})', lambda m: f'adicionando {num(m.group(1))} ' + m.group(2)),
    (r'adding (at least )?(\w+) mana of any (one )?color', lambda m: f'adicionando {"pelo menos " if m.group(1) else ""}{num(m.group(2))} manas de qualquer cor'),
    (rf'adding at least enough mana to cast ({N})', lambda m: 'adicionando mana suficiente para conjurar ' + m.group(1)),
    (r'tapping (.+)', o2(lambda x, m: 'virando ' + x)), (r'untapping (.+)', o2(lambda x, m: 'desvirando ' + x)),
    (r'tapping and sacrificing (.+)', o2(lambda x, m: 'virando e sacrificando ' + x)),
    (r'sacrificing (.+)', o2(lambda x, m: 'sacrificando ' + x)), (r'targeting (.+)', o2(lambda x, m: 'tendo como alvo ' + x)),
    (r'blinking (.+)', o2(lambda x, m: 'exilando e devolvendo ' + x + ' ao campo de batalha')), (r'copying (.+)', o2(lambda x, m: 'copiando ' + x)),
    (rf"copying ({N})'s ability", lambda m: 'copiando a habilidade de ' + m.group(1)),
    (r'exiling (.+)', o2(lambda x, m: 'exilando ' + x)), (r'triggering (.+)', o2(lambda x, m: 'desencadeando ' + x)), (r'triggering its ability', lambda m: 'desencadeando a habilidade dele'),
    (r'airbending (.+)', o2(lambda x, m: 'dobrando ar em ' + x)), (r'earthbending (.+)', o2(lambda x, m: 'dobrando terra em ' + x)),
    (rf'paying ({M})', lambda m: 'pagando ' + m.group(1)), (r'paying (\d+) life', lambda m: f'pagando {m.group(1)} de vida'), (r'paying life', lambda m: 'pagando pontos de vida'),
    (r'paying its mana cost', lambda m: 'pagando seu custo de mana'), (r'discarding a card', lambda m: 'descartando um card'),
    (rf'removing (a|an|\w+|all) {CT} counters? (?:from|on) (.+)', lambda m: (lambda x: x and f'removendo {"todos os marcadores "+ctname(m.group(2)) if m.group(1)=="all" else counters(m.group(1), ctname(m.group(2)))} de ' + x)(obj(m.group(3)))),
    (rf'putting (a|an|\w+|at least one) {CT} counters? on (.+)', lambda m: (lambda x: x and f'colocando {"pelo menos um marcador "+ctname(m.group(2)) if m.group(1)=="at least one" else counters(m.group(1), ctname(m.group(2)))} em ' + x)(obj(m.group(3)))),
    (r'creating (a|an|two|three|\w+) creature tokens?', lambda m: 'criando ' + fichas(m.group(1), 'de criatura')), (r'creating at least one creature token', lambda m: 'criando pelo menos uma ficha de criatura'),
    (r'creating a token', lambda m: 'criando uma ficha'), (rf'creating (a|an|two|three|\w+) {TOKRE}(?: artifact)? tokens?', lambda m: 'criando ' + fichas(m.group(1), 'de ' + TOK[m.group(2)])),
    (r'creating a creature token and a Treasure token', lambda m: 'criando uma ficha de criatura e uma ficha de Tesouro'),
    (rf'creating (a|two|three|\w+) token cop(?:y|ies) of ({N})( with haste)?', lambda m: 'criando ' + fichas(m.group(1), 'que ' + ('é' if m.group(1) in ('a', 'an', 'one') else 'são') + ' cópia de ' + m.group(2)) + (' com ímpeto' if m.group(3) else '')),
    (rf'creating a copy of ({N})', lambda m: 'criando uma cópia de ' + m.group(1)),
    (rf'creating (a|an|two|three|\w+) (\d+/\d+) {TOKRE}(?: artifact)? creature tokens?( with flying)?( and haste| with haste)?', lambda m: 'criando ' + fichas(m.group(1), f'de criatura {TOK[m.group(3)]} {m.group(2)}') + (' com voar' if m.group(4) else '') + ((' e ímpeto' if m.group(4) else ' com ímpeto') if m.group(5) else '')),
    (r'returning (.+?) from (your graveyard|the battlefield|exile) to (the battlefield|your hand)( tapped)?(?: with (a|an|two) ' + CT + r' counters? on it)?',
     lambda m: (lambda x: x and f'devolvendo {x} {de(zone(m.group(2)))} {para(zone(m.group(3)))}{" virado" if m.group(4) else ""}{" com "+counters(m.group(5), ctname(m.group(6))) if m.group(5) else ""}')(obj(m.group(1)))),
    (r'returning (.+?) to (your hand|the battlefield)(?: from (your graveyard|the battlefield|exile))?', lambda m: (lambda x: x and f'devolvendo {x} {para(zone(m.group(2)))}{" "+de(zone(m.group(3))) if m.group(3) else ""}')(obj(m.group(1)))),
    (r'returning (.+?) from (your graveyard) to the battlefield attached to (.+)', lambda m: (lambda x, y: x and y and f'devolvendo {x} de seu cemitério para o campo de batalha anexado a {y}')(obj(m.group(1)), obj(m.group(3)))),
    (r'returning (.+?) from your graveyard to the battlefield without a ' + CT + r' counter on it due to (.+)', lambda m: (lambda x: x and f'devolvendo {x} de seu cemitério para o campo de batalha sem marcador {m.group(2)} por causa de {"seu efeito de substituição" if m.group(3)=="its replacement effect" else (obj(m.group(3)) or "")}')(obj(m.group(1))) if m.group(3) == 'its replacement effect' or obj(m.group(3)) else None),
    (rf'putting ({N}) from your hand onto the battlefield', lambda m: f'colocando {m.group(1)} de sua mão no campo de batalha'), (rf'putting ({N}) onto the battlefield', lambda m: f'colocando {m.group(1)} no campo de batalha'),
    (rf'putting ({N}) from your graveyard onto the top of your library|putting ({N}) on top of your library from your graveyard', lambda m: f'colocando {m.group(1) or m.group(2)} de seu cemitério no topo de seu grimório'),
    (r'causing you to (.+)', causing),
    (r'causing each (opponent|player) to lose (\d+) life( and you to gain (\d+) life)?', lambda m: f'fazendo cada {"oponente" if m.group(1)=="opponent" else "jogador"} perder {m.group(2)} de vida' + (f' e você ganhar {m.group(4)} de vida' if m.group(3) else '')),
    (r'dealing (\d+) damage to (.+)', lambda m: (lambda x: x and f'causando {m.group(1)} de dano a {x}')(obj(m.group(2)))),
    (r'giving (.+?) ([+\-]\d+/[+\-]\d+) until end of turn', lambda m: (lambda x: x and f'dando {m.group(2)} a {x} até o final do turno')(obj(m.group(1)))),
    (r'attaching it to (.+)', o2(lambda x, m: 'anexando-o a ' + x)), (rf'attach(?:ing)? ({N}) to (.+)', lambda m: (lambda x: x and f'anexando {m.group(1)} a {x}')(obj(m.group(2)))),
    (r'with the copy targeting (.+)', o2(lambda x, m: 'com a cópia tendo ' + x + ' como alvo')),
    (rf'causing you to pay ({M}) to create a token copy of ({N})', lambda m: f'fazendo você pagar {m.group(1)} para criar uma ficha que é cópia de {m.group(2)}'),
    (rf'causing you to pay ({M}) to create a (\d+/\d+) {TOKRE}(?: artifact)? creature token', lambda m: f'fazendo você pagar {m.group(1)} para criar uma ficha de criatura {TOK[m.group(3)]} {m.group(2)}'),
    (rf'causing you to draw a card and put ({N}) on top of your library', lambda m: f'fazendo você comprar um card e colocar {m.group(1)} no topo de seu grimório'),
    (r'choosing not to put anything from your hand onto the battlefield', lambda m: 'escolhendo não colocar nada de sua mão no campo de batalha'),
    (rf'it, ({N}),? and ({N}) trigger', lambda m: f'as habilidades dele, de {m.group(1)} e de {m.group(2)} são desencadeadas'),
    (rf'casting a copy of ({N}) without paying its mana cost', lambda m: f'conjurando uma cópia de {m.group(1)} sem pagar seu custo de mana'),
    (r'if applicable', lambda m: 'se for o caso'), (r'if needed', lambda m: 'se precisar'), (r'rounded up', lambda m: 'arredondado para cima'), (r'choosing not to pay', lambda m: 'escolhendo não pagar'),
    (r'taking an extra turn after this one', lambda m: 'jogando um turno extra depois deste'), (r'turning it face up', lambda m: 'virando-o com a face para cima'),
    (r'this combo nets infinite mana', lambda m: 'este combo rende mana infinito'), (r'drawing a card', lambda m: 'comprando um card'),
    (r'doubling each counter on (.+)', o2(lambda x, m: 'dobrando cada marcador em ' + x)),
    # orações que aparecem depois da vírgula
    (r'it triggers( twice)?', lambda m: 'a habilidade dele é desencadeada' + (' duas vezes' if m.group(1) else '')), (r'they trigger', lambda m: 'as habilidades deles são desencadeadas'),
    (rf'({N}) triggers( twice)?', lambda m: f'a habilidade de {m.group(1)} é desencadeada' + (' duas vezes' if m.group(2) else '')),
    (rf'each ({N}) triggers', lambda m: f'a habilidade de cada {m.group(1)} é desencadeada'),
    (rf'it and ({N}) trigger', lambda m: f'as habilidades dele e de {m.group(1)} são desencadeadas'),
    (rf'({N})(?:,| and) ({N})(?:,? and ({N}))?(?: each)? trigger', lambda m: 'as habilidades de ' + ' e '.join(x for x in m.groups() if x) + ' são desencadeadas'),
    (r'its (\w+) ability triggers( twice)?', lambda m: (f'a {ORD[m.group(1)]} habilidade dele é desencadeada' if m.group(1) in ORD else f'a habilidade de {KW[m.group(1)]} dele é desencadeada' if m.group(1) in KW else None) and ((f'a {ORD[m.group(1)]} habilidade dele é desencadeada' if m.group(1) in ORD else f'a habilidade de {KW[m.group(1)]} dele é desencadeada') + (' duas vezes' if m.group(2) else ''))),
    (r'its ability triggers( twice)?', lambda m: 'a habilidade dele é desencadeada' + (' duas vezes' if m.group(1) else '')),
    (rf'its (\w+) ability and ({N}) trigger', lambda m: KW.get(m.group(1)) and f'a habilidade de {KW[m.group(1)]} dele e a de {m.group(2)} são desencadeadas'),
]
GER = [(re.compile(a), f) for a, f in GER]


def head(s):
    for rx, fn in HEAD:
        m = rx.fullmatch(s)
        if m:
            r = fn(m)
            if r:
                return r
    return None


def when_obj(x):
    return obj(x)


def act(m):
    who, how = m.group(1), m.group(2)
    r = ger(how)
    return r and f'Ative {who} {r}'


def abil(m):
    o = m.group(2)
    name = f'a {ORD[o]} habilidade de {m.group(1)}' if o in ORD else f'a habilidade de {KW[o]} de {m.group(1)}' if o in KW else None
    r = ger(m.group(3))
    return name and r and f'Ative {name} {r}'


def PL(m):
    return bool(m.group(3)) or ' tokens by' in m.group(0) or m.group(4).split(',')[0].endswith('them') or m.group(1) in ('both', 'two')


HEAD = [
    (r'Repeat', lambda m: 'Repita'), (r'Repeat from step (\d+)', lambda m: f'Repita a partir do passo {m.group(1)}'), (r'Repeat step (\d+)( as desired)?', lambda m: f'Repita o passo {m.group(1)}' + (' quantas vezes quiser' if m.group(2) else '')),
    (r'Repeat steps (\d+)(?:-| through | to | and )(\d+)', lambda m: f'Repita os passos {m.group(1)} a {m.group(2)}'),
    (r'Repeat each combat phase', lambda m: 'Repita a cada fase de combate'), (r'Repeat each turn', lambda m: 'Repita a cada turno'), (r'Repeat (any number of times|as desired)', lambda m: 'Repita quantas vezes quiser'),
    (r'Repeat an arbitrarily large number of times', lambda m: 'Repita um número arbitrariamente grande de vezes'), (r'Repeat each upkeep', lambda m: 'Repita a cada manutenção'),
    (rf'Resolve the ({N}) trigger(?: from step (\d+))?', lambda m: f'Resolva o gatilho de {m.group(1)}' + (f' do passo {m.group(2)}' if m.group(2) else '')),
    (rf'Resolve the ({N}) ability(?: from step (\d+))?', lambda m: f'Resolva a habilidade de {m.group(1)}' + (f' do passo {m.group(2)}' if m.group(2) else '')),
    (rf"Resolve ({N})'s (?:ability|trigger)", lambda m: f'Resolva a habilidade de {m.group(1)}'), (rf'Resolve ({N})', lambda m: f'Resolva {m.group(1)}'),
    (rf'Resolve the copy of ({N})|Resolve the ({N}) copy', lambda m: f'Resolva a cópia de {m.group(1) or m.group(2)}'),
    (rf'Resolve the (first|second|third|last) ({N}) (trigger|ability)', lambda m: f'Resolva {"o "+ORDM[m.group(1)]+" gatilho" if m.group(3)=="trigger" else "a "+ORD[m.group(1)]+" habilidade"} de {m.group(2)}'),
    (rf'Resolve (both|all) ({N}) (triggers|abilities)(?: from step (\d+)(?: and (\d+))?)?', lambda m: f'Resolva {"os dois" if m.group(1)=="both" and m.group(3)=="triggers" else "as duas" if m.group(1)=="both" else "todos os" if m.group(3)=="triggers" else "todas as"} {"gatilhos" if m.group(3)=="triggers" else "habilidades"} de {m.group(2)}' + (f' do passo {m.group(4)}' if m.group(4) else '') + (f' e {m.group(5)}' if m.group(5) else '')),
    (r'Resolve the (first|second|third|last) trigger', lambda m: f'Resolva o {ORDM[m.group(1)]} gatilho'), (r'Resolve the (\w+) trigger', lambda m: KW.get(m.group(1)) and f'Resolva o gatilho de {KW[m.group(1)]}'),
    (rf"Resolve the ({N}) token's trigger", lambda m: f'Resolva o gatilho da ficha de {m.group(1)}'),
    (rf'({N}) triggers( twice)?', lambda m: f'A habilidade de {m.group(1)} é desencadeada' + (' duas vezes' if m.group(2) else '')),
    (rf'The ({N}) token triggers', lambda m: f'A habilidade da ficha de {m.group(1)} é desencadeada'),
    (rf'({N})(?:,| and) ({N})(?:,? and ({N}))?(?: each)? trigger', lambda m: 'As habilidades de ' + ' e '.join(x for x in m.groups() if x) + ' são desencadeadas'),
    (rf"({N})'s? (\w+) ability triggers", lambda m: (f'A {ORD[m.group(2)]} habilidade de {m.group(1)} é desencadeada' if m.group(2) in ORD else f'A habilidade de {KW[m.group(2)]} de {m.group(1)} é desencadeada' if m.group(2) in KW else None)),
    (rf'Cast ({N})', lambda m: f'Conjure {m.group(1)}'), (rf'Cast ({N}) by paying ({M})(?: due to ({N}))?', lambda m: f'Conjure {m.group(1)} pagando {m.group(2)}' + (f' graças a {m.group(3)}' if m.group(3) else '')),
    (rf'Cast ({N}) by paying its mana cost', lambda m: f'Conjure {m.group(1)} pagando seu custo de mana'),
    (rf'Cast ({N}) from (your graveyard|exile|the top of your library|your hand) by paying (?:({M})|(\d+) life|its mana cost)(?: due to ({N}))?', lambda m: f'Conjure {m.group(1)} {de(zone(m.group(2)))} pagando {m.group(3) or (m.group(4)+" de vida" if m.group(4) else "seu custo de mana")}' + (f' graças a {m.group(5)}' if m.group(5) else '')),
    (rf'Cast ({N}) with buyback by paying ({M})', lambda m: f'Conjure {m.group(1)} com recompra pagando {m.group(2)}'),
    (rf'Cast ({N}) either from your hand or from the command zone by paying ({M}) plus commander tax', lambda m: f'Conjure {m.group(1)} de sua mão ou da zona de comando pagando {m.group(2)} mais o imposto de comandante'),
    (rf"Activate ({N})'s? (\w+) ability by (.+)", abil),
    (rf"Activate ({N}) using ({N})'s ability by (.+)", lambda m: (lambda r: r and f'Ative {m.group(1)} usando a habilidade de {m.group(2)}, {r}')(ger(m.group(3)))),
    (rf'Activate ({N}) by (.+)', act),
    (rf'Activate (the|a|both|two) {TOKRE}(s)?(?: tokens?)? by (.+)', lambda m: (lambda r: r and f'Ative {"os " if PL(m) else "o " if m.group(1) == "the" else "um "}{TOK[m.group(2)]}{"s" if PL(m) else ""} {r}')(ger(m.group(4)))),
    (r'Activate (the land|the creature|a mana-producing permanent|all mana-producing lands you control|any number of creatures|up to \w+ lands) by tapping (?:it|them)',
     lambda m: 'Vire ' + {'the land': 'o terreno', 'the creature': 'a criatura', 'a mana-producing permanent': 'uma permanente que gera mana', 'all mana-producing lands you control': 'todos os terrenos que geram mana que você controla', 'any number of creatures': 'qualquer número de criaturas'}.get(m.group(1), obj(m.group(1)) or '') + ' para gerar mana' if m.group(1) in ('the land', 'the creature', 'a mana-producing permanent', 'all mana-producing lands you control', 'any number of creatures') or obj(m.group(1)) else None),
    (r'Holding priority', lambda m: 'Mantendo a prioridade'), (r'In response', lambda m: 'Em resposta'), (r'If needed', lambda m: 'Se precisar'), (r'Once you have infinite mana', lambda m: 'Quando tiver mana infinito'),
    (r'When (.+?) (?:enters|enter)(?: the battlefield)?', lambda m: (lambda x: x and f'Quando {x} entrar no campo de batalha')(obj(m.group(1)))),
    (r'When (.+?) dies', lambda m: (lambda x: x and f'Quando {x} morrer')(obj(m.group(1)))), (r'When (.+?) leaves the battlefield', lambda m: (lambda x: x and f'Quando {x} deixar o campo de batalha')(obj(m.group(1)))),
    (r'At the beginning of your (next )?(end step|upkeep|combat phase|draw step)', lambda m: f'No início de sua {"próxima " if m.group(1) else ""}' + {'end step': 'etapa final', 'upkeep': 'manutenção', 'combat phase': 'fase de combate', 'draw step': 'etapa de compra'}[m.group(2)]),
    (rf'({N}|The creature|The ({N}) token) enters the battlefield', lambda m: f'{"A criatura" if m.group(1)=="The creature" else "A ficha de "+m.group(2) if m.group(2) else m.group(1)} entra no campo de batalha'),
    (rf'({N}|The creature) dies', lambda m: f'{"A criatura" if m.group(1)=="The creature" else m.group(1)} morre'), (rf'({N}) leaves the battlefield', lambda m: f'{m.group(1)} deixa o campo de batalha'),
    (r'Gain life', lambda m: 'Ganhe pontos de vida'), (r'Draw a card', lambda m: 'Compre um card'), (rf'Play ({N})', lambda m: f'Jogue {m.group(1)}'), (r'Play a land', lambda m: 'Jogue um terreno'),
    (r'Move to your next turn', lambda m: 'Passe para o seu próximo turno'), (r'Pass the turn', lambda m: 'Passe o turno'),
    (rf'Declare ({N}) as an attacker', lambda m: f'Declare {m.group(1)} como atacante'), (rf'Declare ({N}) and ({N}) as attackers', lambda m: f'Declare {m.group(1)} e {m.group(2)} como atacantes'),
    (rf'Deal combat damage to an opponent using ({N})', lambda m: f'Cause dano de combate a um oponente com {m.group(1)}'),
    (rf'Deal combat damage using the creature that has ({N}) attached', lambda m: f'Cause dano de combate com a criatura que tem {m.group(1)} anexado'),
    (rf'Declare the creature with ({N}) attached and ({N}) as attackers', lambda m: f'Declare a criatura com {m.group(1)} anexado e {m.group(2)} como atacantes'),
    (rf'Deal damage to ({N})', lambda m: f'Cause dano a {m.group(1)}'), (rf'Put a ([+\-]\d+/[+\-]\d+) counter on ({N})', lambda m: f'Coloque um marcador {m.group(1)} em {m.group(2)}'),
    (rf'Put ({N}) into your graveyard due to the legend rule', lambda m: f'Coloque {m.group(1)} em seu cemitério por causa da regra das lendas'),
    (rf'Animate ({N}) using ({N})', lambda m: f'Transforme {m.group(1)} em criatura usando {m.group(2)}'), (rf'Cause ({N}) to become an artifact using ({N})', lambda m: f'Faça {m.group(1)} se tornar um artefato usando {m.group(2)}'),
    (rf'Fetch a land using ({N})', lambda m: f'Busque um terreno usando {m.group(1)}'), (rf'The ({N}) ability fizzles due to no longer having a legal target', lambda m: f'A habilidade de {m.group(1)} não resolve por não ter mais um alvo válido'),
]
HEAD = [(re.compile(a), f) for a, f in HEAD]


def lower1(s):
    return s[:1].lower() + s[1:]


def step(s):
    """traduz um passo inteiro (já com nomes trocados por «n») ou devolve None"""
    out = []
    for sent in re.split(r'(?<=\.)\s+(?=[A-Z«])', s.strip()):
        sent = sent.rstrip('.').strip()
        if not sent:
            continue
        parts = re.split(r', (?=[a-z«]|[A-Z][a-z]+ triggers)', sent)
        r = head(parts[0])
        if r is None:
            return None
        res = [r]
        for p in parts[1:]:
            t = ger(p)
            if t is None:
                h = head(p[:1].upper() + p[1:])  # "..., activate X by ..." / "..., cast X ..."
                t = h and lower1(h)
            if t is None:
                return None
            res.append(t)
        out.append(', '.join(res) + '.')
    if not out:
        return None
    r = ' '.join(out)
    r = re.sub(r'(\w+ndo) e (\w+ndo) eles\b', r'\1-os e \2-os', r)
    r = re.sub(r'(\w+ndo) e (\w+ndo) ele\b', r'\1-o e \2-o', r)
    r = re.sub(r'(\w+ndo) eles\b', r'\1-os', r)
    r = re.sub(r'(\w+ndo) ele\b', r'\1-o', r)
    return r


def mask(text, names):
    table = []
    cand = set()
    for n in names:
        n = n.rstrip('*')
        cand.add(n)
        cand.add(n.split(',')[0])
        cand.add(n.split(' // ')[0])
        w = n.split(',')[0].split(' ')[0]
        if len(w) >= 5 and w[0].isupper() and w not in ('Treasure', 'Forest', 'Island', 'Swamp', 'Mountain', 'Plains', 'Blood', 'Spirit', 'Squirrel', 'Goblin', 'Zombie', 'Elemental', 'Dragon', 'Angel', 'Phyrexian', 'Eldrazi', 'Activate', 'Resolve', 'Repeat', 'Holding', 'Kodama'[:0]):
            cand.add(w)
    for n in sorted(cand, key=len, reverse=True):
        if n and n in text:
            text = text.replace(n, f'«{len(table)}»')
            table.append(n)
    return text, table


def unmask(text, table):
    return re.sub(r'«(\d+)»', lambda m: table[int(m.group(1))], text)


# ---------- resultados ----------
FEAT_T = {'creature': 'criatura', 'artifact': 'artefato', 'enchantment': 'encantamento', 'land': 'terreno', 'permanent': 'permanente', 'nonland permanent': 'permanente que não seja terreno'}
FEAT = [
    (r'(Near-)?[Ii]nfinite colorless mana', 'Mana incolor {M}'), (r'(Near-)?[Ii]nfinite colored mana', 'Mana colorido {M}'), (r'(Near-)?[Ii]nfinite mana', 'Mana {M}'),
    (r'(Near-)?[Ii]nfinite (white|blue|black|red|green) mana', lambda m: 'Mana ' + dict(white="branco", blue="azul", black="preto", red="vermelho", green="verde")[m.group(2)] + ' {M}'),
    (r'(Near-)?[Ii]nfinite mana (creatures|lands|artifacts) you control can produce', lambda m: 'Mana {M} das cores que ' + dict(creatures="suas criaturas", lands="seus terrenos", artifacts="seus artefatos")[m.group(2)] + ' produzem'),
    (r'(Near-)?[Ii]nfinite (creature|artifact|enchantment|land|permanent|nonland permanent) ETB', lambda m: 'Entradas {FP} de ' + FEAT_T[m.group(2)] + ' no campo'),
    (r'(Near-)?[Ii]nfinite (creature|artifact|enchantment|land|permanent|nonland permanent) LTB', lambda m: 'Saídas {FP} de ' + FEAT_T[m.group(2)] + ' do campo'),
    (r'(Near-)?[Ii]nfinite death triggers', 'Gatilhos de morte {MP}'), (r'(Near-)?[Ii]nfinite (creature|artifact|enchantment|land|permanent) sacrifice triggers', lambda m: 'Gatilhos {MP} de sacrifício de ' + FEAT_T[m.group(2)]),
    (r'(Near-)?[Ii]nfinite sacrifice triggers', 'Gatilhos de sacrifício {MP}'),
    (r'(Near-)?[Ii]nfinite storm count', 'Contagem de rajada {F}'), (r'(Near-)?[Ii]nfinite (tapped )?creature tokens( with haste)?', lambda m: 'Fichas de criatura {FP}' + (' viradas' if m.group(2) else '') + (' com ímpeto' if m.group(3) else '')),
    (r'(Near-)?[Ii]nfinite (artifact|Treasure|Clue|Food|Blood) tokens', lambda m: 'Fichas de ' + dict(artifact="artefato", Treasure="Tesouro", Clue="Pista", Food="Comida", Blood="Sangue")[m.group(2)] + ' {FP}'),
    (r'(Near-)?[Ii]nfinite \+1/\+1 counters on a creature', 'Marcadores +1/+1 {MP} em uma criatura'), (r'(Near-)?[Ii]nfinite \+1/\+1 counters on creatures you control', 'Marcadores +1/+1 {MP} nas suas criaturas'),
    (r'(Near-)?[Ii]nfinite draw triggers', 'Gatilhos de compra {MP}'), (r'(Near-)?[Ii]nfinite card draw', 'Compra de cards {F}'), (r'(Near-)?[Ii]nfinite landfall triggers', 'Gatilhos de aterragem {MP}'),
    (r'(Near-)?[Ii]nfinite magecraft triggers', 'Gatilhos de artimagia {MP}'), (r'(Near-)?[Ii]nfinite lifegain triggers', 'Gatilhos de ganho de vida {MP}'), (r'(Near-)?[Ii]nfinite lifegain', 'Ganho de vida {M}'),
    (r'(Near-)?[Ii]nfinite lifeloss', 'Perda de vida {F} para os oponentes'), (r'(Near-)?[Ii]nfinite damage', 'Dano {M}'), (r'(Near-)?[Ii]nfinite combat damage', 'Dano de combate {M}'),
    (r'(Near-)?[Ii]nfinite (extra )?turns', 'Turnos {MP}'), (r'(Near-)?[Ii]nfinite combat phases', 'Fases de combate {FP}'), (r'(Near-)?[Ii]nfinite self-mill', 'Triturar o próprio grimório sem limite'), (r'(Near-)?[Ii]nfinite mill', 'Triturar o grimório dos oponentes sem limite'),
    (r'(Near-)?[Ii]nfinite scry (\d+)', lambda m: 'Vidência ' + m.group(2) + ' {F}'), (r'(Near-)?[Ii]nfinite surveil(?: (\d+))?', lambda m: 'Vigiar' + (' ' + m.group(2) if m.group(2) else '') + ' {M}'),
    (r'(Near-)?[Ii]nfinite self-discard triggers', 'Gatilhos {MP} de descarte próprio'), (r'(Near-)?[Ii]nfinite looting', 'Comprar e descartar sem limite'), (r'(Near-)?[Ii]nfinite rummaging', 'Descartar e comprar sem limite'),
    (r'(Near-)?[Ii]nfinite untap of (artifact )?(creatures|lands|artifacts|permanents) you control', lambda m: 'Desvirar ' + dict(creatures="suas criaturas", lands="seus terrenos", artifacts="seus artefatos", permanents="suas permanentes")[m.group(3)] + (' artefato' if m.group(2) else '') + ' sem limite'),
    (r'(Near-)?[Ii]nfinite blinking(?: of (nonland permanents|creatures))?', lambda m: 'Exilar e devolver' + (' permanentes' if m.group(2) and "perm" in m.group(2) else ' criaturas' if m.group(2) else '') + ' sem limite (blink)'),
    (r'Infinitely large creature until end of turn', 'Criatura infinitamente grande até o final do turno'), (r'(Near-)?[Ii]nfinite (charge|loyalty) counters on a (permanent|planeswalker)', lambda m: 'Marcadores de ' + dict(charge="carga", loyalty="lealdade")[m.group(2)] + ' {MP} em uma permanente'),
    (r'Lock', 'Trava (os oponentes não conseguem jogar)'), (r'Win the game', 'Vitória no jogo'), (r'Target opponent loses the game', 'O oponente alvo perde o jogo'), (r'Each opponent loses the game', 'Cada oponente perde o jogo'),
    (r'(Near-)?[Ii]nfinite (cast|spell cast) triggers', 'Gatilhos de conjuração {MP}'), (r'(Near-)?[Ii]nfinite ETB', 'Entradas {FP} no campo'), (r'(Near-)?[Ii]nfinite LTB', 'Saídas {FP} do campo'),
    (r'(Near-)?[Ii]nfinite proliferate', 'Proliferar sem limite'), (r'(Near-)?[Ii]nfinite life', 'Vida {F}'),
    (r'Exile all cards from your library', 'Exilar todo o seu grimório'), (r'Draw your library', 'Comprar todo o seu grimório'), (r'Put your library into your graveyard', 'Colocar todo o seu grimório no cemitério'),
    (r'Mass land destruction', 'Destruição em massa de terrenos'),
]
FEAT = [(re.compile(a), b) for a, b in FEAT]
INF = {'M': 'infinito', 'F': 'infinita', 'MP': 'infinitos', 'FP': 'infinitas'}


def feat(s):
    for rx, rep in FEAT:
        m = rx.fullmatch(s)
        if m:
            r = rep(m) if callable(rep) else rep
            near = bool(m.groups() and m.group(1))
            r = re.sub(r'\{(M|F|MP|FP)\}', lambda k: ('quase ' if near else '') + INF[k.group(1)], r)
            return r.replace('sem limite', 'quase sem limite') if near else r
    return s


if __name__ == '__main__':
    src, dst = sys.argv[1], sys.argv[2]
    d = json.load(gzip.open(src))
    okc = totc = oks = tots = 0
    fail = collections.Counter()
    fe = collections.Counter()
    for c in d['combos']:
        ps = [feat(p) for p in c['p']]
        for a, b in zip(c['p'], ps):
            fe[a == b] += 1
        c['p'] = ps
        if not c.get('d'):
            continue
        totc += 1
        txt, table = mask(c['d'], c['c'])
        lines = [l.strip() for l in txt.split('\n') if l.strip()]
        res = []
        good = True
        for l in lines:
            tots += 1
            r = step(l)
            if r is None:
                good = False
                fail[re.sub(r'\d+', '#', l)] += 1
            else:
                oks += 1
                res.append(unmask(r, table))
        if good:
            okc += 1
        out = []
        for l in lines:
            r = step(l)
            out.append(unmask(r if r is not None else l, table))
        c['d'] = '\n'.join(out)
    print('resultados traduzidos', fe[False], 'de', fe[True] + fe[False])
    print('passos', oks, '/', tots, '| combos inteiros', okc, '/', totc)
    if len(sys.argv) > 3:
        for t, n in fail.most_common(int(sys.argv[3])):
            print(n, t)
    with gzip.open(dst, 'wt', encoding='utf-8', compresslevel=9) as f:
        json.dump(d, f, ensure_ascii=False, separators=(',', ':'))
