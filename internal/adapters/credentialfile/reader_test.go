//go:build darwin || linux

package credentialfile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const readerLimit = 65536

func TestReaderNormalizesCurrentValues(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain", "client-or-secret", "client-or-secret"},
		{"surrounding_ascii_whitespace", " \t\r\nclient-or-secret\r\n\t ", "client-or-secret"},
		{"surrounding_unicode_whitespace", "\u2003\u00a0client-or-secret\u2009\n", "client-or-secret"},
		{"internal_characters", " \talpha beta\tline\nnext\r\n+:/=\x00\u2003omega \n", "alpha beta\tline\nnext\r\n+:/=\x00\u2003omega"},
		{"exact_limit", strings.Repeat("x", readerLimit), strings.Repeat("x", readerLimit)},
		{"exact_limit_before_trimming", "\n" + strings.Repeat("x", readerLimit-2) + "\n", strings.Repeat("x", readerLimit-2)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			valuePath := writeReaderFile(t, directory, "value", tc.raw)
			otherPath := writeReaderFile(t, directory, "other", "other-value")
			reader := New()

			got, err := reader.ReadClientID(valuePath)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)

			clientID, secret, err := reader.ReadPair(valuePath, otherPath)
			require.NoError(t, err)
			assert.Equal(t, tc.want, clientID)
			assert.Equal(t, "other-value", secret)

			clientID, secret, err = reader.ReadPair(otherPath, valuePath)
			require.NoError(t, err)
			assert.Equal(t, "other-value", clientID)
			assert.Equal(t, tc.want, secret)
		})
	}
}

func TestReaderAllowsTwoExactLimitFilesInOnePair(t *testing.T) {
	directory := t.TempDir()
	wantID, wantSecret := strings.Repeat("i", readerLimit), strings.Repeat("s", readerLimit)
	idPath := writeReaderFile(t, directory, "client_id", wantID)
	secretPath := writeReaderFile(t, directory, "client_secret", wantSecret)
	clientID, secret, err := New().ReadPair(idPath, secretPath)
	require.NoError(t, err)
	assert.Equal(t, wantID, clientID)
	assert.Equal(t, wantSecret, secret)
}

func TestReaderClientIDAcquisitionsAreFreshWithoutAnyPairReads(t *testing.T) {
	projection := newReaderProjection(t)
	reader := New()
	value, err := reader.ReadClientID(projection.idPath)
	require.NoError(t, err)
	assert.Equal(t, "client-0", value)
	projection.publish(t, "next", "client-next", "secret-next")
	value, err = reader.ReadClientID(projection.idPath)
	require.NoError(t, err)
	assert.Equal(t, "client-next", value)
	require.NoError(t, os.Remove(projection.idPath))
	for _, candidate := range []*Reader{reader, New()} {
		value, err = candidate.ReadClientID(projection.idPath)
		assert.Empty(t, value)
		assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonNotFound}, projection.idPath, "client-0", "client-next")
	}
	require.NoError(t, os.Symlink(filepath.Join("..data", "client_id"), projection.idPath))
	projection.publish(t, "recovered", "client-recovered", "secret-recovered")
	value, err = reader.ReadClientID(projection.idPath)
	require.NoError(t, err)
	assert.Equal(t, "client-recovered", value)
}

func TestReaderUsesOnlySuppliedPathsWithoutCommonParentRestriction(t *testing.T) {
	clientIDPath := writeReaderFile(t, t.TempDir(), "not-a-client-id-name", "selected-id")
	secretPath := writeReaderFile(t, t.TempDir(), "not-a-secret-name", "selected-secret")
	writeReaderFile(t, filepath.Dir(clientIDPath), "client_secret", "unselected-secret")
	writeReaderFile(t, filepath.Dir(secretPath), "client_id", "unselected-id")

	clientID, secret, err := New().ReadPair(clientIDPath, secretPath)
	require.NoError(t, err)
	assert.Equal(t, "selected-id", clientID)
	assert.Equal(t, "selected-secret", secret)
}

