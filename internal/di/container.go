package di

import (
	"errors"
	"reflect"
	"sync"
)

// ContainerInterface defines the methods for the DI container
type ContainerInterface interface {
	Register(name string, instance interface{})
	Resolve(name string) (interface{}, error)
	ResolveAll(targetType interface{}) ([]interface{}, error)
}

// DIContainer holds instances registered with the DI container.
type DIContainer struct {
	mu        sync.RWMutex
	container map[string]interface{}
}

// NewContainer creates a new DI container.
func NewContainer() *DIContainer {
	return &DIContainer{
		container: make(map[string]interface{}),
	}
}

// Register registers an instance with the DI container.
func (c *DIContainer) Register(name string, instance interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.container[name] = instance
}

// Resolve resolves an instance from the DI container.
func (c *DIContainer) Resolve(name string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	instance, found := c.container[name]
	if !found {
		return nil, errors.New("no instance registered with name " + name)
	}

	return instance, nil
}

// ResolveAll resolves all instances that match the given interface.
func (c *DIContainer) ResolveAll(targetType interface{}) ([]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []interface{}
	targetTypeValue := reflect.TypeOf(targetType)
	if targetTypeValue.Kind() != reflect.Ptr || targetTypeValue.Elem().Kind() != reflect.Interface {
		return nil, errors.New("targetType must be a pointer to an interface")
	}
	targetTypeValue = targetTypeValue.Elem()

	for _, instance := range c.container {
		if instance == nil {
			continue
		}
		instanceType := reflect.TypeOf(instance)
		if instanceType.Implements(targetTypeValue) {
			results = append(results, instance)
		}
	}

	if len(results) == 0 {
		return nil, errors.New("no instances found for the given type")
	}

	return results, nil
}
