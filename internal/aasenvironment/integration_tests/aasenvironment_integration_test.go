package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	code := testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		HealthURL: "http://localhost:6007/health",
	})
	os.Exit(code)
}

func TestUploadAASX(t *testing.T) {
	aasID := "https://example.com/ids/aas/test_aas"
	smID := "https://example.com/ids/sm/test_sm"

	// Create a dummy AASX in memory
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Add environment JSON
	envJSON := fmt.Sprintf(`{
		"assetAdministrationShells": [
			{
				"id": "%s",
				"idShort": "TestAAS",
				"modelType": "AssetAdministrationShell",
				"assetInformation": {
					"assetKind": "Instance",
					"globalAssetId": "global1"
				}
			}
		],
		"submodels": [
			{
				"id": "%s",
				"idShort": "TestSM",
				"modelType": "Submodel"
			}
		],
		"conceptDescriptions": []
	}`, aasID, smID)

	f, err := zw.Create("aasenv-with-no-id.json")
	require.NoError(t, err)
	_, err = f.Write([]byte(envJSON))
	require.NoError(t, err)

	err = zw.Close()
	require.NoError(t, err)

	// Prepare multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.aasx")
	require.NoError(t, err)
	_, err = io.Copy(part, &buf)
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)

	// Upload AASX
	uploadURL := "http://localhost:6007/upload"
	req, err := http.NewRequest("POST", uploadURL, body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Verify content in repositories
	// Wait a bit for persistence
	time.Sleep(2 * time.Second)

	// Check AAS Repository
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	aasURL := fmt.Sprintf("http://localhost:6004/shells/%s", aasIdentifier)
	respAAS, err := http.Get(aasURL)
	require.NoError(t, err)
	defer respAAS.Body.Close()
	assert.Equal(t, http.StatusOK, respAAS.StatusCode, "AAS should be found in AAS repository")

	// Check Submodel Repository
	smIdentifier := base64.RawURLEncoding.EncodeToString([]byte(smID))
	smURL := fmt.Sprintf("http://localhost:6003/submodels/%s", smIdentifier)
	respSM, err := http.Get(smURL)
	require.NoError(t, err)
	defer respSM.Body.Close()
	assert.Equal(t, http.StatusOK, respSM.StatusCode, "Submodel should be found in Submodel repository")
}