func TestReaderRejectsEachUnusableRequiredFileWithoutPartialValues(t *testing.T) {
	cases := []struct {
		name   string
		reason model.CredentialSourceReason
		create func(*testing.T, string) string
	}{
		{"missing", model.CredentialSourceReasonNotFound, func(t *testing.T, directory string) string {
			return filepath.Join(directory, "private-missing-path")
		}},
		{"broken_symlink", model.CredentialSourceReasonNotFound, func(t *testing.T, directory string) string {
			path := filepath.Join(directory, "private-broken-link")
			require.NoError(t, os.Symlink(filepath.Join(directory, "private-missing-target"), path))
			return path
		}},
		{"symlink_loop", model.CredentialSourceReasonReadFailed, func(t *testing.T, directory string) string {
			path := filepath.Join(directory, "private-loop-path")
			require.NoError(t, os.Symlink(filepath.Base(path), path))
			return path
		}},
		{"empty", model.CredentialSourceReasonEmpty, func(t *testing.T, directory string) string {
			return writeReaderFile(t, directory, "private-empty-path", "")
		}},
		{"whitespace", model.CredentialSourceReasonEmpty, func(t *testing.T, directory string) string {
			return writeReaderFile(t, directory, "private-whitespace-path", " \t\r\n\u2003\u00a0")
		}},
		{"oversized", model.CredentialSourceReasonTooLarge, func(t *testing.T, directory string) string {
			return writeReaderFile(t, directory, "private-oversized-path", strings.Repeat("private-credential-marker", 3000))
		}},
		{"one_byte_over_limit", model.CredentialSourceReasonTooLarge, func(t *testing.T, directory string) string {
			return writeReaderFile(t, directory, "private-limit-path", strings.Repeat("x", readerLimit+1))
		}},
		{"oversized_before_trimming", model.CredentialSourceReasonTooLarge, func(t *testing.T, directory string) string {
			return writeReaderFile(t, directory, "private-padded-path", strings.Repeat(" ", readerLimit)+"x")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			badPath := tc.create(t, directory)
			goodPath := writeReaderFile(t, directory, "private-good-path", "private-good-value")
			reader := New()

			value, err := reader.ReadClientID(badPath)
			assert.Empty(t, value)
			assertReaderSourceError(t, err, []model.CredentialSourceReason{tc.reason}, badPath, goodPath, "private-good-value", "private-credential-marker")

			for _, badField := range []string{"client_id", "client_secret"} {
				t.Run(badField, func(t *testing.T) {
					idPath, secretPath := badPath, goodPath
					if badField == "client_secret" {
						idPath, secretPath = goodPath, badPath
					}
					clientID, secret, err := reader.ReadPair(idPath, secretPath)
					assert.Empty(t, clientID)
					assert.Empty(t, secret)
					assertReaderSourceError(t, err, []model.CredentialSourceReason{tc.reason}, badPath, goodPath, "private-good-value", "private-credential-marker")
				})
			}
		})
	}
}

func TestReaderFollowsCurrentProjectedGenerationAndDoesNotKeepLastKnownValues(t *testing.T) {
	projection := newReaderProjection(t)
	reader := New()
	for generation := range 3 {
		if generation != 0 {
			projection.publish(t, fmt.Sprintf("generation-%d", generation), fmt.Sprintf("client-%d", generation), fmt.Sprintf("secret-%d", generation))
		}
		clientID, secret, err := reader.ReadPair(projection.idPath, projection.secretPath)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("client-%d", generation), clientID)
		assert.Equal(t, fmt.Sprintf("secret-%d", generation), secret)
		value, err := reader.ReadClientID(projection.idPath)
		require.NoError(t, err)
		assert.Equal(t, clientID, value)
	}

	for _, field := range []string{"client_id", "client_secret"} {
		t.Run(field, func(t *testing.T) {
			projection := newReaderProjection(t)
			reader := New()
			_, _, err := reader.ReadPair(projection.idPath, projection.secretPath)
			require.NoError(t, err)
			lostPath := projection.idPath
			if field == "client_secret" {
				lostPath = projection.secretPath
			}
			require.NoError(t, os.Remove(lostPath))
			for _, candidate := range []*Reader{reader, New()} {
				clientID, secret, err := candidate.ReadPair(projection.idPath, projection.secretPath)
				assert.Empty(t, clientID)
				assert.Empty(t, secret)
				assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonNotFound}, projection.idPath, projection.secretPath, "client-0", "secret-0")
			}
			if field == "client_id" {
				value, err := reader.ReadClientID(lostPath)
				assert.Empty(t, value)
				assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonNotFound}, lostPath, "client-0")
			}
			require.NoError(t, os.Symlink(filepath.Join("..data", field), lostPath))
			projection.publish(t, "recovered", "client-recovered", "secret-recovered")
			clientID, secret, err := reader.ReadPair(projection.idPath, projection.secretPath)
			require.NoError(t, err)
			assert.Equal(t, "client-recovered", clientID)
			assert.Equal(t, "secret-recovered", secret)
		})
	}
}

