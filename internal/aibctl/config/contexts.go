package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"
)

func (c *Config) Find(name string) (*Context, bool) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i], true
		}
	}
	return nil, false
}

// Upsert replaces the context with the same name or appends a new one.
func (c *Config) Upsert(ctx Context) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == ctx.Name {
			c.Contexts[i] = ctx
			return
		}
	}
	c.Contexts = append(c.Contexts, ctx)
}

func (c *Config) Use(name string) error {
	if _, ok := c.Find(name); !ok {
		return fmt.Errorf("context %q not found", name)
	}
	c.CurrentContext = name
	return nil
}

func (c *Config) Delete(name string) error {
	idx := slices.IndexFunc(c.Contexts, func(ctx Context) bool { return ctx.Name == name })
	if idx < 0 {
		return fmt.Errorf("context %q not found", name)
	}
	c.Contexts = slices.Delete(c.Contexts, idx, idx+1)
	if c.CurrentContext == name {
		c.CurrentContext = ""
	}
	return nil
}

// Save validates cfg and writes it atomically with owner-only permissions.
func Save(path string, cfg *Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}
