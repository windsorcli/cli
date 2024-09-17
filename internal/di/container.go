package di

import (
	"errors"
	"reflect"
	"sync"
)

// container is a map that holds instances registered with the DI container.
// The key is a string identifier, and the value is the instance itself.
var (
	container = make(map[string]interface{})
	mu        sync.RWMutex
)

// Register registers an instance with the DI container.
func Register(name string, instance interface{}) {
	mu.Lock()
	defer mu.Unlock()
	container[name] = instance
}

// Resolve resolves an instance from the DI container.
func Resolve(name string, target interface{}) error {
	mu.RLock()
	defer mu.RUnlock()

	instance, found := container[name]
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