func TestOpenedCredentialKeepsReadOnlyNonblockingDescriptorUntilClose(t *testing.T) {
	path := writeReaderFile(t, t.TempDir(), "credential", " \tinside value\n")
	opened := openReaderCredential(t, path)
	fd := int(opened.file.Fd())
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	require.NoError(t, err)
	assert.Equal(t, unix.O_RDONLY, flags&unix.O_ACCMODE)
	assert.NotZero(t, flags&unix.O_NONBLOCK)
	assert.Equal(t, path, opened.path)
	require.NotNil(t, opened.info)
	assert.True(t, opened.info.Mode().IsRegular())
	currentInfo, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, os.SameFile(opened.info, currentInfo))

	value, err := opened.readValue()
	require.NoError(t, err)
	assert.Equal(t, "inside value", value)
	assertReaderDescriptorOpen(t, fd)
	require.NoError(t, opened.close())
	assertReaderDescriptorClosed(t, fd)
}

func TestOpenedCredentialBoundedReadRejectsGrowthAfterDescriptorInspection(t *testing.T) {
	for _, extra := range []string{"x", strings.Repeat("x", readerLimit)} {
		t.Run(fmt.Sprintf("growth_%d", len(extra)), func(t *testing.T) {
			path := writeReaderFile(t, t.TempDir(), "private-growing-path", strings.Repeat("x", readerLimit))
			opened := openReaderCredential(t, path)
			fd := int(opened.file.Fd())
			require.Equal(t, int64(readerLimit), opened.info.Size())
			writer, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
			require.NoError(t, err)
			_, err = writer.WriteString(extra)
			require.NoError(t, err)
			require.NoError(t, writer.Close())

			value, err := opened.readValue()
			assert.Empty(t, value)
			assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonTooLarge}, path)
			offset, seekErr := unix.Seek(fd, 0, io.SeekCurrent)
			require.NoError(t, seekErr)
			assert.Equal(t, int64(readerLimit+1), offset, "bounded reads must consume at most one extra byte")
			require.NoError(t, opened.close())
			assertReaderDescriptorClosed(t, fd)
		})
	}
}

func TestOpenedCredentialBoundedReadRejectsEmptyAfterDescriptorInspection(t *testing.T) {
	path := writeReaderFile(t, t.TempDir(), "private-truncated-path", "previous-private-value")
	opened := openReaderCredential(t, path)
	fd := int(opened.file.Fd())
	require.Positive(t, opened.info.Size())
	require.NoError(t, os.Truncate(path, 0))

	value, err := opened.readValue()
	assert.Empty(t, value)
	assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonEmpty}, path, "previous-private-value")
	require.NoError(t, opened.close())
	assertReaderDescriptorClosed(t, fd)
}

func TestPairAcquisitionRejectsImmutableGenerationChangesAtEveryStage(t *testing.T) {
	for _, boundary := range []string{"between_opens", "before_reads", "between_reads", "before_revalidation"} {
		t.Run(boundary, func(t *testing.T) {
			projection := newReaderProjection(t)
			clientID := openReaderCredential(t, projection.idPath)
			if boundary == "between_opens" {
				projection.publish(t, "next", "client-next", "secret-next")
			}
			secret := openReaderCredential(t, projection.secretPath)
			idFD, secretFD := int(clientID.file.Fd()), int(secret.file.Fd())
			if boundary == "before_reads" {
				projection.publish(t, "next", "client-next", "secret-next")
			}
			idValue, err := clientID.readValue()
			require.NoError(t, err)
			assert.Equal(t, "client-0", idValue)
			if boundary == "between_reads" {
				projection.publish(t, "next", "client-next", "secret-next")
			}
			secretValue, err := secret.readValue()
			require.NoError(t, err)
			wantSecret := "secret-0"
			if boundary == "between_opens" {
				wantSecret = "secret-next"
			}
			assert.Equal(t, wantSecret, secretValue)
			if boundary == "before_revalidation" {
				projection.publish(t, "next", "client-next", "secret-next")
			}
			currentID, err := os.Stat(projection.idPath)
			require.NoError(t, err)
			require.False(t, os.SameFile(clientID.info, currentID), "the fixture must change the actual opened target")
			assertReaderDescriptorOpen(t, idFD)
			assertReaderDescriptorOpen(t, secretFD)

			gotID, gotSecret, err := finishPair(clientID, secret, idValue, secretValue)
			assert.Empty(t, gotID)
			assert.Empty(t, gotSecret)
			assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonGenerationChanged}, projection.idPath, projection.secretPath, idValue, secretValue)
			assertReaderDescriptorClosed(t, idFD)
			assertReaderDescriptorClosed(t, secretFD)

			gotID, gotSecret, err = New().ReadPair(projection.idPath, projection.secretPath)
			require.NoError(t, err)
			assert.Equal(t, "client-next", gotID)
			assert.Equal(t, "secret-next", gotSecret)
		})
	}
}

