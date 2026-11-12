#!/bin/bash
# teste de rede: duas cópias do programa na mesma máquina (anfitrião + visitante)
cd /home/user/mesa && go build -o build/mesa-linux . || exit 1
for n in a b; do
  mkdir -p build/net$n/decks build/net$n/shots
  cp /mnt/project-files/decks/gabranth-v4-500.txt build/net$n/decks/Gabranth.txt
  cp build/run/clip.txt build/net$n/clip.txt
done
(cd build/neta && MESA_NET=host MESA_DIR=$PWD MESA_TEST=$PWD/shots MESA_CLIP=$PWD/clip.txt LIBGL_ALWAYS_SOFTWARE=1 timeout 200 xvfb-run -a -s "-screen 0 1500x950x24" ../mesa-linux > out.txt 2>&1) &
until grep -q editor build/neta/out.txt 2>/dev/null; do sleep 1; done
(cd build/netb && MESA_NET=join MESA_DIR=$PWD MESA_TEST=$PWD/shots MESA_CLIP=$PWD/clip.txt LIBGL_ALWAYS_SOFTWARE=1 timeout 200 xvfb-run -a -s "-screen 0 1500x950x24" ../mesa-linux > out.txt 2>&1) &
wait
echo "== anfitrião"; grep -v xvfb build/neta/out.txt | tail -6
echo "== visitante"; grep -v xvfb build/netb/out.txt | tail -6
