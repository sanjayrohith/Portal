package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultClientConfigPath returns the user's Portal configuration path.
func DefaultClientConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find user home directory: %w", err)
	}
	return filepath.Join(home, ".portal", "config.yaml"), nil
}

// LoadClientConfig loads a YAML client configuration over production defaults.
// An empty path resolves to ~/.portal/config.yaml.
func LoadClientConfig(path string) (*ClientConfig, error) {
	if path == "" {
		var err error
		path, err = DefaultClientConfigPath()
		if err != nil {
			return nil, err
		}
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read client config %q: %w", path, err)
	}
	var file clientConfigFile
	if err := yaml.Unmarshal(contents, &file); err != nil {
		return nil, fmt.Errorf("parse client config %q: %w", path, err)
	}

	config := DefaultClientConfig()
	if file.ServerAddr != "" {
		config.ServerAddr = file.ServerAddr
	}
	if file.LocalTarget != "" {
		config.LocalTarget = file.LocalTarget
	}
	if file.Subdomain != "" {
		config.Subdomain = file.Subdomain
	}
	if file.AuthToken != "" {
		config.AuthToken = file.AuthToken
	}
	if file.HostHeader != "" {
		config.HostHeader = file.HostHeader
	}
	if file.InspectorAddr != "" {
		config.InspectorAddr = file.InspectorAddr
	}
	if file.KeepAliveInterval.Duration != 0 {
		config.KeepAliveInterval = file.KeepAliveInterval.Duration
	}
	if file.LogLevel != "" {
		config.LogLevel = file.LogLevel
	}
	config.InsecureSkipVerify = file.InsecureSkipVerify
	return config, nil
}

// LoadClientConfigIfExists loads the default file when present, otherwise
// returns production defaults without creating a file or directory.
func LoadClientConfigIfExists(path string) (*ClientConfig, error) {
	if path == "" {
		var err error
		path, err = DefaultClientConfigPath()
		if err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultClientConfig(), nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect client config %q: %w", path, err)
	}
	return LoadClientConfig(path)
}

type clientConfigFile struct {
	ServerAddr         string        `yaml:"server_addr"`
	LocalTarget        string        `yaml:"local_target"`
	Subdomain          string        `yaml:"subdomain"`
	AuthToken          string        `yaml:"auth_token"`
	HostHeader         string        `yaml:"host_header"`
	InspectorAddr      string        `yaml:"inspector_addr"`
	InsecureSkipVerify bool          `yaml:"insecure_skip_verify"`
	KeepAliveInterval  durationValue `yaml:"keepalive_interval"`
	LogLevel           string        `yaml:"log_level"`
}

type durationValue struct{ time.Duration }

func (d *durationValue) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		value, err := time.ParseDuration(node.Value)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", node.Value, err)
		}
		d.Duration = value
		return nil
	}
	var nanoseconds int64
	if err := node.Decode(&nanoseconds); err != nil {
		return fmt.Errorf("duration must be a string such as 15s: %w", err)
	}
	d.Duration = time.Duration(nanoseconds)
	return nil
}