func TestPairRevalidationChecksBothTargetsIndependently(t *testing.T) {
	for _, changedField := range []string{"client_id", "client_secret"} {
		t.Run(changedField, func(t *testing.T) {
			directory := t.TempDir()
			idPath := writeReaderFile(t, directory, "client_id", "same-client")
			secretPath := writeReaderFile(t, directory, "client_secret", "old-secret")
			clientID := openReaderCredential(t, idPath)
			secret := openReaderCredential(t, secretPath)
			idFD, secretFD := int(clientID.file.Fd()), int(secret.file.Fd())
			idValue, err := clientID.readValue()
			require.NoError(t, err)
			secretValue, err := secret.readValue()
			require.NoError(t, err)
			changedPath, replacement := idPath, "same-client"
			if changedField == "client_secret" {
				changedPath, replacement = secretPath, "next-secret"
			}
			freshPath := writeReaderFile(t, directory, "fresh-target", replacement)
			require.NoError(t, os.Rename(freshPath, changedPath))

			gotID, gotSecret, err := finishPair(clientID, secret, idValue, secretValue)
			assert.Empty(t, gotID)
			assert.Empty(t, gotSecret)
			assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonGenerationChanged}, idPath, secretPath, idValue, secretValue)
			assertReaderDescriptorClosed(t, idFD)
			assertReaderDescriptorClosed(t, secretFD)
			gotID, gotSecret, err = New().ReadPair(idPath, secretPath)
			require.NoError(t, err)
			assert.Equal(t, "same-client", gotID)
			wantSecret := "old-secret"
			if changedField == "client_secret" {
				wantSecret = "next-secret"
			}
			assert.Equal(t, wantSecret, gotSecret)
		})
	}
}

func TestPairRevalidationClassifiesDisappearedTargetsAndClosesBoth(t *testing.T) {
	for _, missingField := range []string{"client_id", "client_secret"} {
		t.Run(missingField, func(t *testing.T) {
			projection := newReaderProjection(t)
			clientID := openReaderCredential(t, projection.idPath)
			secret := openReaderCredential(t, projection.secretPath)
			idFD, secretFD := int(clientID.file.Fd()), int(secret.file.Fd())
			idValue, err := clientID.readValue()
			require.NoError(t, err)
			secretValue, err := secret.readValue()
			require.NoError(t, err)
			missingPath := projection.idPath
			if missingField == "client_secret" {
				missingPath = projection.secretPath
			}
			require.NoError(t, os.Remove(missingPath))

			gotID, gotSecret, err := finishPair(clientID, secret, idValue, secretValue)
			assert.Empty(t, gotID)
			assert.Empty(t, gotSecret)
			assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonNotFound}, missingPath, idValue, secretValue)
			assertReaderDescriptorClosed(t, idFD)
			assertReaderDescriptorClosed(t, secretFD)
		})
	}
}

func TestPairValidatedBeforeLaterPublicationKeepsOnlyThatOperationPair(t *testing.T) {
	projection := newReaderProjection(t)
	clientID := openReaderCredential(t, projection.idPath)
	secret := openReaderCredential(t, projection.secretPath)
	idFD, secretFD := int(clientID.file.Fd()), int(secret.file.Fd())
	idValue, err := clientID.readValue()
	require.NoError(t, err)
	secretValue, err := secret.readValue()
	require.NoError(t, err)
	gotID, gotSecret, err := finishPair(clientID, secret, idValue, secretValue)
	require.NoError(t, err)
	assertReaderDescriptorClosed(t, idFD)
	assertReaderDescriptorClosed(t, secretFD)

	projection.publish(t, "next", "client-next", "secret-next")
	assert.Equal(t, "client-0", gotID)
	assert.Equal(t, "secret-0", gotSecret)
	gotID, gotSecret, err = New().ReadPair(projection.idPath, projection.secretPath)
	require.NoError(t, err)
	assert.Equal(t, "client-next", gotID)
	assert.Equal(t, "secret-next", gotSecret)
}

