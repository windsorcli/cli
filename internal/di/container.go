package di

import (
	"errors"
	"reflect"
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
func (c *Container) Resolve(name string, target interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	instance, found := c.container[name]
	if !found {
		return errors.New("no instance registered with name " + name)
	}

	// Type assertion to ensure the target is of the correct type
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}

	targetElem := targetValue.Elem()
	instanceValue := reflect.ValueOf(instance)
	if !instanceValue.Type().AssignableTo(targetElem.Type()) {
		return errors.New("cannot assign instance of type " + instanceValue.Type().String() + " to target of type " + targetElem.Type().String())
	}

	targetElem.Set(instanceValue)
	return nil
}
