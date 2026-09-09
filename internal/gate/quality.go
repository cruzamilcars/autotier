package gate

import "strings"

// QualityGate: verifica si la respuesta del tier barato es suficiente.
// Si falla, el proxy escala un rung (cascade, max 2 escalaciones).
type Result struct {
	Pass    bool
	Reasons []string
}

var hedgingPhrases = []string{
	"como modelo de lenguaje",
	"as an ai",
	"as a language model",
	"no estoy seguro",
	"i'm not sure",
	"i am not sure",
	"podria ser",
	"podría ser",
	"es dificil saber",
	"es difícil saber",
	"no tengo suficiente contexto",
}

const minChars = 40

// Check evalua texto de respuesta. testsFailed viene del harness/CI del usuario.
func Check(text string, testsFailed bool) Result {
	r := Result{Pass: true}
	trimmed := strings.TrimSpace(text)
	if len([]rune(trimmed)) < minChars {
		r.Pass = false
		r.Reasons = append(r.Reasons, "too_short")
	}
	lower := strings.ToLower(trimmed)
	for _, h := range hedgingPhrases {
		if strings.Contains(lower, h) {
			r.Pass = false
			r.Reasons = append(r.Reasons, "hedging")
			break
		}
	}
	if testsFailed {
		r.Pass = false
		r.Reasons = append(r.Reasons, "tests_failed")
	}
	return r
}
