package di

import (
	"errors"
	"sync"
)

// Container holds instances registered with the DI container.
type Container struct {
	mu        sync.RWMutex
	container map[string]interface{}
}

// NewContainer creates a new DI container.
func NewContainer() *Container {
	return &Container{
		container: make(map[string]interface{}),
	}
}

// Register registers an instance with the DI container.
func (c *Container) Register(name string, instance interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.container[name] = instance
}

// Resolve resolves an instance from the DI container.
func (c *Container) Resolve(name string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	instance, found := c.container[name]
	if !found {
		return nil, errors.New("no instance registered with name " + name)
	}

	return instance, nil
}
