package di

import (
	"errors"
	"reflect"
	"sync"
)

// Injector defines the methods for the injector.
type Injector interface {
	Register(name string, instance interface{})
	Resolve(name string) (interface{}, error)
	ResolveAll(targetType interface{}) ([]interface{}, error)
}

// simpleInjector holds instances registered with the injector.
type simpleInjector struct {
	mu    sync.RWMutex
	items map[string]interface{}
}

// NewInjector creates a new injector.
func NewInjector() *simpleInjector {
	return &simpleInjector{
		items: make(map[string]interface{}),
	}
}

// Register registers an instance with the injector.
func (i *simpleInjector) Register(name string, instance interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.items[name] = instance
}

// Resolve resolves an instance from the injector.
func (i *simpleInjector) Resolve(name string) (interface{}, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	instance, found := i.items[name]
	if !found {
		return nil, errors.New("no instance registered with name " + name)
	}

	return instance, nil
}

// ResolveAll resolves all instances that match the given interface.
func (i *simpleInjector) ResolveAll(targetType interface{}) ([]interface{}, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var results []interface{}
	targetTypeValue := reflect.TypeOf(targetType)
	if targetTypeValue.Kind() != reflect.Ptr || targetTypeValue.Elem().Kind() != reflect.Interface {
		return nil, errors.New("targetType must be a pointer to an interface")
	}
	targetTypeValue = targetTypeValue.Elem()

	for _, instance := range i.items {
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

// Ensure simpleInjector implements Injector interface
var _ Injector = (*simpleInjector)(nil)
