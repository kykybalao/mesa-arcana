// Guarda o programa no celular para abrir sem internet. Imagens do Scryfall também ficam guardadas depois de vistas uma vez.
const V = 'mesa-arcana-v1';
const BASE = ['./', 'index.html', 'manifest.json', 'icone.png', 'icone-192.png', 'icone-512.png', 'dados/cartas.js', 'dados/pt.js', 'dados/analise.js'];
self.addEventListener('install', e => e.waitUntil(caches.open(V).then(c => c.addAll(BASE)).then(() => self.skipWaiting())));
self.addEventListener('activate', e => e.waitUntil(caches.keys().then(ks => Promise.all(ks.filter(k => k !== V).map(k => caches.delete(k)))).then(() => self.clients.claim())));
self.addEventListener('fetch', e => {
  const r = e.request;
  if (r.method !== 'GET') return;
  const img = /cards\.scryfall\.io/.test(r.url);
  const local = new URL(r.url).origin === location.origin;
  if (!local && !img) return; // API do Scryfall: sempre pela rede
  e.respondWith(caches.match(r).then(hit => {
    const net = fetch(r).then(res => {
      if (res && (res.ok || res.type === 'opaque')) { const cp = res.clone(); caches.open(V).then(c => c.put(r, cp)); }
      return res;
    }).catch(() => hit);
    return local ? (hit ? (net, hit) : net) : (hit || net);
  }));
});
