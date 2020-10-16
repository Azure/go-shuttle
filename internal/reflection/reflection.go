package reflection

import (
	"fmt"
	"reflect"
)

// GetReflectionValue returns the value specified by a key in an interface{} obj, if it exists
func GetReflectionValue(obj interface{}, key string) *string {
	reflectedObj := reflect.ValueOf(obj).Elem()
	reflectedVal := reflectedObj.FieldByName(key)

	if isZeroOfUnderlyingType(reflectedVal) {
		return nil
	}
	res := fmt.Sprintf("%v", reflectedVal.Interface().(interface{}))
	return &res
}

// GetType returns the name of the struct passed in
func GetType(obj interface{}) string {
	valueOf := reflect.ValueOf(obj)

	if valueOf.Type().Kind() == reflect.Ptr {
		return reflect.Indirect(valueOf).Type().Name()
	}
	return valueOf.Type().Name()
}

func isZeroOfUnderlyingType(x interface{}) bool {
	return reflect.DeepEqual(x, reflect.Zero(reflect.TypeOf(x)).Interface())
}