func TestReaderNonRegularSourcesFailPromptly(t *testing.T) {
	process := newReaderProcessFixture(t)
	goodPath := writeReaderFile(t, process.directory, "private-good-path", "private-good-value")
	directory := filepath.Join(process.directory, "private-directory")
	require.NoError(t, os.Mkdir(directory, 0o755))
	fifo := filepath.Join(process.directory, "private-fifo")
	require.NoError(t, unix.Mkfifo(fifo, 0o644))
	socket := filepath.Join(process.directory, "socket")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })

	for _, tc := range []struct {
		name    string
		path    string
		reasons string
	}{
		{"directory", directory, "not_regular"},
		{"fifo", fifo, "not_regular"},
		{"socket", socket, "not_regular,read_failed"},
		{"null_device", "/dev/null", "not_regular"},
		{"zero_device", "/dev/zero", "not_regular"},
	} {
		for _, operation := range []string{"client_id", "pair_id", "pair_secret"} {
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				process.run(t, false, map[string]string{
					"MODE": "failure", "OPERATION": operation, "BAD_PATH": tc.path,
					"GOOD_PATH": goodPath, "REASONS": tc.reasons,
				})
			})
		}
	}
}

func TestReaderClientIDDoesNotRequireOrReadSiblingSecret(t *testing.T) {
	process := newReaderProcessFixture(t)
	idPath := writeReaderFile(t, process.directory, "client_id", " \tselected-client\n")
	secretPath := filepath.Join(process.directory, "client_secret")
	for _, state := range []string{"absent", "empty", "unreadable", "fifo"} {
		t.Run(state, func(t *testing.T) {
			if state != "absent" {
				switch state {
				case "empty", "unreadable":
					writeReaderFile(t, process.directory, "client_secret", "")
					if state == "unreadable" {
						require.NoError(t, os.Chmod(secretPath, 0))
					}
				case "fifo":
					require.NoError(t, unix.Mkfifo(secretPath, 0o644))
				}
				t.Cleanup(func() { require.NoError(t, os.Remove(secretPath)) })
			}
			process.run(t, true, map[string]string{"MODE": "client_id", "GOOD_PATH": idPath})
		})
	}
}

func TestReaderPermissionDenialUsesRealUnprivilegedProcess(t *testing.T) {
	process := newReaderProcessFixture(t)
	goodPath := writeReaderFile(t, process.directory, "private-good-path", "private-good-value")
	deniedPath := writeReaderFile(t, process.directory, "private-denied-path", "private-denied-value")
	require.NoError(t, os.Chmod(deniedPath, 0))
	deniedDirectory := filepath.Join(process.directory, "private-denied-directory")
	require.NoError(t, os.Mkdir(deniedDirectory, 0o755))
	traversalPath := writeReaderFile(t, deniedDirectory, "private-traversal-path", "private-denied-value")
	require.NoError(t, os.Chmod(deniedDirectory, 0))
	t.Cleanup(func() { require.NoError(t, os.Chmod(deniedDirectory, 0o755)) })

	for _, badPath := range []string{deniedPath, traversalPath} {
		for _, operation := range []string{"client_id", "pair_id", "pair_secret"} {
			t.Run(filepath.Base(badPath)+"/"+operation, func(t *testing.T) {
				process.run(t, true, map[string]string{
					"MODE": "permission", "OPERATION": operation, "BAD_PATH": badPath,
					"GOOD_PATH": goodPath, "REASONS": "permission_denied",
				})
			})
		}
	}
}

func TestPairRevalidationPermissionDenialClosesBothDescriptors(t *testing.T) {
	process := newReaderProcessFixture(t)
	process.run(t, true, map[string]string{"MODE": "revalidation_permission"})
}

func TestReaderClosesPublicOperationDescriptorsOnEveryExit(t *testing.T) {
	process := newReaderProcessFixture(t)
	writeReaderFile(t, process.directory, "client_id", "private-client-id")
	writeReaderFile(t, process.directory, "client_secret", "private-secret")
	writeReaderFile(t, process.directory, "empty", "")
	writeReaderFile(t, process.directory, "whitespace", " \t\r\n")
	writeReaderFile(t, process.directory, "oversized", strings.Repeat("x", readerLimit+1))
	process.run(t, false, map[string]string{"MODE": "descriptors", "DIRECTORY": process.directory})
}

