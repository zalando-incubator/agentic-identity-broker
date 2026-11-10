//go:build integration

package postgres

func providerUpdateError(_ bool, err error) error {
	return err
}
