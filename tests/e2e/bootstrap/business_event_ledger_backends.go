//go:build !integration

package bootstrap

import "errors"

func LedgerBackends() []LedgerBackend { return []LedgerBackend{LedgerMemory} }

func openLedgerPostgres(*LedgerHarness) error {
	return errors.New("PostgreSQL ledger acceptance requires the integration build tag")
}
