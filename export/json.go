package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/toqueteos/geziyor/internal"
)

// JSONLine exports response data as JSON streaming file
type JSONLine struct {
	FileName   string
	EscapeHTML bool
	Prefix     string
	Indent     string
}

// Export exports response data as JSON streaming file
func (e *JSONLine) Export(exports chan interface{}) error {
	filename := internal.DefaultString(e.FileName, "out.json")
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("could not open file %q: %w", filename, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(e.EscapeHTML)
	encoder.SetIndent(e.Prefix, e.Indent)

	// Export data as responses came
	for res := range exports {
		if err := encoder.Encode(res); err != nil {
			internal.Logger.Printf("JSON encoding error on exporter: %v\n", err)
		}
	}

	return nil
}

// JSON exports response data as JSON
type JSON struct {
	FileName   string
	EscapeHTML bool
}

// Export exports response data as JSON
func (e *JSON) Export(exports chan interface{}) error {
	filename := internal.DefaultString(e.FileName, "out.json")
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("could not open file %q: %w", filename, err)
	}
	defer file.Close()

	_, err = file.Write([]byte("[\n"))
	if err != nil {
		return fmt.Errorf("could not write json start: %w", err)
	}

	// Write first line
	for res := range exports {
		data, err := marshalLine("\t", res, e.EscapeHTML)
		if err != nil {
			internal.Logger.Printf("JSON encoding error on exporter: %v\n", err)
			break
		}
		_, err = file.Write(data)
		if err != nil {
			return fmt.Errorf("could not write json result: %w", err)
		}
		// Forcibly break, this loop only goes through the first result received via exports channel
		break
	}

	// Write all others
	for res := range exports {
		data, err := marshalLine(",\n\t", res, e.EscapeHTML)
		if err != nil {
			internal.Logger.Printf("JSON encoding error on exporter: %v\n", err)
			continue
		}
		_, err = file.Write(data)
		if err != nil {
			return fmt.Errorf("could not write json result: %w", err)
		}
	}

	_, err = file.WriteString("]\n")
	if err != nil {
		return fmt.Errorf("could not write json end: %w", err)
	}

	return nil
}

func marshalLine(prefix string, t interface{}, escapeHTML bool) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(escapeHTML)

	buf.WriteString(prefix)
	err := encoder.Encode(t) // Write actual data

	return buf.Bytes(), err
}
