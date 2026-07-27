package commands

import (
	"math/rand"
	"strings"
	"time"
)

var wcgRng = rand.New(rand.NewSource(time.Now().UnixNano()))

// Curated dictionary of English words indexed by length (3 to 16)
var wcgDictionary = map[int][]string{
	3: {
		"cat", "dog", "sun", "pen", "box", "hat", "car", "run", "sky", "cup",
		"map", "fan", "bus", "key", "ice", "bed", "pin", "fox", "ant", "fly",
		"bat", "cow", "owl", "pig", "bar", "boy", "day", "jam", "net", "toy",
	},
	4: {
		"book", "fish", "bird", "tree", "moon", "star", "fire", "wind", "rain", "snow",
		"door", "lamp", "desk", "ship", "frog", "lion", "duck", "bear", "gold", "ring",
		"blue", "pink", "rose", "leaf", "rock", "sand", "wave", "king", "hero", "game",
	},
	5: {
		"apple", "bread", "chair", "clock", "earth", "house", "lemon", "music", "night", "ocean",
		"paper", "plant", "queen", "river", "robot", "snake", "table", "tiger", "train", "water",
		"beach", "cloud", "dance", "dream", "fruit", "green", "heart", "horse", "light", "magic",
	},
	6: {
		"animal", "banana", "bridge", "castle", "dragon", "engine", "flower", "forest", "garden", "island",
		"jungle", "monkey", "planet", "rabbit", "rocket", "silver", "spider", "stream", "sunset", "yellow",
		"buffer", "camera", "coffee", "doctor", "guitar", "laptop", "mirror", "number", "orange", "person",
	},
	7: {
		"airplane", "balloon", "blanket", "captain", "diamond", "dolphin", "feather", "giraffe", "hamster", "journey",
		"kitchen", "lantern", "monster", "octopus", "penguin", "pyramid", "rainbow", "silence", "thunder", "volcano",
		"battery", "biscuit", "chariot", "crystal", "freedom", "holiday", "kingdom", "painter", "scanner", "village",
	},
	8: {
		"dinosaur", "elephant", "flamingo", "football", "hospital", "kangaroo", "mountain", "notebook", "painting", "umbrella",
		"universe", "building", "calendar", "computer", "firework", "fountain", "treasure", "sandwich", "squirrel", "triangle",
		"barbecue", "chemical", "champion", "downtown", "engineer", "marathon", "midnight", "passport", "starfish", "wildlife",
	},
	9: {
		"astronaut", "butterfly", "chocolate", "dandelion", "harmonica", "jellyfish", "lighthouse", "orchestra", "pineapple", "spaceship",
		"submarine", "sunflower", "telescope", "adventure", "almanac", "avalanche", "champagne", "firefighter", "landscape", "microscope",
		"nightmare", "sanctuary", "saxophone", "tarantula", "vegetable", "waterfall", "chameleon", "crocodile", "firehouse", "porcupine",
	},
	10: {
		"blacksmith", "centipede", "dictionary", "earthquake", "helicopter", "locomotive", "marshmallow", "motorcycle", "rainforest", "rollercoaster",
		"rollerblade", "skateboard", "strawberry", "trampoline", "underwater", "volleyball", "watermelons", "woodpecker", "cheesecake", "dermatology",
		"locomotion", "metropolis", "superhero", "tournament", "wandering", "wilderness", "friendship", "leadership", "playstation", "basketball",
	},
	11: {
		"caterpillar", "destination", "electricity", "grasshopper", "illustration", "marshmallows", "masterpiece", "microphones", "neighborhood", "performance",
		"refrigerator", "skateboards", "snowboarding", "submarines", "supermarket", "telephones", "thunderstorm", "transformers", "waterfalls", "windmills",
	},
	12: {
		"cheeseburgers", "constellation", "disappearances", "encyclopedia", "extraterrestrial", "huckleberry", "illustrations", "jurisdiction", "kindergarten", "locomotives",
		"microscopes", "neighborhoods", "organizations", "photographer", "refrigerators", "skateboarding", "supermarket", "thunderstorms", "underground", "volleyballs",
	},
	13: {
		"accomplishment", "archaeologist", "autobiography", "characteristics", "congratulations", "disappointment", "embarrassment", "encyclopedias", "extravaganza", "identification",
		"international", "investigation", "misunderstanding", "multiplication", "pharmaceutical", "qualification", "recommendation", "transformation", "transportation", "unpredictable",
	},
	14: {
		"acknowledgment", "administrators", "characteristics", "classifications", "congratulation", "disadvantageous", "discrimination", "extravaganzas", "identification", "implementation",
		"individualism", "infrastructure", "interpretation", "investigations", "multiplications", "recommendations", "reconstruction", "representation", "responsibility", "transformations",
	},
	15: {
		"acknowledgments", "characterization", "congratulations", "counteroffensive", "disadvantages", "discontinuation", "experimentation", "identifications", "implementations", "incomprehensible",
		"indestructible", "infrastructures", "interpretations", "misunderstandings", "personification", "recommendation", "representations", "responsibilities", "standardization", "telecommunication",
	},
	16: {
		"characterizations", "counteroffensives", "discontinuations", "disillusionment", "electrocardiogram", "experimentations", "incomprehensible", "indestructibility", "industrialization", "institutionalized",
		"interchangeable", "intercontinental", "internationalize", "miscommunications", "misinterpretations", "personifications", "standardizations", "telecommunications", "unconstitutional", "unpredictability",
	},
}

// getRandomWord returns a random word of target length `length` and its scrambled anagram.
func getRandomWord(length int) (string, string) {
	words, ok := wcgDictionary[length]
	if !ok || len(words) == 0 {
		// Fallback to length 3
		words = wcgDictionary[3]
		length = 3
	}

	word := words[wcgRng.Intn(len(words))]
	return word, scrambleWord(word)
}

// scrambleWord shuffles the letters of a word so it differs from the original if possible.
func scrambleWord(word string) string {
	runes := []rune(word)
	n := len(runes)
	if n <= 1 {
		return word
	}

	for i := 0; i < 10; i++ {
		wcgRng.Shuffle(n, func(i, j int) {
			runes[i], runes[j] = runes[j], runes[i]
		})
		if string(runes) != word {
			break
		}
	}
	return strings.ToUpper(string(runes))
}
