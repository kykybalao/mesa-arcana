package main

// mats.go: playmat padrão "Mesa Arcana" e o conjunto das cores do Magic (5 de uma cor, 10 de duas).
// Tudo é desenhado pelo programa, ponto a ponto; os emblemas são desenhos próprios (sol, gota, lua, chama, folha).

import "math"

type v3 = [3]float64

func sstep(a, b, x float64) float64 {
	t := max(0, min(1, (x-a)/(b-a)))
	return t * t * (3 - 2*t)
}

// starR: raio da borda de uma estrela de n pontas no ângulo a (r1 nas pontas, r2 nos vales)
func starR(a float64, n int, r1, r2 float64) float64 {
	al := math.Pi / float64(n)
	t := math.Mod(math.Mod(a, 2*al)+2*al, 2*al)
	if t > al {
		t = 2*al - t
	}
	return r1 * r2 * math.Sin(al) / (r2*math.Sin(t) + r1*math.Sin(al-t))
}

// emblemas: x, y em unidades do raio do emblema (y cresce para baixo); devolvem se o ponto está pintado
func embSun(x, y float64) bool {
	d := math.Hypot(x, y)
	if d < 0.42 {
		return true
	}
	return d > 0.52 && d < starR(math.Atan2(y, x), 12, 1, 0.62)
}
func dropIn(x, y, r float64) bool { // gota com a ponta para cima
	cy := 1 - r
	if y >= cy {
		return math.Hypot(x, y-cy) < r
	}
	if y < -1 {
		return false
	}
	return math.Abs(x) < r*math.Pow((y+1)/(cy+1), 0.75)
}
func embDrop(x, y float64) bool {
	return dropIn(x, y, 0.58) && !(math.Hypot(x+0.2, y-0.5) < 0.2 && math.Hypot(x+0.1, y-0.42) > 0.2)
}
func embMoon(x, y float64) bool {
	return math.Hypot(x, y) < 0.9 && math.Hypot(x-0.38, y+0.12) > 0.74
}
func embFlame(x, y float64) bool {
	xx := x - 0.16*math.Sin(y*2.6+0.6)
	return dropIn(xx, y, 0.56) && !dropIn((x-0.02)/0.5, (y-0.45)/0.5, 0.56)
}
func embLeaf(x, y float64) bool {
	c, s := math.Cos(0.6), math.Sin(0.6)
	x, y = x*c-y*s, x*s+y*c
	if math.Hypot(x-0.62, y) > 1.18 || math.Hypot(x+0.62, y) > 1.18 {
		return false
	}
	if math.Abs(x) < 0.035 { // nervura do meio
		return false
	}
	for k := -2; k <= 2; k++ { // nervuras de lado
		if d := math.Abs((y - float64(k)*0.3) + math.Abs(x)*0.55); d < 0.028 {
			return false
		}
	}
	return true
}

// cover2: quanto do ponto fica dentro do emblema (4 amostras, para a borda sair lisa)
func cover2(f func(x, y float64) bool, x, y, px float64) float64 {
	n := 0.0
	for _, o := range [][2]float64{{-0.25, -0.25}, {0.25, -0.25}, {-0.25, 0.25}, {0.25, 0.25}} {
		if f(x+o[0]*px, y+o[1]*px) {
			n += 0.25
		}
	}
	return n
}

type manaTheme struct {
	key, name string
	acc       v3
	tex       func(u, v float64) v3
	emb       func(x, y float64) bool
}

