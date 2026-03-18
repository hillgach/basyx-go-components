package openapi

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment/aasx"
)

type AASEnvironmentAPIController struct {
	service AASEnvironmentAPIServicer
}

func NewAASEnvironmentAPIController(s AASEnvironmentAPIServicer) *AASEnvironmentAPIController {
	return &AASEnvironmentAPIController{
		service: s,
	}
}

func (c *AASEnvironmentAPIController) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/upload", c.PostUpload)
	return r
}

func (c *AASEnvironmentAPIController) PostUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Printf("AASENVAPI-POSTUPLOAD-DEFERCLOSEFILE: %v\n", closeErr)
		}
	}()

	ignoreDuplicates := r.FormValue("ignore-duplicates") == "true"

	tmpFile, err := os.CreateTemp("", "upload-*.aasx")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			fmt.Printf("AASENVAPI-POSTUPLOAD-DEFERREMOVEFILE: %v\n", removeErr)
		}
	}()
	defer func() {
		if closeErr := tmpFile.Close(); closeErr != nil {
			fmt.Printf("AASENVAPI-POSTUPLOAD-DEFERCLOSEFILE: %v\n", closeErr)
		}
	}()

	size, err := io.Copy(tmpFile, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := tmpFile.Seek(0, 0); err != nil {
		http.Error(w, fmt.Sprintf("AASENVAPI-POSTUPLOAD-SEEKFILE: %v", err), http.StatusInternalServerError)
		return
	}

	deserializer := aasx.NewDeserializer(tmpFile, size)
	env, relatedFiles, err := deserializer.Read()
	if err != nil {
		http.Error(w, fmt.Sprintf("AASENVAPI-POSTUPLOAD-READAASX: %v", err), http.StatusBadRequest)
		return
	}

	filesMap := make(map[string]io.Reader)
	for _, rf := range relatedFiles {
		filesMap[rf.Path] = rf.Reader
	}

	if err := tmpFile.Close(); err != nil {
		http.Error(w, fmt.Sprintf("AASENVAPI-POSTUPLOAD-CLOSEFILE: %v", err), http.StatusInternalServerError)
		return
	}

	result, err := c.service.PostUpload(r.Context(), env, filesMap, ignoreDuplicates)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load environment: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(result.Code)
	_, _ = w.Write([]byte(fmt.Sprintf("%v", result.Body)))
}
