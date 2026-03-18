package aasx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
)

type InMemoryFile struct {
	Path        string
	ContentType string
	Reader      io.Reader
}

type Deserializer struct {
	reader io.ReaderAt
	size   int64
}

func NewDeserializer(reader io.ReaderAt, size int64) *Deserializer {
	return &Deserializer{
		reader: reader,
		size:   size,
	}
}

func (d *Deserializer) Read() (types.IEnvironment, []InMemoryFile, error) {
	zr, err := zip.NewReader(d.reader, d.size)
	if err != nil {
		return nil, nil, fmt.Errorf("AASX-READ-ZIPOPEN: %w", err)
	}

	var env types.IEnvironment
	var relatedFiles []InMemoryFile

	for _, f := range zr.File {
		if isEnvironmentFile(f.Name) {
			env, err = d.parseEnvironment(f)
			if err != nil {
				return nil, nil, fmt.Errorf("AASX-READ-PARSEENV: %w", err)
			}
		} else if !f.FileInfo().IsDir() && !isMetadata(f.Name) {
			rc, err := f.Open()
			if err != nil {
				return nil, nil, fmt.Errorf("AASX-READ-OPENFILE: %w", err)
			}
			buf := new(bytes.Buffer)
			if _, err = io.Copy(buf, rc); err != nil {
				_ = rc.Close()
				return nil, nil, fmt.Errorf("AASX-READ-COPYFILE: %w", err)
			}
			_ = rc.Close()

			relatedFiles = append(relatedFiles, InMemoryFile{
				Path:        f.Name,
				ContentType: guessContentType(f.Name),
				Reader:      buf,
			})
		}
	}

	if env == nil {
		return nil, nil, fmt.Errorf("AASX-READ-NOENV: no environment file found in AASX")
	}

	return env, relatedFiles, nil
}

func isEnvironmentFile(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".json") && (strings.Contains(p, "aasenv") || strings.Contains(p, "env"))
}

func (d *Deserializer) parseEnvironment(f *zip.File) (types.IEnvironment, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rc.Close()
	}()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var jsonable any
	if err := json.Unmarshal(data, &jsonable); err != nil {
		return nil, err
	}

	if strings.HasSuffix(strings.ToLower(f.Name), ".json") {
		return jsonization.EnvironmentFromJsonable(jsonable)
	}
	return nil, fmt.Errorf("unsupported environment file format: %s", f.Name)
}

func isMetadata(path string) bool {
	return path == "[Content_Types].xml" || strings.HasPrefix(path, "_rels/") || strings.HasPrefix(path, "metadata/")
}

func guessContentType(path string) string {
	p := strings.ToLower(path)
	if strings.HasSuffix(p, ".png") {
		return "image/png"
	}
	if strings.HasSuffix(p, ".jpg") || strings.HasSuffix(p, ".jpeg") {
		return "image/jpeg"
	}
	if strings.HasSuffix(p, ".pdf") {
		return "application/pdf"
	}
	if strings.HasSuffix(p, ".json") {
		return "application/json"
	}
	if strings.HasSuffix(p, ".xml") {
		return "application/xml"
	}
	return "application/octet-stream"
}