var manaThemes = []manaTheme{
	{"W", "Branco", v3{250, 236, 190}, func(u, v float64) v3 {
		n := fbm(u*4, v*3, 201)
		a := math.Atan2(v-0.5, (u-0.5)*16/9)
		ray := math.Pow(math.Sin(a*9+n*2)*0.5+0.5, 3)
		c := mix(v3{66, 58, 44}, v3{138, 122, 90}, n*1.3-0.1)
		c = mix(c, v3{214, 196, 150}, ray*0.22*sstep(0.08, 0.4, math.Hypot((u-0.5)*16/9, v-0.5)))
		vn := math.Abs(math.Sin((u*3 + v*2 + fbm(u*3, v*3, 207)*5) * math.Pi))
		return mix(c, v3{236, 226, 196}, math.Pow(1-vn, 16)*0.45)
	}, embSun},
	{"U", "Azul", v3{150, 220, 245}, func(u, v float64) v3 {
		n := fbm(u*4, v*3, 221)
		w := math.Sin((v*11+fbm(u*3, v*3, 222)*5+math.Sin(u*9)*0.4)*math.Pi)*0.5 + 0.5
		c := mix(v3{6, 20, 46}, v3{22, 84, 140}, n*1.35-0.15)
		c = mix(c, v3{96, 190, 226}, math.Pow(w, 9)*0.5)
		return mix(c, v3{210, 240, 250}, math.Pow(fbm(u*30, v*18, 225), 6)*1.2)
	}, embDrop},
	{"B", "Preto", v3{176, 150, 206}, func(u, v float64) v3 {
		n := fbm(u*3.5, v*2.5, 241)
		s := fbm(u*7+n*2.5, v*5-n*1.5, 243)
		c := mix(v3{9, 7, 13}, v3{52, 40, 66}, s*1.5-0.3)
		c = mix(c, v3{110, 86, 130}, math.Pow(s, 4)*1.2)
		if hash2(int(u*1280), int(v*720), 247) > 0.9991 {
			c = mix(c, v3{200, 180, 230}, 0.7)
		}
		return c
	}, embMoon},
	{"R", "Vermelho", v3{255, 176, 80}, func(u, v float64) v3 {
		n := fbm(u*5, v*3.5, 261)
		r := math.Abs(fbm(u*9+n*1.5, v*6, 263)-0.5) * 2
		c := mix(v3{28, 9, 7}, v3{120, 32, 16}, n*1.4-0.2)
		c = mix(c, v3{255, 140, 36}, math.Pow(1-r, 10)*0.9)
		return mix(c, v3{255, 220, 120}, math.Pow(1-r, 40)*0.6)
	}, embFlame},
	{"G", "Verde", v3{182, 220, 110}, func(u, v float64) v3 {
		n := fbm(u*6, v*4, 281)
		l := fbm(u*24, v*14, 283)
		c := mix(v3{8, 26, 15}, v3{40, 108, 56}, n*1.4-0.2)
		c = mix(c, v3{120, 170, 70}, math.Pow(l, 4)*1.2)
		return mix(c, v3{226, 232, 150}, math.Pow(fbm(u*3+4, v*2, 287), 5)*0.9)
	}, embLeaf},
}

const matGold = 0.75

var goldC = v3{232, 185, 74}

// matFrame: moldura fina dourada com losangos nos cantos, igual para todo o conjunto
func matFrame(c v3, u, v float64) v3 {
	const asp = 16.0 / 9
	x, y := u*asp, v // em unidades da altura
	dx, dy := min(x, asp-x), min(y, 1-y)
	d := min(dx, dy)
	for _, r := range [][2]float64{{0.030, 0.0016}, {0.040, 0.0008}} {
		if math.Abs(d-r[0]) < r[1] && max(dx, dy) > 0.03 {
			c = mix(c, goldC, matGold)
		}
	}
	if k := math.Abs(dx-0.035) + math.Abs(dy-0.035); k < 0.022 { // losango do canto
		c = mix(c, v3{14, 16, 20}, 0.85)
		if k > 0.017 || k < 0.006 {
			c = mix(c, goldC, 0.9)
		}
	}
	return c
}

// emblemAt pinta o emblema f centrado em (cu, cv), com raio r (em alturas), dentro de um anel
func emblemAt(c v3, f func(x, y float64) bool, acc v3, u, v, cu, cv, r float64) v3 {
	const asp = 16.0 / 9
	x, y := (u-cu)*asp/r, (v-cv)/r
	d := math.Hypot(x, y)
	if d > 1.75 {
		return c
	}
	c = mix(c, v3{0, 0, 0}, 0.42*sstep(1.7, 1.1, d)) // escurece atrás, para o emblema aparecer
	c = mix(c, acc, 0.34*cover2(f, x, y, 1.0/(r*720)))
	for _, rr := range [][2]float64{{1.42, 0.012}, {1.52, 0.005}} {
		if math.Abs(d-rr[0]) < rr[1] {
			c = mix(c, acc, 0.5)
		}
	}
	return c
}

