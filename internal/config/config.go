package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Service struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

type Config struct {
	Services []Service `yaml:"services"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) FindService(name string) (*Service, bool) {
	for i := range c.Services {
		if c.Services[i].Name == name {
			return &c.Services[i], true
		}
	}
	return nil, false
}
