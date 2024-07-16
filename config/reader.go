package config

import (
	"fmt"
	"strconv"
)

type aReader struct {
	conf interface{}
}

// Get string value
// if not exists, return def
func (obj aReader) String(def string) string {
	if nil == obj.conf {
		return def
	}

	if _, ok := obj.conf.(int); ok {
		return fmt.Sprintf("%d", obj.conf)
	}

	if _, ok := obj.conf.(int64); ok {
		return fmt.Sprintf("%d", obj.conf)
	}

	if _, ok := obj.conf.(float32); ok {
		return fmt.Sprintf("%.f", obj.conf)
	}

	if _, ok := obj.conf.(float64); ok {
		return fmt.Sprintf("%.f", obj.conf)
	}

	return fmt.Sprintf("%v", obj.conf)
}

// Get int value
// if not exists, return def
func (obj aReader) Int(def int) int {
	if _, ok := obj.conf.(int); ok {
		return int(obj.conf.(int))
	}

	if _, ok := obj.conf.(int64); ok {
		return int(obj.conf.(int64))
	}

	if _, ok := obj.conf.(string); !ok {
		return int(def)
	}

	value, err := strconv.ParseInt(obj.conf.(string), 10, 64)
	if nil != err {
		return int(def)
	}

	return int(value)
}

// Get int64 value
// if not exists, return def
func (obj aReader) Int64(def int64) int64 {
	if _, ok := obj.conf.(int); ok {
		return int64(obj.conf.(int))
	}

	if _, ok := obj.conf.(int64); ok {
		return int64(obj.conf.(int64))
	}

	if _, ok := obj.conf.(string); !ok {
		return int64(def)
	}

	value, err := strconv.ParseInt(obj.conf.(string), 10, 64)
	if nil != err {
		return int64(def)
	}

	return value
}

// Get float32 value
// if not exists, return def
func (obj aReader) Float32(def float32) float32 {
	if _, ok := obj.conf.(int); ok {
		return float32(obj.conf.(int))
	}

	if _, ok := obj.conf.(int64); ok {
		return float32(obj.conf.(int64))
	}

	if _, ok := obj.conf.(float32); ok {
		return float32(obj.conf.(float32))
	}

	if _, ok := obj.conf.(float64); ok {
		return float32(obj.conf.(float64))
	}

	if _, ok := obj.conf.(string); !ok {
		return float32(def)
	}

	value, err := strconv.ParseFloat(obj.conf.(string), 32)
	if nil != err {
		return float32(def)
	}

	return float32(value)
}

// Get float64 value
// if not exists, return def
func (obj aReader) Float64(def float64) float64 {
	if _, ok := obj.conf.(int); ok {
		return float64(obj.conf.(int))
	}

	if _, ok := obj.conf.(int64); ok {
		return float64(obj.conf.(int64))
	}

	if _, ok := obj.conf.(float32); ok {
		return float64(obj.conf.(float32))
	}

	if _, ok := obj.conf.(float64); ok {
		return float64(obj.conf.(float64))
	}

	if _, ok := obj.conf.(string); !ok {
		return float64(def)
	}

	value, err := strconv.ParseFloat(obj.conf.(string), 64)
	if nil != err {
		return float64(def)
	}

	return value
}

func (obj aReader) Bool() bool {
	if _, ok := obj.conf.(string); ok {
		if "true" == obj.conf.(string) {
			return true
		}
	}

	return false
}

// Get interface{} value
func (obj aReader) Value() interface{} {
	return obj.conf
}
