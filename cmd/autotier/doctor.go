package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cruzamilcars/autotier/internal/agents"
	"github.com/cruzamilcars/autotier/internal/learn"
)

// cmdDoctor verifica que autotier esta realmente activo y decide de verdad:
// proxy alcanzable, modelos listados, una decision real con ahorro, registry
// y learn legibles. Pensado para responder "¿esto hace algo o es pantalla?".
func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	proxyURL := fs.String("proxy", "http://127.0.0.1:4000", "base del proxy (vacio = salta checks HTTP)")
	agentsDir := fs.String("agents-dir", agents.DefaultDir, "registry")
	learnPath := fs.String("learn", "", "estado bandit a validar")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	fail := 0
	check := func(name string, ok bool, detail string) {
		mark := "PASS"
		if !ok {
			mark = "FAIL"
			fail++
		}
		fmt.Printf("[%s] %s %s\n", mark, name, detail)
	}
	client := &http.Client{Timeout: 10 * time.Second}

	if *proxyURL == "" {
		fmt.Println("(sin proxy: solo checks locales)")
	} else {
		base := strings.TrimRight(*proxyURL, "/")
		// 1. healthz
		_, err := client.Get(base + "/healthz")
		check("proxy responde /healthz", err == nil, base)
		// 2. models contiene auto
		hasAuto := false
		if resp, err := client.Get(base + "/v1/models"); err == nil {
			var v struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&v) == nil {
				for _, d := range v.Data {
					if d.ID == "auto" {
						hasAuto = true
					}
				}
			}
			resp.Body.Close()
		}
		check("proxy lista modelo auto", hasAuto, base+"/v1/models")
		// 3. decision real con ahorro (prueba de que no es pantalla)
		routed := false
		sav := 0.0
		body, _ := json.Marshal(map[string]any{
			"model": "auto",
			"messages": []map[string]string{
				{"role": "user", "content": "doctor check: lista los archivos del repo"},
			},
		})
		if resp, err := client.Post(base+"/v1/chat/completions", "application/json", bytes.NewReader(body)); err == nil {
			var v struct {
				Autotier struct {
					Tier       string  `json:"tier"`
					SavingsPct float64 `json:"savings_pct"`
					Decider    string  `json:"decider"`
				} `json:"autotier"`
			}
			if json.NewDecoder(resp.Body).Decode(&v) == nil && v.Autotier.Tier != "" {
				routed = true
				sav = v.Autotier.SavingsPct
			}
			resp.Body.Close()
		}
		check("proxy rutea de verdad", routed, fmt.Sprintf("tier con ahorro=%.0f%% (si es 0%% y quality 100%%, el mock responde pero no ahorra en ese caso)", sav))
	}

	// 4. registry
	reg, err := agents.Load(*agentsDir)
	check("registry de agentes", err == nil, fmt.Sprintf("%d agentes", len(reg)))
	// 5. learn (opcional)
	if *learnPath != "" {
		_, err := learn.Load(*learnPath)
		check("estado learn legible", err == nil, *learnPath)
	}
	if fail > 0 {
		fmt.Printf("%d checks fallaron — ver arriba como activarlo.\n", fail)
		return 1
	}
	fmt.Println("todo activo.")
	return 0
}
