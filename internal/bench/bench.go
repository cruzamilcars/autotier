package bench

// Ingesta de benchmarks publicos al catalogo de capacidades.
// Cada fuente declara dominio, URL, fecha y peso. El importer promedia la
// evidencia por tier (segun los modelos ejemplo del catalogo), la mezcla con
// el prior curado (blend ponderado) y propone scores 0-10 con procedencia.
// Nada se aplica sin `--write`: `bench report` solo muestra deltas.
//
// Los aliases de datos se mapean a los ejemplos del catalogo via aliasMap
// (familias, no SKUs exactos: "gpt-5.x" promedia gpt-5-2/gpt-5-1).

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cruzamilcars/autotier/internal/catalog"
)

type Source struct {
	Name    string
	Domain  string
	URL     string
	Date    string
	Weight  float64
	Scores  map[string]float64 // alias -> 0-100
	Exclude map[string]string  // alias -> motivo documentado
	File    string
}

// aliasMap: ejemplo del catalogo -> aliases aceptados en datos.
var aliasMap = map[string][]string{
	"claude-haiku-4-5":  {"claude-haiku-4-5"},
	"gpt-4o-mini":       {"gpt-4o-mini"},
	"kimi-k2-lite":      {},
	"claude-sonnet-4-6": {"claude-sonnet-4-6"},
	"gpt-5.x":           {"gpt-5-2", "gpt-5-1", "gpt-5"},
	"kimi-k2.5":         {"kimi-k2-5"},
	"claude-opus-5":     {"claude-opus-5", "claude-opus-4-8", "claude-opus-4-6", "claude-opus-4-5"},
	"gpt-5.x-high":      {"gpt-5-2", "gpt-5-1"},
	"kimi-k2-thinking":  {"kimi-k2-thinking"},
}

// Evidence es el promedio 0-10 por tier para un dominio, con n modelos.
type Evidence struct {
	Score float64
	N     int
	From  []string // "fuente:alias=valor"
}

// Blend mezcla prior curado con evidencia (pesos configurables).
func Blend(prior int, ev float64, wPrior, wEv float64) int {
	if wPrior+wEv == 0 {
		return prior
	}
	v := (float64(prior)*wPrior + ev*wEv) / (wPrior + wEv)
	r := int(v + 0.5)
	if r < 0 {
		r = 0
	}
	if r > 10 {
		r = 10
	}
	return r
}

// LoadDir lee *.yaml del directorio.
func LoadDir(dir string) ([]Source, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Source
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		s, err := parseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// parseFile: parser minimo para nuestro esquema fijo (sin dependencias).
func parseFile(path string) (Source, error) {
	s := Source{Scores: map[string]float64{}, Exclude: map[string]string{}, Weight: 1, File: filepath.Base(path)}
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	section := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 256*1024), 256*1024)
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indented := strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")
		if !indented && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		if !indented {
			switch k {
			case "name":
				s.Name = v
			case "domain":
				s.Domain = v
			case "url":
				s.URL = v
			case "date":
				s.Date = v
			case "weight":
				if w, err := strconv.ParseFloat(v, 64); err == nil {
					s.Weight = w
				}
			}
			continue
		}
		switch section {
		case "scores":
			if num, err := strconv.ParseFloat(v, 64); err == nil {
				s.Scores[strings.ToLower(k)] = num
			}
		case "exclude":
			s.Exclude[strings.ToLower(k)] = v
		}
	}
	if s.Name == "" || s.Domain == "" {
		return s, fmt.Errorf("faltan name/domain")
	}
	return s, sc.Err()
}

// RecencyFactor decae el peso con la edad: 0.5^(dias/halfLife), piso minF.
// Fechas futuras o halfLife<=0 (desactivado) devuelven 1. Formato YYYY-MM-DD.
func RecencyFactor(sourceDate, asOf string, halfLifeDays, minF float64) float64 {
	if halfLifeDays <= 0 {
		return 1
	}
	const layout = "2006-01-02"
	sd, err1 := time.Parse(layout, strings.TrimSpace(sourceDate))
	ad, err2 := time.Parse(layout, strings.TrimSpace(asOf))
	if err1 != nil || err2 != nil {
		return 1
	}
	age := ad.Sub(sd).Hours() / 24
	if age <= 0 {
		return 1
	}
	f := math.Pow(0.5, age/halfLifeDays)
	if f < minF {
		f = minF
	}
	return f
}

// EffectiveWeights combina peso manual x recencia por fuente.
func EffectiveWeights(sources []Source, asOf string, halfLifeDays, minF float64) map[string]float64 {
	out := map[string]float64{}
	for _, s := range sources {
		out[s.Name] = s.Weight * RecencyFactor(s.Date, asOf, halfLifeDays, minF)
	}
	return out
}

// TierEvidence promedia la evidencia 0-10 por tier para un dominio,
// ponderada por peso efectivo de cada fuente.
func TierEvidence(models []catalog.Model, sources []Source, domain string, weights map[string]float64) map[string]Evidence {
	out := map[string]Evidence{}
	for _, m := range models {
		var sum, wsum float64
		var from []string
		n := 0
		for _, src := range sources {
			if src.Domain != domain {
				continue
			}
			w := 1.0
			if weights != nil {
				if ew, ok := weights[src.Name]; ok {
					w = ew
				}
			}
			for _, ex := range m.Examples {
				for _, al := range aliasMap[ex] {
					key := strings.ToLower(al)
					if _, excluded := src.Exclude[key]; excluded {
						continue
					}
					if v, ok := src.Scores[key]; ok {
						sum += (v / 10.0) * w
						wsum += w
						n++
						from = append(from, src.Name+":"+key+"="+fmt.Sprintf("%.1f", v))
					}
				}
			}
		}
		if n == 0 || wsum == 0 {
			continue
		}
		out[m.ID] = Evidence{Score: sum / wsum, N: n, From: from}
	}
	return out
}

// Cell describe una celda del reporte.
type Cell struct {
	Tier     string
	Domain   string
	Prior    int
	Evidence float64
	N        int
	Blended  int
	Changed  bool
	Sources  []string
}

// Report cruza catalogo con evidencia para los dominios con datos.
// wPrior/wEv ponderan prior vs evidencia; asOf/halfLife/minF, la recencia.
func Report(models []catalog.Model, sources []Source, wPrior, wEv float64, asOf string, halfLife, minF float64) []Cell {
	weights := EffectiveWeights(sources, asOf, halfLife, minF)
	domains := map[string]bool{}
	for _, s := range sources {
		domains[s.Domain] = true
	}
	var doms []string
	for d := range domains {
		doms = append(doms, d)
	}
	sort.Strings(doms)
	var out []Cell
	for _, d := range doms {
		ev := TierEvidence(models, sources, d, weights)
		var srcs []string
		for _, s := range sources {
			if s.Domain == d {
				srcs = append(srcs, s.Name+" ("+s.Date+")")
			}
		}
		for _, m := range models {
			e, ok := ev[m.ID]
			if !ok {
				continue
			}
			b := Blend(m.ScoreFor(d), e.Score, wPrior, wEv)
			out = append(out, Cell{
				Tier: m.ID, Domain: d, Prior: m.ScoreFor(d),
				Evidence: e.Score, N: e.N, Blended: b,
				Changed: b != m.ScoreFor(d), Sources: srcs,
			})
		}
	}
	return out
}
