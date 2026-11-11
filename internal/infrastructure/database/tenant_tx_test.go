package database

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type failBeginner struct{ called bool }

func (f *failBeginner) Begin(context.Context) (pgx.Tx, error) {
	f.called = true
	return nil, errors.New("must not be called")
}

func TestWithTenantTxRejectsInvalidTenantWithoutTouchingDB(t *testing.T) {
	for _, id := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000", "' OR 1=1 --"} {
		db := &failBeginner{}
		err := WithTenantTx(context.Background(), db, id, func(pgx.Tx) error {
			t.Fatal("callback must not run")
			return nil
		})
		if !errors.Is(err, ErrInvalidTenant) {
			t.Errorf("id %q: want ErrInvalidTenant, got %v", id, err)
		}
		if db.called {
			t.Errorf("id %q: database was touched", id)
		}
	}
}
