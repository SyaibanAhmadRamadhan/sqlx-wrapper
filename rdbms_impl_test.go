package wsqlx_test

import (
	"context"
	"database/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Masterminds/squirrel"
	wsqlx "github.com/SyaibanAhmadRamadhan/sqlx-wrapper"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
)

func Test_sqlxWrapper_Queryx(t *testing.T) {
	dbMock, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer dbMock.Close()

	ctx := context.TODO()
	sqlxDB := sqlx.NewDb(dbMock, "sqlmock")

	sqlxx := wsqlx.NewRdbms(sqlxDB)

	t.Run("should return correct Query result", func(t *testing.T) {
		query := squirrel.Select("*").From("users").
			Where(squirrel.Eq{"id": 1})

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM users WHERE id = ?`)).
			WithArgs(1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

		err := sqlxx.QuerySq(ctx, query, func(rows *sqlx.Rows) (err error) {
			for rows.Next() {
				var id int
				err := rows.Scan(&id)
				require.NoError(t, err)
				require.Equal(t, 1, id)
			}
			return nil
		})
		require.NoError(t, err)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should return correct Query Row result", func(t *testing.T) {
		query := squirrel.Select("*").From("users").
			Where(squirrel.Eq{"id": 1}).Limit(1)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM users WHERE id = ? LIMIT 1`)).
			WithArgs(1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

		var id int
		err = sqlxx.QueryRowSq(ctx, query, wsqlx.QueryRowScanTypeDefault, &id)
		require.NoError(t, err)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should return correct Query Row result", func(t *testing.T) {
		query := squirrel.Select("*").From("users").
			Where(squirrel.Eq{"id": 1}).Limit(1)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM users WHERE id = ? LIMIT 1`)).
			WithArgs(1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

		var id int
		err = sqlxx.QueryRowSq(ctx, query, wsqlx.QueryRowScanTypeDefault, &id)
		require.NoError(t, err)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should be return with commit db", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectCommit()

		err = sqlxx.DoTx(ctx, &sql.TxOptions{}, func(tx wsqlx.Rdbms) (err error) {
			return nil
		})
		require.NoError(t, err)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should be return with rollback db", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectRollback()

		err = sqlxx.DoTx(ctx, &sql.TxOptions{}, func(tx wsqlx.Rdbms) (err error) {
			return errors.New("rollback")
		})
		require.Error(t, err)

		require.NoError(t, mock.ExpectationsWereMet())
	})
}
