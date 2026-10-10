package credentialfile

import (
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

const maxCredentialSize = 65536

type Reader struct{}

func New() *Reader {
	return &Reader{}
}

func (r *Reader) ReadClientID(path string) (string, error) {
	credential, err := openCredential(path)
	if err != nil {
		return "", err
	}
	value, readErr := credential.readValue()
	closeErr := credential.close()
	if readErr != nil {
		return "", readErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return value, nil
}

func (r *Reader) ReadPair(clientIDPath, clientSecretPath string) (string, string, error) {
	clientID, err := openCredential(clientIDPath)
	if err != nil {
		return "", "", err
	}
	clientSecret, err := openCredential(clientSecretPath)
	if err != nil {
		_ = clientID.close()
		return "", "", err
	}
	clientIDValue, err := clientID.readValue()
	if err != nil {
		_ = clientID.close()
		_ = clientSecret.close()
		return "", "", err
	}
	clientSecretValue, err := clientSecret.readValue()
	if err != nil {
		_ = clientID.close()
		_ = clientSecret.close()
		return "", "", err
	}
	return finishPair(clientID, clientSecret, clientIDValue, clientSecretValue)
}

type openedCredential struct {
	path string
	file *os.File
	info os.FileInfo
}

func openCredential(path string) (*openedCredential, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		if info, statErr := os.Stat(path); statErr == nil && !info.Mode().IsRegular() {
			return nil, model.NewCredentialSourceError(model.CredentialSourceReasonNotRegular)
		}
		return nil, credentialError(err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, credentialError(err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, model.NewCredentialSourceError(model.CredentialSourceReasonNotRegular)
	}
	if info.Size() > maxCredentialSize {
		_ = file.Close()
		return nil, model.NewCredentialSourceError(model.CredentialSourceReasonTooLarge)
	}
	return &openedCredential{path: path, file: file, info: info}, nil
}

func (c *openedCredential) readValue() (string, error) {
	data, err := io.ReadAll(io.LimitReader(c.file, maxCredentialSize+1))
	if err != nil {
		return "", credentialError(err)
	}
	if len(data) > maxCredentialSize {
		return "", model.NewCredentialSourceError(model.CredentialSourceReasonTooLarge)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", model.NewCredentialSourceError(model.CredentialSourceReasonEmpty)
	}
	return value, nil
}

func (c *openedCredential) close() error {
	if err := c.file.Close(); err != nil {
		return credentialError(err)
	}
	return nil
}

func finishPair(clientID, clientSecret *openedCredential, clientIDValue, clientSecretValue string) (string, string, error) {
	currentID, idErr := os.Stat(clientID.path)
	currentSecret, secretErr := os.Stat(clientSecret.path)
	var validationErr error
	switch {
	case idErr != nil:
		validationErr = credentialError(idErr)
	case secretErr != nil:
		validationErr = credentialError(secretErr)
	case !os.SameFile(clientID.info, currentID) || !os.SameFile(clientSecret.info, currentSecret):
		validationErr = model.NewCredentialSourceError(model.CredentialSourceReasonGenerationChanged)
	}
	idCloseErr := clientID.close()
	secretCloseErr := clientSecret.close()
	if validationErr != nil {
		return "", "", validationErr
	}
	if idCloseErr != nil {
		return "", "", idCloseErr
	}
	if secretCloseErr != nil {
		return "", "", secretCloseErr
	}
	return clientIDValue, clientSecretValue, nil
}

func credentialError(err error) *model.CredentialSourceError {
	reason := model.CredentialSourceReasonReadFailed
	switch {
	case os.IsNotExist(err):
		reason = model.CredentialSourceReasonNotFound
	case os.IsPermission(err):
		reason = model.CredentialSourceReasonPermissionDenied
	}
	return model.NewCredentialSourceError(reason)
}
