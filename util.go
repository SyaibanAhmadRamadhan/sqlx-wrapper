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
	DBQueryParameter   = attribute.Key("db.query.parameter")
	DBTxIsolationLevel = attribute.Key("db.tx.isolation")
	DBTxReadOnly       = attribute.Key("db.tx.readonly")
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

	return DBQueryParameter.StringSlice(ss)
}
