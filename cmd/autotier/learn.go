package main

import (
	"flag"
	"fmt"

	"github.com/cruzamilcars/autotier/internal/learn"
)

func cmdLearn(args []string) int {
	if len(args) == 0 {
		fmt.Println("uso: autotier learn <status|reset> [--learn learn.json]")
		return 1
	}
	switch args[0] {
	case "status":
		return learnStatus(args[1:])
	case "reset":
		return learnReset(args[1:])
	default:
		fmt.Println("subcomando desconocido:", args[0])
		return 1
	}
}

func learnPath(args []string, name string) (string, int) {
	fs := flag.NewFlagSet("learn "+name, flag.ContinueOnError)
	p := fs.String("learn", "learn.json", "estado bandit JSON")
	if err := fs.Parse(args); err != nil {
		return "", 1
	}
	return *p, 0
}

func learnStatus(args []string) int {
	path, code := learnPath(args, "status")
	if code != 0 {
		return code
	}
	st, err := learn.Load(path)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	keys := st.Keys()
	if len(keys) == 0 {
		fmt.Printf("sin evidencia en %s (cold start: mandan las reglas)\n", path)
		return 0
	}
	fmt.Printf("%-28s %6s %6s %6s\n", "BRAZO tier|dominio", "pulls", "wins", "media")
	for _, k := range keys {
		pulls, wins, mean := st.Stats(k)
		fmt.Printf("%-28s %6d %6d %6.2f\n", k, pulls, wins, mean)
	}
	return 0
}

func learnReset(args []string) int {
	path, code := learnPath(args, "reset")
	if code != 0 {
		return code
	}
	if err := learn.New().Save(path); err != nil {
		fmt.Println("error:", err)
		return 1
	}
	fmt.Printf("estado reiniciado en %s\n", path)
	return 0
}
