package di

import (
	"fmt"
	"reflect"
	"sync"
)

// Injector defines the methods for the injector.
type Injector interface {
	Register(name string, instance interface{})
	Resolve(name string) interface{}
	ResolveAll(targetType interface{}) ([]interface{}, error)
}

// BaseInjector holds instances registered with the injector.
type BaseInjector struct {
	mu    sync.RWMutex
	items map[string]interface{}
}

// NewInjector creates a new injector.
func NewInjector() *BaseInjector {
	return &BaseInjector{
		items: make(map[string]interface{}),
	}
}

// Register registers an instance with the injector.
func (i *BaseInjector) Register(name string, instance interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.items[name] = instance
}

// Resolve resolves an instance from the injector.
func (i *BaseInjector) Resolve(name string) interface{} {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.items[name]
}

// ResolveAll resolves all instances that match the given interface.
func (i *BaseInjector) ResolveAll(targetType interface{}) ([]interface{}, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var results []interface{}
	targetTypeValue := reflect.TypeOf(targetType)
	if targetTypeValue.Kind() != reflect.Ptr || targetTypeValue.Elem().Kind() != reflect.Interface {
		return nil, fmt.Errorf("targetType must be a pointer to an interface")
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

	return results, nil
}

// Ensure BaseInjector implements Injector interface
var _ Injector = (*BaseInjector)(nil)