// Child processes put a deadline around real opens and permit permission checks even when the suite is root.
func TestCredentialReaderProcess(t *testing.T) {
	mode := os.Getenv("AIB_CREDENTIAL_READER_MODE")
	if mode == "" {
		return
	}
	limit := readerChildDescriptorLimit(t)
	goodPath := os.Getenv("AIB_CREDENTIAL_READER_GOOD_PATH")
	if mode == "client_id" {
		value, err := New().ReadClientID(goodPath)
		require.NoError(t, err)
		assert.Equal(t, "selected-client", value)
		assertNoReaderDescriptors(t, limit, []string{goodPath})
		return
	}
	if mode == "descriptors" {
		checkReaderPublicDescriptors(t, os.Getenv("AIB_CREDENTIAL_READER_DIRECTORY"), limit)
		return
	}
	if mode == "revalidation_permission" {
		checkReaderRevalidationPermissions(t)
		return
	}
	require.Contains(t, []string{"failure", "permission"}, mode)
	badPath := os.Getenv("AIB_CREDENTIAL_READER_BAD_PATH")
	if mode == "permission" {
		require.NotZero(t, os.Geteuid(), "permission tests must exercise a non-root reader")
		contents, err := os.ReadFile(goodPath)
		require.NoError(t, err, "all shared fixture ancestors must remain traversable")
		require.Equal(t, "private-good-value", string(contents))
		file, err := os.Open(badPath)
		if file != nil {
			require.NoError(t, file.Close())
		}
		require.ErrorIs(t, err, os.ErrPermission, "the actual operating system must deny this fixture")
	}
	var reasons []model.CredentialSourceReason
	for _, reason := range strings.Split(os.Getenv("AIB_CREDENTIAL_READER_REASONS"), ",") {
		reasons = append(reasons, model.CredentialSourceReason(reason))
	}
	reader := New()
	var badDescriptorsBefore []int
	if mode == "failure" {
		badDescriptorsBefore = readerTargetDescriptors(t, limit, badPath)
	}
	switch os.Getenv("AIB_CREDENTIAL_READER_OPERATION") {
	case "client_id":
		value, err := reader.ReadClientID(badPath)
		assert.Empty(t, value)
		assertReaderSourceError(t, err, reasons, badPath, goodPath, "private-good-value", "private-denied-value")
	case "pair_id":
		clientID, secret, err := reader.ReadPair(badPath, goodPath)
		assert.Empty(t, clientID)
		assert.Empty(t, secret)
		assertReaderSourceError(t, err, reasons, badPath, goodPath, "private-good-value", "private-denied-value")
	case "pair_secret":
		clientID, secret, err := reader.ReadPair(goodPath, badPath)
		assert.Empty(t, clientID)
		assert.Empty(t, secret)
		assertReaderSourceError(t, err, reasons, badPath, goodPath, "private-good-value", "private-denied-value")
	default:
		t.Fatal("unknown child reader operation")
	}
	assertNoReaderDescriptors(t, limit, []string{goodPath})
	if mode == "failure" {
		assert.Equal(t, badDescriptorsBefore, readerTargetDescriptors(t, limit, badPath), "a non-regular target descriptor must not remain open")
	}
}

func checkReaderRevalidationPermissions(t *testing.T) {
	t.Helper()
	require.NotZero(t, os.Geteuid())
	for _, deniedField := range []string{"client_id", "client_secret"} {
		t.Run(deniedField, func(t *testing.T) {
			directory, err := os.MkdirTemp("/tmp", "aib-reader-validation-")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, os.RemoveAll(directory)) })
			idDirectory, secretDirectory := filepath.Join(directory, "id"), filepath.Join(directory, "secret")
			require.NoError(t, os.Mkdir(idDirectory, 0o755))
			require.NoError(t, os.Mkdir(secretDirectory, 0o755))
			idPath := writeReaderFile(t, idDirectory, "client_id", "private-client-id")
			secretPath := writeReaderFile(t, secretDirectory, "client_secret", "private-secret")
			clientID := openReaderCredential(t, idPath)
			secret := openReaderCredential(t, secretPath)
			idFD, secretFD := int(clientID.file.Fd()), int(secret.file.Fd())
			idValue, err := clientID.readValue()
			require.NoError(t, err)
			secretValue, err := secret.readValue()
			require.NoError(t, err)
			deniedDirectory, deniedPath := idDirectory, idPath
			if deniedField == "client_secret" {
				deniedDirectory, deniedPath = secretDirectory, secretPath
			}
			t.Cleanup(func() { require.NoError(t, os.Chmod(deniedDirectory, 0o755)) })
			require.NoError(t, os.Chmod(deniedDirectory, 0))
			_, err = os.Stat(deniedPath)
			require.ErrorIs(t, err, os.ErrPermission)
			assertReaderDescriptorOpen(t, idFD)
			assertReaderDescriptorOpen(t, secretFD)

			gotID, gotSecret, err := finishPair(clientID, secret, idValue, secretValue)
			assert.Empty(t, gotID)
			assert.Empty(t, gotSecret)
			assertReaderSourceError(t, err, []model.CredentialSourceReason{model.CredentialSourceReasonPermissionDenied}, idPath, secretPath, idValue, secretValue)
			assertReaderDescriptorClosed(t, idFD)
			assertReaderDescriptorClosed(t, secretFD)

			require.NoError(t, os.Chmod(deniedDirectory, 0o755))
			gotID, gotSecret, err = New().ReadPair(idPath, secretPath)
			require.NoError(t, err)
			assert.Equal(t, "private-client-id", gotID)
			assert.Equal(t, "private-secret", gotSecret)
		})
	}
}

