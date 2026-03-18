package aasenvironment

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	aasrepo "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	cdrepo "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	smrepo "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
)

type AASEnvironment struct {
	AasBackend *aasrepo.AssetAdministrationShellDatabase
	SmBackend  *smrepo.SubmodelDatabase
	CdBackend  *cdrepo.ConceptDescriptionBackend
}

func (s *AASEnvironment) LoadEnvironment(ctx context.Context, env types.IEnvironment, files map[string]io.Reader, ignoreDuplicates bool) error {
	if env == nil {
		return nil
	}

	// 1. Upload Concept Descriptions
	for _, cd := range env.ConceptDescriptions() {
		if err := s.uploadConceptDescription(ctx, cd, ignoreDuplicates); err != nil {
			return fmt.Errorf("AASENV-LOADENV-UPLOADCD: %w", err)
		}
	}

	// 2. Upload Submodels
	for _, sm := range env.Submodels() {
		if err := s.uploadSubmodel(ctx, sm, files, ignoreDuplicates); err != nil {
			return fmt.Errorf("AASENV-LOADENV-UPLOADSM: %w", err)
		}
	}

	// 3. Upload Shells
	for _, aas := range env.AssetAdministrationShells() {
		if err := s.uploadAAS(ctx, aas, files, ignoreDuplicates); err != nil {
			return fmt.Errorf("AASENV-LOADENV-UPLOADAAS: %w", err)
		}
	}

	return nil
}

func (s *AASEnvironment) uploadConceptDescription(ctx context.Context, cd types.IConceptDescription, ignoreDuplicates bool) error {
	id := cd.ID()
	existing, err := s.CdBackend.GetConceptDescriptionByID(ctx, id)
	if err != nil {
		// If not found, create it
		return s.CdBackend.CreateConceptDescription(ctx, cd)
	}

	if ignoreDuplicates {
		return nil
	}

	if shouldUpdateIdentifiable(existing, cd) {
		return s.CdBackend.PutConceptDescription(ctx, id, cd)
	}

	return nil
}

func (s *AASEnvironment) uploadSubmodel(ctx context.Context, sm types.ISubmodel, files map[string]io.Reader, ignoreDuplicates bool) error {
	id := sm.ID()
	existing, err := s.SmBackend.GetSubmodelByIDWithContext(ctx, id, "")
	if err != nil {
		if err := s.SmBackend.CreateSubmodelWithContext(ctx, sm); err != nil {
			return fmt.Errorf("AASE-UPSM-CREATESM: %w", err)
		}
	} else {
		if !ignoreDuplicates && shouldUpdateIdentifiable(existing, sm) {
			if _, err := s.SmBackend.PutSubmodelWithContext(ctx, id, sm); err != nil {
				return fmt.Errorf("AASE-UPSM-UPDATESM: %w", err)
			}
		}
	}

	// Handle internal files
	fileElements := collectFileElements(sm)
	for _, fe := range fileElements {
		if fe.element.Value() != nil && *fe.element.Value() != "" {
			path := *fe.element.Value()
			// Some AASX packages use absolute paths or relative paths with leading /
			cleanPath := strings.TrimPrefix(path, "/")
			if reader, ok := files[cleanPath]; ok {
				fileName := cleanPath
				if idx := strings.LastIndex(cleanPath, "/"); idx != -1 {
					fileName = cleanPath[idx+1:]
				}
				if err := s.uploadFileToSM(ctx, id, fe.path, fileName, reader); err != nil {
					return fmt.Errorf("AASE-UPSM-UPLOADFILE: %w", err)
				}
			}
		}
	}

	return nil
}

func (s *AASEnvironment) uploadAAS(ctx context.Context, aas types.IAssetAdministrationShell, files map[string]io.Reader, ignoreDuplicates bool) error {
	id := aas.ID()
	_, err := s.AasBackend.GetAssetAdministrationShellByID(ctx, id)
	if err != nil {
		if err := s.AasBackend.CreateAssetAdministrationShell(ctx, aas); err != nil {
			return err
		}
	} else {
		if !ignoreDuplicates {
			if _, err := s.AasBackend.PutAssetAdministrationShellByID(ctx, id, aas); err != nil {
				return err
			}
		}
	}

	// Handle Thumbnail
	if aas.AssetInformation() != nil && aas.AssetInformation().DefaultThumbnail() != nil {
		thumb := aas.AssetInformation().DefaultThumbnail()
		if thumb.Path() != "" {
			if reader, ok := files[thumb.Path()]; ok {
				if err := s.uploadThumbnailToAAS(ctx, id, thumb.Path(), reader); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (s *AASEnvironment) uploadFileToSM(_ context.Context, smID, path, fileName string, reader io.Reader) error {
	tmpFile, err := copyToTempFile("aas-file-*", reader)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
		_ = tmpFile.Close()
	}()

	return s.SmBackend.UploadFileAttachment(smID, path, tmpFile, fileName)
}

func (s *AASEnvironment) uploadThumbnailToAAS(ctx context.Context, aasID, fileName string, reader io.Reader) error {
	tmpFile, err := copyToTempFile("aas-thumb-*", reader)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
		_ = tmpFile.Close()
	}()

	return s.AasBackend.PutThumbnailByAASID(ctx, aasID, fileName, tmpFile)
}

func copyToTempFile(pattern string, reader io.Reader) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return nil, err
	}

	if _, err := tmpFile.Seek(0, 0); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return nil, err
	}

	return tmpFile, nil
}

func shouldUpdateIdentifiable(existing, new types.IIdentifiable) bool {
	if existing.Administration() == nil && new.Administration() == nil {
		return false
	}
	if existing.Administration() == nil || new.Administration() == nil {
		return true
	}
	eAdmin := existing.Administration()
	nAdmin := new.Administration()
	return eAdmin.Version() != nAdmin.Version() || eAdmin.Revision() != nAdmin.Revision()
}

type fileElementInfo struct {
	path    string
	element types.IFile
}

func collectFileElements(sm types.ISubmodel) []fileElementInfo {
	return collectRecursive(sm.SubmodelElements(), "")
}

func collectRecursive(elements []types.ISubmodelElement, currentPath string) []fileElementInfo {
	var results []fileElementInfo
	for _, element := range elements {
		idShort := ""
		if element.IDShort() != nil {
			idShort = *element.IDShort()
		}
		path := idShort
		if currentPath != "" {
			path = currentPath + "." + idShort
		}

		if fe, ok := element.(types.IFile); ok {
			results = append(results, fileElementInfo{
				path:    path,
				element: fe,
			})
		} else if sec, ok := element.(types.ISubmodelElementCollection); ok {
			results = append(results, collectRecursive(sec.Value(), path)...)
		} else if sel, ok := element.(types.ISubmodelElementList); ok {
			results = append(results, collectRecursive(sel.Value(), path)...)
		}
	}
	return results
}
