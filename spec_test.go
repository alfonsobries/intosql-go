package intosql

import (
	"encoding/json"
	"testing"
)

func TestNewSpecFillsIntoSQLDefaults(t *testing.T) {
	encoded, err := json.Marshal(NewSpec(SpecOptions{Dialect: DialectPostgreSQL}))
	if err != nil {
		t.Fatal(err)
	}

	equal(t, string(encoded), `{"version":1,`+
		`"input":{"sheet":null,"header_row":null,"delimiter":null,"encoding":null},`+
		`"output":{"format":"sql","dialect":"postgresql","table":null,"schema":null,"create_table":true,"column_names":"snake_case","style":"inserts","batch_rows":500},`+
		`"inference":{"sample_rows":10000,"date_order":"auto","keep_leading_zeros":true,"locale":null},`+
		`"columns":[]}`)

	custom := NewSpec(SpecOptions{
		Dialect:         DialectMariaDB,
		Format:          FormatJSON,
		Table:           "ventas",
		Sheet:           "Q3",
		Schema:          "sales",
		SkipCreateTable: true,
		BatchRows:       50,
	})

	equal(t, *custom.Input.Sheet, "Q3")
	equal(t, *custom.Output.Schema, "sales")
	equal(t, *custom.Output.Table, "ventas")
	equal(t, custom.Output.Format, FormatJSON)
	equal(t, custom.Output.CreateTable, false)
	equal(t, custom.Output.BatchRows, 50)
}