func checkReaderPublicDescriptors(t *testing.T, directory string, limit int) {
	t.Helper()
	idPath, secretPath := filepath.Join(directory, "client_id"), filepath.Join(directory, "client_secret")
	paths := []string{idPath, secretPath, filepath.Join(directory, "empty"), filepath.Join(directory, "whitespace"), filepath.Join(directory, "oversized")}
	reader := New()
	for range 32 {
		clientID, secret, err := reader.ReadPair(idPath, secretPath)
		require.NoError(t, err)
		require.Equal(t, "private-client-id", clientID)
		require.Equal(t, "private-secret", secret)
		assertNoReaderDescriptors(t, limit, paths)
		value, err := reader.ReadClientID(idPath)
		require.NoError(t, err)
		require.Equal(t, "private-client-id", value)
		assertNoReaderDescriptors(t, limit, paths)
	}
	for _, tc := range []struct {
		name   string
		reason model.CredentialSourceReason
	}{
		{"missing", model.CredentialSourceReasonNotFound},
		{"empty", model.CredentialSourceReasonEmpty},
		{"whitespace", model.CredentialSourceReasonEmpty},
		{"oversized", model.CredentialSourceReasonTooLarge},
	} {
		badPath := filepath.Join(directory, tc.name)
		value, err := reader.ReadClientID(badPath)
		assert.Empty(t, value)
		assertReaderSourceError(t, err, []model.CredentialSourceReason{tc.reason}, badPath)
		assertNoReaderDescriptors(t, limit, paths)
		for _, badFirst := range []bool{true, false} {
			first, second := badPath, secretPath
			if !badFirst {
				first, second = idPath, badPath
			}
			clientID, secret, err := reader.ReadPair(first, second)
			assert.Empty(t, clientID)
			assert.Empty(t, secret)
			assertReaderSourceError(t, err, []model.CredentialSourceReason{tc.reason}, badPath, "private-client-id", "private-secret")
			assertNoReaderDescriptors(t, limit, paths)
		}
	}
}

func readerChildDescriptorLimit(t *testing.T) int {
	t.Helper()
	var limit unix.Rlimit
	require.NoError(t, unix.Getrlimit(unix.RLIMIT_NOFILE, &limit))
	limit.Cur = min(limit.Max, 256)
	require.NoError(t, unix.Setrlimit(unix.RLIMIT_NOFILE, &limit))
	return int(limit.Cur)
}

func assertNoReaderDescriptors(t *testing.T, limit int, paths []string) {
	t.Helper()
	for _, path := range paths {
		assert.Empty(t, readerTargetDescriptors(t, limit, path), "credential descriptors retained after return")
	}
}

func readerTargetDescriptors(t *testing.T, limit int, path string) []int {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	identity, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	var descriptors []int
	for fd := range limit {
		_, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if errors.Is(err, unix.EBADF) {
			continue
		}
		require.NoError(t, err)
		var descriptor unix.Stat_t
		require.NoError(t, unix.Fstat(fd, &descriptor))
		if uint64(descriptor.Dev) == uint64(identity.Dev) && uint64(descriptor.Ino) == uint64(identity.Ino) {
			descriptors = append(descriptors, fd)
		}
	}
	return descriptors
}

func assertReaderSourceError(t *testing.T, err error, reasons []model.CredentialSourceReason, forbidden ...string) {
	t.Helper()
	require.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
	var sourceError *model.CredentialSourceError
	require.ErrorAs(t, err, &sourceError)
	assert.Contains(t, reasons, sourceError.Reason)
	var pathError *os.PathError
	assert.False(t, errors.As(err, &pathError), "raw PathError must not cross the reader boundary")
	var errno syscall.Errno
	assert.False(t, errors.As(err, &errno), "raw OS errors must not remain in the source-error chain")
	current := err
	for range 8 {
		if current == nil {
			break
		}
		printable := fmt.Sprintf("%s | %v | %+v | %#v", current, current, current, current)
		assert.Contains(t, printable, model.ErrCredentialSourceUnavailable.Error())
		for _, value := range forbidden {
			if value != "" {
				assert.NotContains(t, printable, value)
			}
		}
		current = errors.Unwrap(current)
	}
	assert.Nil(t, current, "source errors must have a bounded safe chain")
}

