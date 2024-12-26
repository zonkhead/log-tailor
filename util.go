package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

func stderrln(s string) {
	fmt.Fprintln(os.Stderr, s)
}

func stderrf(fs string, a ...any) {
	fmt.Fprintf(os.Stderr, fs, a...)
}

func stderr(_struct any) {
	_yaml, _ := yaml.Marshal(_struct)
	stderrln(string(_yaml))
}
