package schema

import (
	_ "embed"
	"github.com/xeipuuv/gojsonschema"
)

//go:embed v2_properties.json
var propertiesSchema []byte
var schemaLoader = gojsonschema.NewBytesLoader(propertiesSchema)

func ValidateProperties(json []byte) (*gojsonschema.Result, error) {
	return gojsonschema.Validate(schemaLoader, gojsonschema.NewBytesLoader(json))
}
