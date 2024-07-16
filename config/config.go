package config

import (
	"strings"
)

type iconfig interface {
	Get(args ...string) aReader
}

var iconf iconfig

// Configuration initialization
//
// @param	f 	config file
func Init(f string) {
	if strings.HasSuffix(f, ".xml") {
		iconf = GetXml(f)
	}
}

// Read configuration information
//
// @param args 	Configuration properties
func Get(args ...string) aReader {
	if nil == iconf {
		return aReader{}
	}

	arg := make([]string, 0)
	arg = append(arg, "gffg")
	arg = append(arg, args...)
	return iconf.Get(arg...)
}
