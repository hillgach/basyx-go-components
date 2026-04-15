package main

import (
	"database/sql"
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const composeFilePath = "./docker_compose/docker_compose.yml"
const integrationTestDSN = "host=127.0.0.1 port=6432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable"

var allowedIntegrationPackages = map[string]struct{}{
	"github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/integration_tests":                  {},
	"github.com/eclipse-basyx/basyx-go-components/internal/smregistry/integration_tests":                   {},
	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/integration_tests":                {},
	"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/integration_tests":           {},
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/integration_tests": {},
	"github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/integration_tests":             {},
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     composeFilePath,
		PreDownBeforeUp: true,
		HealthURL:       "http://127.0.0.1:6004/health",
		HealthTimeout:   3 * time.Minute,
	}))
}

func TestIntegration(t *testing.T) {
	packages := []string{
		"github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/smregistry/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/integration_tests",
	}

	for _, pkg := range packages {
		pkg := pkg
		t.Run(strings.ReplaceAll(pkg, "/", "_"), func(t *testing.T) {
			t.Helper()
			resetDatabase(t)
			_, ok := allowedIntegrationPackages[pkg]
			require.True(t, ok, "unsupported integration package: %s", pkg)

			// #nosec G204 -- pkg is validated against a static allow-list above.
			cmd := exec.Command("go", "test", "-v", "-count=1", pkg)
			cmd.Env = append(os.Environ(), "BASYX_EXTERNAL_COMPOSE=1")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			require.NoError(t, cmd.Run(), "failed integration package: %s", pkg)
		})
	}
}

func resetDatabase(t *testing.T) {
	t.Helper()

	db, err := sql.Open("postgres", integrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	rows, err := db.Query("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	tables := make([]string, 0, 64)
	for rows.Next() {
		var table string
		require.NoError(t, rows.Scan(&table))
		tables = append(tables, table)
	}
	require.NoError(t, rows.Err())

	for _, table := range tables {
		truncateQuery := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", quoteIdentifier(table))
		_, execErr := db.Exec(truncateQuery)
		require.NoErrorf(t, execErr, "failed to truncate table %s", table)
	}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
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
	defer func() {
		_ = resp.Body.Close()
	}()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Verify content in repositories
	// Wait a bit for persistence
	time.Sleep(2 * time.Second)

	// Check AAS Repository
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	aasURL := fmt.Sprintf("http://localhost:6004/shells/%s", aasIdentifier)
	respAAS, err := http.Get(aasURL)
	require.NoError(t, err)
	defer func() {
		_ = respAAS.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, respAAS.StatusCode, "AAS should be found in AAS repository")

	// Check Submodel Repository
	smIdentifier := base64.RawURLEncoding.EncodeToString([]byte(smID))
	smURL := fmt.Sprintf("http://localhost:6003/submodels/%s", smIdentifier)
	respSM, err := http.Get(smURL)
	require.NoError(t, err)
	defer func() {
		_ = respSM.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, respSM.StatusCode, "Submodel should be found in Submodel repository")
}