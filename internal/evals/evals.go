package evals

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/cruzamilcars/autotier/internal/classifier"
	"github.com/cruzamilcars/autotier/internal/policy"
)

// Case es un item de evals/suite.yaml (parser minimalista, sin dependencias).
type Case struct {
	Name       string
	Domain     string
	ExpectTier string
	Prompt     string
}

type Result struct {
	Name       string
	Domain     string
	Expect     string
	Got        string
	Pass       bool
	Complexity float64
	Detected   string
}

// ParseSuite lee suite.yaml con formato:
// - {name: x, domain: y, expect_tier: z, prompt: "..."}
// o bloques con claves name:/domain:/expect_tier:/prompt:. Solo usa stdlib.
func ParseSuite(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Case
	var cur Case
	flush := func() {
		if cur.Name != "" {
			out = append(out, cur)
			cur = Case{}
		}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 512*1024), 512*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "- ") {
			flush()
			line = strings.TrimPrefix(line, "- ")
			// forma inline {name: x, domain: y, ...}
			if strings.HasPrefix(line, "{") {
				m := parseInline(line)
				out = append(out, Case{
					Name: m["name"], Domain: m["domain"],
					ExpectTier: m["expect_tier"], Prompt: m["prompt"],
				})
				continue
			}
		}
		if v, ok := kv(line, "name:"); ok {
			if cur.Name != "" {
				flush()
			}
			cur.Name = v
		} else if v, ok := kv(line, "domain:"); ok {
			cur.Domain = v
		} else if v, ok := kv(line, "expect_tier:"); ok {
			cur.ExpectTier = v
		} else if v, ok := kv(line, "prompt:"); ok {
			cur.Prompt = v
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("suite vacia o formato no reconocido: %s", path)
	}
	return out, nil
}

func kv(line, key string) (string, bool) {
	if !strings.HasPrefix(line, key) {
		return "", false
	}
	v := strings.TrimSpace(strings.TrimPrefix(line, key))
	v = strings.Trim(v, `"'`)
	return v, true
}

func parseInline(line string) map[string]string {
	m := map[string]string{}
	inner := strings.Trim(line, "{}")
	// split respetando comillas dobles del prompt
	var cur strings.Builder
	var key, val string
	inQuotes := false
	pairs := []string{}
	for _, r := range inner {
		switch r {
		case '"':
			inQuotes = !inQuotes
			cur.WriteRune(r)
		case ',':
			if inQuotes {
				cur.WriteRune(r)
			} else {
				pairs = append(pairs, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	pairs = append(pairs, cur.String())
	for _, p := range pairs {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key = strings.TrimSpace(kv[0])
		val = strings.Trim(strings.TrimSpace(kv[1]), `"`)
		m[key] = val
	}
	return m
}

// Run clasifica cada caso y compara el tier decidido vs esperado.
func Run(cases []Case, mode policy.Mode) []Result {
	out := make([]Result, 0, len(cases))
	for _, c := range cases {
		prompt := c.Prompt
		if prompt == "" {
			prompt = c.Name
		}
		comp := classifier.Score(prompt, false, false, 0)
		dom := classifier.DetectDomain(prompt)
		if c.Domain != "" {
			dom = c.Domain // la suite manda: evalua routing, no deteccion
		}
		dec := policy.Decide(comp, dom, mode, "", 0, 800, 512)
		out = append(out, Result{
			Name: c.Name, Domain: dom, Expect: c.ExpectTier,
			Got: dec.Tier, Pass: dec.Tier == c.ExpectTier,
			Complexity: comp, Detected: classifier.DetectDomain(prompt),
		})
	}
	return out
}
