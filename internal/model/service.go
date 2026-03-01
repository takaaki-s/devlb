package model

// Service defines a service with its default listening port.
type Service struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

// Route represents a routing entry for a service.
type Route struct {
	BackendPort int    `yaml:"backend_port"`
	Label       string `yaml:"label,omitempty"`
	Active      bool   `yaml:"active"`
}
