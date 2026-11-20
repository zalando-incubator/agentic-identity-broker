//go:build integration

package storage_test

func providerUpdateError(_ bool, err error) error {
	return err
}
