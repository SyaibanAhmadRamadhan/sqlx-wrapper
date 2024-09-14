package wsqlx

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"reflect"
	"strings"
)

type callbackRows func(rows *sqlx.Rows) (err error)

type QueryRowScanType uint8

const (
	QueryRowScanTypeDefault QueryRowScanType = iota + 1
	QueryRowScanTypeStruct
)

const TracerName = "github.com/SyaibanAhmadRamadhan/sqlx-wrapper"
const InstrumentVersion = "v1.0.0"
const sqlOperationUnknown = "UNKNOWN"

const (
	DBQueryTextKey     = attribute.Key("db.pg.query.text")
	DBOperationNameKey = attribute.Key("db.pg.operation_name")
	DBArgs             = attribute.Key("db.pg.args")
	DBTxIsolationLevel = attribute.Key("db.pg.tx.isolation")
	DBTxReadOnly       = attribute.Key("db.pg.tx.readonly")
)

func recordError(span trace.Span, err error) {
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func makeParamAttr(args []any) attribute.KeyValue {
	ss := make([]string, len(args))
	for i := range args {
		t := reflect.TypeOf(args[i])
		ss[i] = fmt.Sprintf("%s: %v", t, args[i])
	}

	return DBArgs.StringSlice(ss)
}

func sqlOperationName(stmt string) string {
	parts := strings.Fields(stmt)
	if len(parts) == 0 {
		return sqlOperationUnknown
	}
	return strings.ToUpper(parts[0])
}
