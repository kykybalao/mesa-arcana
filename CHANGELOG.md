# Histórico de mudanças

O que mudou em cada versão do Mesa Arcana, separado por plataforma. Como usar cada versão: [Windows](versoes/Windows.md), [Celular (Android)](versoes/Celular.md) e [PSP](versoes/PSP.md).

## 2.0 (27/11/2026)

### Novo nas três versões

- **Partida contra a IA.** Escolha o seu deck e o deck da IA para treinar. A IA desvira, compra, joga um terreno, conjura o comandante ou a carta mais cara que puder pagar, ataca quando compensa e bloqueia. Quando ela ataca, você escolhe os bloqueadores. O dano de combate entre vocês dois é aplicado sozinho. Dos efeitos das cartas, ela lê só remoção (destruir todas, destruir ou exilar uma criatura) e compra.
- **Decks de exemplo:** os 5 decks de Commander de Foundations: Calling All Angels (Giada), Keen Engineering (Sai), Wretched Ranks (Ghoulcaller Gisa), Reign of Dragons (Lathliss) e Tramplesaurus Rex (Ghalta). No Windows e no PSP eles já vêm com as ilustrações baixadas.
- **20 de vida** quando o deck não tem comandante (40 com comandante).

### Windows

- **Exporta deck para o PSP:** o botão "PSP" na tela do deck grava a pasta pronta para o Memory Stick.
- **Na mesa:**
  - **Estilos das cartas na mesa** (tecla E): imagem oficial, moldura do Mesa Arcana com a ilustração, arte estendida, arte inteira ou proxy simples.
  - **"Baixar só as ilustrações"** na tela do deck.
  - **Zoom da carta** (Z ou botão do meio) com o texto e os contadores.
  - **Busca no grimório** com filtros de nome, tipo, custo de mana e cor.
- **Em rede:**
  - Ordem dos turnos no d20, "Passar a vez" com botão "Responder" e um timer que só avisa.
  - Caixas de vida para cada oponente.
  - Dá para ver o cemitério e o exílio dos outros jogadores.
  - Imagens das cartas, playmat e sleeve passam de um jogador para o outro (dá para desligar nas opções).
- **Aparência:** playmat padrão "Mesa Arcana", 5 playmats de cor e 10 de duas cores, desenhados pelo programa.
- **Atalhos:**
  - Atalhos trocáveis na lista do F1.
  - Com o mouse em cima de uma carta: G (cemitério), X (exílio), H (mão) e B (fundo do grimório).
  - Rodinha do mouse na vida.
  - Clique e botão direito nos contadores.
- **Menu da pilha inteira** no cemitério e no exílio.
- **Histórico de partidas:** o botão "Resultado" registra vitória, derrota ou empate e mostra a taxa de vitória por deck.

### Celular (Android): primeira versão

- App Android (APK) feito para toque:
  - toque vira e desvira;
  - toque duplo na mão joga a carta;
  - arrastar move a carta;
  - segurar abre o menu, com "Ampliar carta".
- Mesa completa:
  - mulligan, fases, fichas e contadores;
  - equipar e anexar, transformar;
  - vidência, moer, busca no grimório e desfazer.
- Importa listas de deck e pacotes offline (.json).
- Funciona sem internet. As imagens das cartas vêm do Scryfall quando há conexão.

### PSP: primeira versão

- Roda em PSP com firmware customizado.
- Joga sozinho ou em rede ad hoc com até 4 PSPs (salas 1 a 4).
- Painel da carta à esquerda, com texto grande que rola pela alavanca.
- Nome e custo em cada carta, e P/R na mão.
- Na mesa:
  - equipar e prender cartas;
  - mulligan, fichas e marcadores;
  - busca no grimório e menu da pilha inteira.
- Histórico de partidas e lista de todos os atalhos (R + SELECT).
- Feito para o PSP de verdade:
  - lê o Memory Stick só ao abrir o deck;
  - o processador sobe para 333 MHz só nessa hora;
  - não redesenha a tela parada;
  - mostra bateria e relógio.

## 1.0 (12/11/2026)

Primeira versão, só para Windows (e uma versão HTML para navegador).

- **Funciona sem internet:**
  - o texto de 33 mil cartas já vem dentro do programa;
  - sem imagem, a carta aparece como proxy de playtest.
- **Importa listas:**
  - aceita Moxfield, Archidekt, Arena, MTGO, EDHREC e texto puro;
  - entende nomes em inglês ou português e sugere correção para nomes errados.
- **Cartas em português:**
  - texto oficial quando existe, tradução não oficial marcada para parte das outras;
  - botão para voltar ao inglês.
- **Bracket e combos:**
  - bracket estimada pelos critérios oficiais;
  - combos do Commander Spellbook com o passo a passo.
- **Mesa:**
  - mulligan, fases anunciadas, fichas e contadores (com um clique);
  - equipar e anexar, imposto de comandante manual, vidência e desfazer;
  - poder e resistência calculados pelo texto da carta;
  - painel da esquerda escondível e zonas no campo.
- **Multijogador em rede local:**
  - até 6 jogadores, com chat e mão revelada;
  - setas entre cartas e mesas, modos de visão da mesa, placas de jogador e marcadores.
- **Aparência:** playmats e sleeves, com os seus próprios nas pastas `playmats` e `sleeves`.
- **Criador de cartas próprias.**
- **Fazedor de proxies:**
  - três estilos: moldura, arte estendida e arte inteira;
  - moldes para lições, dupla face e fusão;
  - nome novo na carta e imagem do seu PC;
  - PNG no tamanho de impressão ou folhas A4 com 9 cartas.
