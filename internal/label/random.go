package label

import "math/rand/v2"

var adjectives = []string{
	"bold", "calm", "cool", "dark", "fast",
	"gold", "keen", "lean", "neat", "pale",
	"pure", "rare", "safe", "slim", "soft",
	"tall", "warm", "wide", "wise", "wild",
}

var nouns = []string{
	"ant", "bat", "cat", "cow", "dog",
	"elk", "fox", "fly", "jay", "owl",
	"ram", "ray", "bee", "eel", "yak",
	"ape", "cod", "hen", "hog", "kit",
}

// RandomLabel generates a random "adjective-noun" label.
func RandomLabel() string {
	a := adjectives[rand.IntN(len(adjectives))]
	n := nouns[rand.IntN(len(nouns))]
	return a + "-" + n
}