func monoMat(t manaTheme) func(u, v float64) v3 {
	return func(u, v float64) v3 {
		c := emblemAt(t.tex(u, v), t.emb, t.acc, u, v, 0.5, 0.5, 0.17)
		return matFrame(c, u, v)
	}
}

func duoMat(a, b manaTheme, seed uint32) func(u, v float64) v3 {
	return func(u, v float64) v3 {
		e := u + (0.5-v)*0.22 + (fbm(u*4, v*4, seed)-0.5)*0.16 // divisa inclinada e irregular
		k := sstep(0.44, 0.56, e)
		c := mix(a.tex(u, v), b.tex(u, v), k)
		c = mix(c, mix(a.acc, b.acc, 0.5), 0.35*math.Pow(1-math.Abs(k-0.5)*2, 6)) // brilho onde as cores se encontram
		c = emblemAt(c, a.emb, a.acc, u, v, 0.26, 0.5, 0.13)
		c = emblemAt(c, b.emb, b.acc, u, v, 0.74, 0.5, 0.13)
		return matFrame(c, u, v)
	}
}

// arcanaMat: o playmat padrão. Feltro azul-petróleo com trama fina, moldura dourada e a estrela do programa no centro.
func arcanaMat(u, v float64) v3 {
	const asp = 16.0 / 9
	n := fbm(u*3, v*2, 301)
	c := mix(v3{10, 26, 36}, v3{22, 58, 66}, n*1.3-0.1)
	c = mix(c, v3{52, 40, 92}, sstep(0.45, 0.9, fbm(u*2+7, v*1.5+3, 305))*0.55) // manchas violeta
	w := math.Sin(u*asp*720*0.9) * math.Sin(v*720*0.9)                          // trama do tecido
	c = mix(c, v3{0, 0, 0}, 0.05+0.05*w)
	x, y := (u-0.5)*asp, v-0.5
	d, a := math.Hypot(x, y), math.Atan2(y, x)
	glow := sstep(0.55, 0, d)
	c = mix(c, v3{40, 140, 130}, glow*0.22)
	// linhas que saem do centro
	if d > 0.25 && d < 0.47 && math.Abs(math.Mod(a/(math.Pi/16)+64.5, 1)-0.5) < 0.012/d*0.25 {
		c = mix(c, goldC, 0.22)
	}
	for _, r := range [][3]float64{{0.245, 0.0014, 0.7}, {0.232, 0.0007, 0.6}, {0.47, 0.0012, 0.45}, {0.31, 0.0006, 0.3}} {
		if math.Abs(d-r[0]) < r[1] {
			c = mix(c, goldC, r[2])
		}
	}
	// pontos nas oito direções
	for k := 0; k < 8; k++ {
		ak := float64(k) * math.Pi / 4
		if math.Hypot(x-0.31*math.Cos(ak), y-0.31*math.Sin(ak)) < 0.008 {
			c = mix(c, goldC, 0.8)
		}
	}
	// estrela de oito pontas (a mesma do verso das cartas), em duas camadas
	px := 1.0 / 720
	big := cover2(func(x, y float64) bool { return math.Hypot(x, y) < starR(math.Atan2(y, x), 8, 0.21, 0.085) }, x, y, px)
	in := cover2(func(x, y float64) bool { return math.Hypot(x, y) < starR(math.Atan2(y, x)+math.Pi/8, 8, 0.12, 0.06) }, x, y, px)
	c = mix(c, goldC, 0.30*big)
	c = mix(c, v3{12, 22, 28}, 0.75*in)
	if d < 0.034 {
		c = mix(c, v3{70, 200, 170}, 0.8)
	}
	return matFrame(c, u, v)
}

func manaMats(add func(name string, f func(u, v float64) v3)) {
	for _, t := range manaThemes {
		add(t.name, monoMat(t))
	}
	// as dez duplas, na ordem da roda de cores: vizinhas primeiro, opostas depois
	for k, p := range [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 0}, {0, 2}, {1, 3}, {2, 4}, {3, 0}, {4, 1}} {
		a, b := manaThemes[p[0]], manaThemes[p[1]]
		add(a.name+" e "+b.name, duoMat(a, b, uint32(400+k)))
	}
}
