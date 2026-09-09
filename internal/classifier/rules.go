package classifier

import "strings"

// Clasificador por reglas <2ms, sin llamada LLM extra.
// Senales: longitud, keywords criticas vs triviales, marcadores de codigo,
// multimodal/ocr, profundidad de conversacion.

var criticalKeywords = []string{
	"arquitectura", "architecture", "auth", "seguridad", "security",
	"migracion", "migration", "rls", "disena", "design system",
	"tradeoff", "cqrs", "event sourcing", "distribuido", "distributed",
	"auditoria", "audit", "threat model", "release risk", "dependencias criticas",
}

var trivialKeywords = []string{
	"busca", "lista", "resume", "grep", "donde esta", "find",
	"enumera", "cuenta", "formatea", "traduce", "summarize", "list files",
}

var codeMarkers = []string{
	"```", "func ", "def ", "class ", "import ", "package ",
	"SELECT ", "fn ", "const ", "interface ",
}

var researchKeywords = []string{
	"investiga", "research", "compara", "benchmark", "estado del arte", "landscape",
}

var ocrKeywords = []string{
	"ocr", "factura", "escaneado", "screenshot", "captura", "transcribe",
}

var presentationKeywords = []string{
	"presentacion", "slides", "landing", "copy", "hero section", "pitch",
}

var designKeywords = []string{
	"figma", "paleta", "tipografia", "layout", "responsive", "mockup", "hig",
}

var dataKeywords = []string{
	"dataframe", "csv", "sql", "dashboard", "metricas", "dataset", "pandas",
}

// buildKeywords: tareas de construccion/testing necesitan competencia media+.
// stakesKeywords: alta visibilidad (launch, prod) sube un rung.
var buildKeywords = []string{
	"refactor", "implementa", "agrega tests", "escribe tests", "deploy",
	"migra el codigo", "fix the bug", "agrega la feature",
}

var stakesKeywords = []string{
	"lanzamiento", "launch", "pitch", "produccion", "production",
	"release", "publico", "board-level",
}

// Score devuelve complejidad 0-1.
func Score(prompt string, hasCode, hasImage bool, historyTurns int) float64 {
	lower := strings.ToLower(prompt)
	score := 0.2

	if len(prompt) > 2000 {
		score += 0.2
	} else if len(prompt) > 500 {
		score += 0.1
	}
	if hasCode || HasCodeMarkers(prompt) {
		score += 0.15
	}
	if hasImage {
		score += 0.15
	}
	if historyTurns > 5 {
		score += 0.1
	}
	// Criticas: la primera pesa +0.35; cada distinta extra +0.1 (tope +0.55).
	// Asi "auth + threat model + migracion" llega a frontera aunque sea corto.
	hits := 0
	for _, k := range criticalKeywords {
		if strings.Contains(lower, k) {
			hits++
		}
	}
	if hits > 0 {
		critBonus := 0.35 + 0.1*float64(hits-1)
		if critBonus > 0.55 {
			critBonus = 0.55
		}
		score += critBonus
	}
	for _, k := range buildKeywords {
		if strings.Contains(lower, k) {
			score += 0.15
			break
		}
	}
	for _, k := range stakesKeywords {
		if strings.Contains(lower, k) {
			score += 0.2
			break
		}
	}
	for _, k := range trivialKeywords {
		if strings.Contains(lower, k) {
			score -= 0.15
			break
		}
	}
	if score < 0 {
		score = 0
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// HasCodeMarkers detecta codigo inline sin que el caller lo declare.
func HasCodeMarkers(prompt string) bool {
	for _, m := range codeMarkers {
		if strings.Contains(prompt, m) {
			return true
		}
	}
	return false
}

// DetectDomain clasifica el dominio de la tarea para cruzar con el catalogo.
func DetectDomain(prompt string) string {
	lower := strings.ToLower(prompt)
	containsAny := func(keys []string) bool {
		for _, k := range keys {
			if strings.Contains(lower, k) {
				return true
			}
		}
		return false
	}
	switch {
	case containsAny(ocrKeywords):
		return "ocr"
	case HasCodeMarkers(prompt):
		return "code"
	case containsAny(criticalKeywords):
		return "reasoning"
	case containsAny(researchKeywords):
		return "research"
	case containsAny(presentationKeywords):
		return "presentation"
	case containsAny(designKeywords):
		return "design"
	case containsAny(dataKeywords):
		return "data_analysis"
	default:
		return "general"
	}
}
