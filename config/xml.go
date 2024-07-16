package config

import (
	"io"
	"os"
	"strings"

	"github.com/tinyhubs/tinydom"
)

type aXml struct {
	xmldoc tinydom.XMLDocument
}

// Generate configuration parsing object based on xml file
//
// @param	 f	config file
func GetXml(f string) *aXml {
	obj := &aXml{}

	xmlFile, err := os.Open(f)
	if err != nil {
		return obj
	}
	defer xmlFile.Close()

	byteValue, err := io.ReadAll(xmlFile)
	if nil != err {
		return obj
	}

	xmldoc, _ := tinydom.LoadDocument(strings.NewReader(string(byteValue)))
	obj.xmldoc = xmldoc
	return obj
}

// Read configuration information
//
// @param args 	Configuration properties
func (this *aXml) Get(args ...string) aReader {
	if 0 == len(args) || nil == this.xmldoc {
		return aReader{}
	}

	var xml tinydom.XMLElement
	for _, arg := range args {
		if nil == xml {
			xml = this.xmldoc.FirstChildElement(arg)

			continue
		}

		if nil == xml {
			break
		}

		xml = xml.FirstChildElement(arg)
	}

	if nil == xml {
		return aReader{}
	}

	return aReader{
		conf: xml.Text(),
	}
}
