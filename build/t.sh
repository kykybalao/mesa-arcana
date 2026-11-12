#!/bin/bash
# compila e roda o roteiro de teste com tela virtual
cd /home/user/mesa && go build -o build/mesa-linux . || exit 1
rm -rf build/run && mkdir -p build/run/decks build/run/shots
cp /mnt/project-files/decks/gabranth-v4-500.txt build/run/decks/Gabranth.txt
printf 'Commander\n1 Atraxa, Praetors Voice (2XM) 190 *F*\n\nDeck\n1x Sol Rnig\n3 Forest (SLD) 108\n1 Fire // Ice\n1x Blood Artist (c21) 136 [Aristocrats] ^Have,#37d67a^\nSwords to Plowshares x2\n2\tCounterspell\nSB: 1 Negate\n1 Xyzzy Fake Card\nSideboard\n1 Duress\n' > build/run/clip.txt
mkdir -p build/run/dados/imagens; for id in $(python3 -c "import gzip,json;d=json.load(gzip.open(\"dados/cartas.json.gz\"));print(*[c[\"i\"] for c in d[\"cartas\"] if c[\"n\"] in (\"Plains\",\"Swamp\",\"Judge Magister Gabranth\",\"Sol Ring\") and not c.get(\"tk\")])"); do cp /tmp/fake.jpg build/run/dados/imagens/$id.jpg; done
cd build/run && MESA_DIR=$PWD MESA_TEST=$PWD/shots MESA_CLIP=$PWD/clip.txt LIBGL_ALWAYS_SOFTWARE=1 timeout ${MESA_TO:-170} xvfb-run -a -s "-screen 0 1500x950x24" ../mesa-linux 2>&1 | grep -v "xvfb-run" | head -${1:-12}
ls shots
