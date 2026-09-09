package classifier

import "testing"

func TestScoreTrivialLow(t *testing.T) {
	s := Score("busca donde esta la funcion login y lista sus archivos", false, false, 0)
	if s > 0.35 {
		t.Fatalf("trivial deberia ser <=0.35, got %.2f", s)
	}
}

func TestScoreCriticalHigh(t *testing.T) {
	s := Score("Disena la arquitectura de autenticacion con threat model y plan de migracion distribuida", false, false, 0)
	if s <= 0.70 {
		t.Fatalf("critico deberia ser >0.70, got %.2f", s)
	}
}

func TestDetectDomain(t *testing.T) {
	cases := map[string]string{
		"transcribe esta factura escaneada OCR":      "ocr",
		"```func main()``` refactoriza esto":         "code",
		"compara el estado del arte de routers":      "research",
		"escribe el copy de la landing hero section": "presentation",
	}
	for prompt, want := range cases {
		if got := DetectDomain(prompt); got != want {
			t.Fatalf("prompt %q: got %s want %s", prompt, got, want)
		}
	}
}
