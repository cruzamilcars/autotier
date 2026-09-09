package gate

import "testing"

func TestCheckPass(t *testing.T) {
	r := Check("Implementacion:\n- Cambio acotado por pasos\n- Tests que lo cubren\n- Verificacion local", false)
	if !r.Pass {
		t.Fatalf("debio pasar: %+v", r)
	}
}

func TestCheckTooShort(t *testing.T) {
	r := Check("ok", false)
	if r.Pass {
		t.Fatalf("texto de 2 chars debe fallar por too_short")
	}
}

func TestCheckHedging(t *testing.T) {
	r := Check("No estoy seguro, podria ser varias cosas, como modelo de lenguaje no tengo contexto suficiente para decidir nada concreto", false)
	if r.Pass {
		t.Fatalf("hedging debe fallar")
	}
}

func TestCheckTestsFailed(t *testing.T) {
	r := Check("Implementacion completa con muchos detalles y verificacion exhaustiva de cada paso del plan acordado", true)
	if r.Pass {
		t.Fatalf("tests_failed debe fallar aunque el texto sea largo")
	}
}