func assertReaderDescriptorOpen(t *testing.T, fd int) {
	t.Helper()
	_, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	require.NoError(t, err, "credential descriptors stay open until pair validation")
}

func assertReaderDescriptorClosed(t *testing.T, fd int) {
	t.Helper()
	_, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	assert.ErrorIs(t, err, unix.EBADF, "the actual credential descriptor must be closed")
}

func openReaderCredential(t *testing.T, path string) *openedCredential {
	t.Helper()
	opened, err := openCredential(path)
	require.NoError(t, err)
	require.NotNil(t, opened)
	require.NotNil(t, opened.file)
	t.Cleanup(func() {
		if _, err := opened.file.Stat(); err == nil {
			require.NoError(t, opened.file.Close())
		}
	})
	offset, seekErr := unix.Seek(int(opened.file.Fd()), 0, io.SeekCurrent)
	require.NoError(t, seekErr)
	assert.Zero(t, offset, "opening and inspecting a descriptor must not read its value")
	return opened
}

func writeReaderFile(t *testing.T, directory, name, value string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	require.NoError(t, os.WriteFile(path, []byte(value), 0o644))
	require.NoError(t, os.Chmod(path, 0o644))
	return path
}

type readerProjection struct {
	directory  string
	idPath     string
	secretPath string
}

func newReaderProjection(t *testing.T) *readerProjection {
	t.Helper()
	projection := &readerProjection{directory: t.TempDir()}
	projection.idPath = filepath.Join(projection.directory, "client_id")
	projection.secretPath = filepath.Join(projection.directory, "client_secret")
	projection.publish(t, "generation-0", "client-0", "secret-0")
	require.NoError(t, os.Symlink(filepath.Join("..data", "client_id"), projection.idPath))
	require.NoError(t, os.Symlink(filepath.Join("..data", "client_secret"), projection.secretPath))
	return projection
}

func (p *readerProjection) publish(t *testing.T, generation, clientID, secret string) {
	t.Helper()
	directory := filepath.Join(p.directory, generation)
	require.NoError(t, os.Mkdir(directory, 0o755))
	writeReaderFile(t, directory, "client_id", clientID)
	writeReaderFile(t, directory, "client_secret", secret)
	next := filepath.Join(p.directory, "..next")
	require.NoError(t, os.Symlink(generation, next))
	require.NoError(t, os.Rename(next, filepath.Join(p.directory, "..data")))
}

type readerProcessFixture struct {
	directory string
	binary    string
	identity  *syscall.Credential
}

func newReaderProcessFixture(t *testing.T) *readerProcessFixture {
	t.Helper()
	// /tmp avoids private ancestors in platform-specific testing temporary directories.
	directory, err := os.MkdirTemp("/tmp", "aib-reader-")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(directory, 0o755))
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(directory)) })
	binary, err := os.Executable()
	require.NoError(t, err)
	fixture := &readerProcessFixture{directory: directory, binary: binary}
	if os.Geteuid() != 0 {
		return fixture
	}
	nobody, err := user.Lookup("nobody")
	require.NoError(t, err)
	uid, err := strconv.ParseInt(nobody.Uid, 10, 32)
	require.NoError(t, err)
	gid, err := strconv.ParseInt(nobody.Gid, 10, 32)
	require.NoError(t, err)
	require.NotZero(t, uid)
	fixture.identity = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	input, err := os.Open(binary)
	require.NoError(t, err)
	defer func() { require.NoError(t, input.Close()) }()
	fixture.binary = filepath.Join(directory, "reader-process")
	output, err := os.OpenFile(fixture.binary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	require.NoError(t, err)
	_, err = io.Copy(output, input)
	require.NoError(t, err)
	require.NoError(t, output.Close())
	require.NoError(t, os.Chmod(fixture.binary, 0o755))
	return fixture
}

func (p *readerProcessFixture) run(t *testing.T, unprivileged bool, values map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, p.binary, "-test.run=^TestCredentialReaderProcess$", "-test.count=1")
	command.Dir = p.directory
	command.Env = os.Environ()
	for key, value := range values {
		command.Env = append(command.Env, "AIB_CREDENTIAL_READER_"+key+"="+value)
	}
	if unprivileged && p.identity != nil {
		command.SysProcAttr = &syscall.SysProcAttr{Credential: p.identity}
	}
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	require.NoError(t, ctx.Err(), "credential source blocked instead of failing promptly: %s", output.String())
	require.NoError(t, err, "real reader child failed: %s", output.String())
}
