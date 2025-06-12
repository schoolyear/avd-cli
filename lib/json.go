package lib

import (
	"encoding/json"
	"fmt"
	"github.com/adhocore/jsonc"
	"github.com/friendsofgo/errors"
	"io"
	"io/fs"
	"os"
)

func UnmarshalJSONorJSON5File[T any](searchFs fs.FS, name string) (out *T, cleanJSON []byte, err error) {
	data, _, err := ReadJSONOrJSON5AsJSON(searchFs, name)
	if err != nil {
		return nil, nil, err
	}

	if err := json.Unmarshal(data, &out); err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse json file")
	}

	return out, data, nil
}

// ReadJSONOrJSON5File reads either the json or json5 file
// returns an error if both are found
// returns os.ErrNotExist if neither are found
func ReadJSONOrJSON5File(searchFs fs.FS, name string) (data []byte, isJSON5 bool, err error) {
	path, isJSON5, err := findJSONOrJSON5Path(searchFs, name)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to find either json or json5 path")
	}

	f, err := searchFs.Open(path)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to open file %s", path)
	}
	defer f.Close()

	data, err = io.ReadAll(f)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to read json file %s", path)
	}

	return data, isJSON5, nil
}

func ReadJSONOrJSON5AsJSON(searchFs fs.FS, name string) (data []byte, wasJSON5 bool, err error) {
	data, isJSON5, err := ReadJSONOrJSON5File(searchFs, name)
	if err != nil {
		return nil, false, err
	}

	if isJSON5 {
		data = jsonc.New().Strip(data)
	}

	return data, isJSON5, nil
}

// findJSONOrJSON5Path tries both the .json and .json5 extension
// returns an error if both are found
// returns os.ErrNotExist if neither are found
// returns the path and whether it is a json5 file on one is found
func findJSONOrJSON5Path(searchFs fs.FS, name string) (path string, json5 bool, err error) {
	jsonPath := name + ".json"
	json5Path := name + ".json5"
	var jsonPathExists, json5PathExists bool

	jsonInfo, err := fs.Stat(searchFs, jsonPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", false, errors.Wrap(err, "failed to get info on json path")
		}
	} else {
		if jsonInfo.IsDir() {
			return "", false, fmt.Errorf("expected json file, but found directory %s", jsonPath)
		}

		jsonPathExists = true
	}

	json5Info, err := fs.Stat(searchFs, json5Path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", false, errors.Wrap(err, "failed to get info on json5 path")
		}
	} else {
		if json5Info.IsDir() {
			return "", false, fmt.Errorf("expected json5 file, but found directory %s", jsonPath)
		}

		json5PathExists = true
	}

	switch {
	case jsonPathExists && json5PathExists:
		return "", false, fmt.Errorf("both a json and a json5 file were found. choose one")
	case jsonPathExists:
		return jsonPath, false, nil
	case json5PathExists:
		return json5Path, true, nil
	default:
		return "", false, errors.Wrap(os.ErrNotExist, "neither json or json5 file was found")
	}
}

// JSON5Unsupported can be wrapped around objects with a custom unmarshaler that does not support json5
type JSON5Unsupported[T any] struct{ V T }

func (j JSON5Unsupported[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.V)
}

func (j *JSON5Unsupported[T]) UnmarshalJSON(bytes []byte) error {
	var v T
	if err := json.Unmarshal(jsonc.New().Strip(bytes), &v); err != nil {
		return err
	}
	j.V = v
	return nil
}

type JSONCombinedMarshaller struct {
	// Caller is responsible for making sure objects have no overlapping keys are marshal to JSON objects
	Objects []any
}

func (j JSONCombinedMarshaller) MarshalJSON() ([]byte, error) {
	out := []byte{'{'}
	for i, obj := range j.Objects {
		objOut, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, objOut[1:len(objOut)-1]...)
	}
	return append(out, '}'), nil
}
