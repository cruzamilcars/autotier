# Contributing

## Lo que más suma (en orden)

1. **Adapters nuevos o actualizados** (`internal/adapters/` + `adapters/`):
   los harnesses cambian rápido (modelos, frontmatter, endpoints). Un adapter es
   una plantilla + su mapeo tier→modelo. Incluye cómo lo verificaste.
2. **Fuentes de benchmarks** (`internal/bench/data/*.yaml`): nombre, dominio,
   URL, fecha, peso, scores por alias y `exclude` con motivo si quitas un dato.
   Todo número sin fuente se rechaza.
3. **Casos de eval** (`evals/suite.yaml`): prompt + dominio + tier esperado.
   Deben pasar en `cost` sin learn (cold start determinista).

## Reglas

- Solo stdlib de Go (cero dependencias): el binario debe compilar offline.
- `gofmt -l .` vacío, `go vet ./...` limpio, `go test -count=1 ./...` verde,
  `eval` 6/6. El CI lo verifica todo.
- Sin llaves ni secretos en el repo. Los logs (`*.log.jsonl`), `out/` y
  binarios están en `.gitignore`.
- Cada fix de compatibilidad con un harness trae test de regresión
  (ver `internal/proxy/proxy_test.go`: caso `finish_reason` vs OpenCode).
- Flags del CLI van antes de los posicionales (límite del parser stdlib);
  documenta el uso así en README y `--help`.
