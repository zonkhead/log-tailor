package main

import (
	logger "log"
	"os"

	"gopkg.in/yaml.v3"
)

type OutputMap map[string]any

type Config struct {
	Limit     int
	Format    string
	MatchRule string      `yaml:"match-rule"`
	Projects  []string    `yaml:"projects"`
	Common    []OutputMap `yaml:"common-output"`
	Logs      []Log       `yaml:"logs"`
	Filters   []string    `yaml:"filters"`
	Buffered  bool
}

type Log struct {
	Name    string      `yaml:"name"`
	ResType string      `yaml:"type"`
	Output  []OutputMap `yaml:"output"`
}

// Reads the yaml config from stdin
func getConfig(data []byte, args *cmdlnArgs) *Config {
	// First, check to see if there actually is stdin data.
	if data == nil {
		config := &Config{}
		return config.setDefaults().overrideFields(args)
	}

	var config Config

	err := yaml.Unmarshal(data, &config)
	if err != nil {
		logger.Printf("Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	// Fix it
	if config.MatchRule == "" {
		config.MatchRule = "all"
	}

	return config.overrideFields(args).validatePaths()
}

func (c *Config) setDefaults() *Config {
	if c.MatchRule == "" {
		c.MatchRule = "all"
	}
	c.Buffered = false
	return c
}

// Let the command line args override their equivalents
// in the yaml config.
func (c *Config) overrideFields(args *cmdlnArgs) *Config {
	if args == nil {
		return c
	}
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

	c.Buffered = args.buffered

	return c
}

// Check paths to make sure key() syntax is valid
func (c *Config) validatePaths() *Config {
	for k := range c.Common {
		validateOutput(c.Common[k])
	}
	for _, l := range c.Logs {
		for k := range l.Output {
			validateOutput(l.Output[k])
		}
	}
	return c
}

// Helper for validatePaths()
func validateOutput(o any) {
	switch o := o.(type) {
	case string:
		if _, err := validatePathElements(o); err != nil {
			logAndDie(err.Error())
		}
	case OutputMap:
		if hasKeys(o, "src", "regex", "value") {
			s := o["src"].(string)
			if _, err := validatePathElements(s); err != nil {
				logAndDie(err.Error())
			}
		} else {
			for k := range o {
				switch kv := o[k].(type) {
				case string:
					if _, err := validatePathElements(kv); err != nil {
						logAndDie(err.Error())
					}
				case OutputMap:
					validateOutput(kv)
				}
			}
		}
	}
}

func logAndDie(msg string) {
	logger.Println(msg)
	os.Exit(1)
}
