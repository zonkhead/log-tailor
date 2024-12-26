package main

import (
	"gopkg.in/yaml.v3"
	"io"
	logger "log"
	"os"
)

type Config struct {
	Limit     int
	Format    string
	MatchRule string   `yaml:"match-rule"`
	Projects  []string `yaml:"projects"`
	Common    []any    `yaml:"common-output"`
	Logs      []Log    `yaml:"logs"`
	Filters   []string `yaml:"filters"`
}

type Log struct {
	Name    string `yaml:"name"`
	ResType string `yaml:"type"`
	Output  []any  `yaml:"output"`
}

// Reads the yaml config from stdin
func getConfig(args *cmdlnArgs) *Config {
	// First, check to see if there actually is stdin data.
	stat, _ := os.Stdin.Stat()
	if stat.Mode()&os.ModeCharDevice != 0 {
		config := &Config{}
		return config.setDefaults().overrideFields(args)
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		logger.Printf("Error reading from stdin: %v\n", err)
		os.Exit(1)
	}

	var config Config

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		logger.Printf("Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	// Fix it
	config.Limit = 0
	if config.MatchRule == "" {
		config.MatchRule = "all"
	}

	return config.overrideFields(args)
}

func (c *Config) setDefaults() *Config {
	if c.MatchRule == "" {
		c.MatchRule = "all"
	}
	return c
}

func (c *Config) overrideFields(args *cmdlnArgs) *Config {
	c.Limit = args.limit
	c.Format = args.format

	if len(args.projIDs) > 0 {
		c.Projects = args.projIDs
	}

	if len(args.logs) > 0 {
		newLogs := []Log{}
		for _, l := range args.logs {
			newLogs = append(newLogs, Log{Name: l})
		}
		c.Logs = newLogs
	}

	if len(args.filters) > 0 {
		var newFilters []string
		for _, f := range args.filters {
			newFilters = append(newFilters, f)
		}
		c.Filters = newFilters
	}

	if len(c.Projects) == 0 {
		stderrln("\nYou must specify at least one project.")
		os.Exit(1)
	}

	return c
}
